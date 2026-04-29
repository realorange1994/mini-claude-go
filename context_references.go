package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ContextReference represents a parsed @ reference in a user message.
type ContextReference struct {
	Raw       string // original text like @file:main.go
	Kind      string // "file", "folder", "diff", "staged", "git", "url"
	Target    string // path, url, or count
	LineStart int    // 1-indexed line start (0 = not specified)
	LineEnd   int    // 1-indexed line end (0 = not specified)
	Start     int    // byte offset in original message
	End       int    // byte end offset
}

// ContextReferenceResult holds the result of expanding @ references.
type ContextReferenceResult struct {
	Message         string              // final message with context injected
	OriginalMessage string              // original message before expansion
	References      []ContextReference  // parsed references
	Warnings        []string            // warnings (soft limit exceeded, etc.)
	InjectedTokens  int                 // estimated tokens injected
	Expanded        bool                // whether any references were expanded
	Blocked         bool                // whether injection was blocked (hard limit)
}

// referencePattern matches @file:path, @folder:path, @diff, @staged, @git:N, @url:url
var referencePattern = regexp.MustCompile(`@(?:(?P<simple>diff|staged)\b|(?P<kind>file|folder|git|url):(?P<value>\S+))`)

// Sensitive directories that should never be exposed via @ references.
var sensitiveDirs = []string{
	".ssh", ".aws", ".gnupg", ".kube", ".docker", ".azure",
	".config/gh", ".config/git",
}

// ParseContextReferences extracts @ references from a user message.
func ParseContextReferences(message string) []ContextReference {
	if message == "" {
		return nil
	}

	matches := referencePattern.FindAllStringSubmatchIndex(message, -1)
	var refs []ContextReference

	for _, m := range matches {
		// m[0], m[1] = full match start/end
		raw := message[m[0]:m[1]]

		// Check for simple references (@diff, @staged)
		simpleStart, simpleEnd := m[2], m[3]
		if simpleStart >= 0 && simpleEnd >= 0 {
			kind := message[simpleStart:simpleEnd]
			refs = append(refs, ContextReference{
				Raw:    raw,
				Kind:   kind,
				Target: "",
				Start:  m[0],
				End:    m[1],
			})
			continue
		}

		// Named references (@file:path, @folder:path, @git:N, @url:url)
		kindStart, kindEnd := m[4], m[5]
		valueStart, valueEnd := m[6], m[7]
		kind := message[kindStart:kindEnd]
		value := stripTrailingPunctuation(message[valueStart:valueEnd])

		// Parse line range for @file:path:10-50
		var target string
		var lineStart, lineEnd int
		if kind == "file" {
			target, lineStart, lineEnd = parseFileTarget(value)
		} else {
			target = value
		}

		refs = append(refs, ContextReference{
			Raw:       raw,
			Kind:      kind,
			Target:    target,
			LineStart: lineStart,
			LineEnd:   lineEnd,
			Start:     m[0],
			End:       m[1],
		})
	}

	return refs
}

// PreprocessContextReferences expands @ references in a user message.
// Returns the expanded message with context injected, plus metadata.
func PreprocessContextReferences(message string, cwd string, contextLength int) ContextReferenceResult {
	refs := ParseContextReferences(message)
	if len(refs) == 0 {
		return ContextReferenceResult{
			Message:         message,
			OriginalMessage: message,
		}
	}

	var warnings []string
	var blocks []string
	injectedTokens := 0

	for _, ref := range refs {
		block, warning := expandReference(ref, cwd)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if block != "" {
			blocks = append(blocks, block)
			injectedTokens += len(block) / 4 // rough token estimate
		}
	}

	// Token budget guardrails (matching hermes-agent)
	hardLimit := contextLength / 2 // 50% hard limit
	softLimit := contextLength / 4 // 25% soft limit
	if hardLimit < 1 {
		hardLimit = 1
	}
	if softLimit < 1 {
		softLimit = 1
	}

	if injectedTokens > hardLimit {
		warnings = append(warnings, fmt.Sprintf(
			"@ context injection refused: %d tokens exceeds the 50%% hard limit (%d).",
			injectedTokens, hardLimit))
		return ContextReferenceResult{
			Message:         message,
			OriginalMessage: message,
			References:      refs,
			Warnings:        warnings,
			InjectedTokens:  injectedTokens,
			Expanded:        false,
			Blocked:         true,
		}
	}

	if injectedTokens > softLimit {
		warnings = append(warnings, fmt.Sprintf(
			"@ context injection warning: %d tokens exceeds the 25%% soft limit (%d).",
			injectedTokens, softLimit))
	}

	// Remove @ reference tokens from message, then append context blocks
	stripped := removeReferenceTokens(message, refs)
	final := stripped
	if len(warnings) > 0 {
		final += "\n\n--- Context Warnings ---\n"
		for _, w := range warnings {
			final += "- " + w + "\n"
		}
	}
	if len(blocks) > 0 {
		final += "\n\n--- Attached Context ---\n"
		final += "IMPORTANT: The following context has been attached by the user. Use this context directly instead of calling tools to read the same files or run the same commands.\n\n"
		final += strings.Join(blocks, "\n\n")
	}

	return ContextReferenceResult{
		Message:         strings.TrimSpace(final),
		OriginalMessage: message,
		References:      refs,
		Warnings:        warnings,
		InjectedTokens:  injectedTokens,
		Expanded:        len(blocks) > 0 || len(warnings) > 0,
		Blocked:         false,
	}
}

// expandReference expands a single @ reference into a context block.
func expandReference(ref ContextReference, cwd string) (block string, warning string) {
	switch ref.Kind {
	case "file":
		return expandFileReference(ref, cwd)
	case "folder":
		return expandFolderReference(ref, cwd)
	case "diff":
		return expandGitReference(ref, cwd, []string{"diff"}, "git diff")
	case "staged":
		return expandGitReference(ref, cwd, []string{"diff", "--staged"}, "git diff --staged")
	case "git":
		count := 1
		if ref.Target != "" {
			fmt.Sscanf(ref.Target, "%d", &count)
			if count < 1 {
				count = 1
			}
			if count > 10 {
				count = 10
			}
		}
		return expandGitReference(ref, cwd, []string{"log", fmt.Sprintf("-%d", count), "-p"}, fmt.Sprintf("git log -%d -p", count))
	case "url":
		return expandURLReference(ref)
	default:
		return "", ref.Raw + ": unsupported reference type"
	}
}

// expandFileReference reads a file and returns it as a code block.
func expandFileReference(ref ContextReference, cwd string) (block string, warning string) {
	path := resolvePath(cwd, ref.Target)

	if err := ensurePathAllowed(path); err != "" {
		return "", ref.Raw + ": " + err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", ref.Raw + ": file not found"
	}
	if info.IsDir() {
		return "", ref.Raw + ": path is a directory, use @folder: instead"
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", ref.Raw + ": " + err.Error()
	}

	if isBinaryContent(content) {
		return "", ref.Raw + ": binary files are not supported"
	}

	text := string(content)
	lang := codeFenceLanguage(path)

	// Apply line range if specified
	var displayText string
	var rangeHint string
	if ref.LineStart > 0 {
		lines := strings.Split(text, "\n")
		startIdx := ref.LineStart - 1
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := len(lines)
		if ref.LineEnd > 0 && ref.LineEnd <= len(lines) {
			endIdx = ref.LineEnd
		}
		if endIdx < startIdx {
			endIdx = startIdx
		}

		var selected []string
		for i := startIdx; i < endIdx; i++ {
			selected = append(selected, fmt.Sprintf("%4d | %s", i+1, lines[i]))
		}
		displayText = strings.Join(selected, "\n")
		rangeHint = fmt.Sprintf(":%d-%d", ref.LineStart, ref.LineEnd)
	} else {
		displayText = text
	}

	tokens := len(displayText) / 4

	return fmt.Sprintf("\U0001f4c4 @file:\"%s\"%s (%d tokens)\n```%s\n%s\n```", ref.Target, rangeHint, tokens, lang, displayText), ""
}

// expandFolderReference lists a directory tree.
func expandFolderReference(ref ContextReference, cwd string) (block string, warning string) {
	path := resolvePath(cwd, ref.Target)

	if err := ensurePathAllowed(path); err != "" {
		return "", ref.Raw + ": " + err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", ref.Raw + ": folder not found"
	}
	if !info.IsDir() {
		return "", ref.Raw + ": path is not a directory"
	}

	listing := buildFolderListing(path, cwd, 200)
	tokens := len(listing) / 4

	return fmt.Sprintf("\U0001f4c1 @folder:\"%s\" (%d tokens)\n%s", ref.Target, tokens, listing), ""
}

// expandGitReference runs a git command and returns its output.
func expandGitReference(ref ContextReference, cwd string, args []string, label string) (block string, warning string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", ref.Raw + ": " + strings.TrimSpace(string(output))
	}

	content := strings.TrimSpace(string(output))
	if content == "" {
		content = "(no output)"
	}
	tokens := len(content) / 4

	return fmt.Sprintf("\U0001f9fe %s (%d tokens)\n```diff\n%s\n```", label, tokens, content), ""
}

// expandURLReference fetches a URL and returns its content.
func expandURLReference(ref ContextReference) (block string, warning string) {
	url := ref.Target

	// Validate URL scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", ref.Raw + ": only http/https URLs are supported"
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", ref.Raw + ": " + err.Error()
	}
	req.Header.Set("User-Agent", "miniClaudeCode/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", ref.Raw + ": fetch failed: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", ref.Raw + ": fetch failed with status " + resp.Status
	}

	// Limit response body to 500KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024))
	if err != nil {
		return "", ref.Raw + ": read failed: " + err.Error()
	}

	content := string(body)
	if content == "" {
		return "", ref.Raw + ": no content returned"
	}

	// Strip HTML tags for text extraction
	text := stripHTMLTags(content)
	tokens := len(text) / 4

	return fmt.Sprintf("\U0001f310 @url:\"%s\" (%d tokens)\n%s", url, tokens, text), ""
}

// resolvePath resolves a path relative to cwd.
func resolvePath(cwd, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(cwd, target))
}

// ensurePathAllowed checks if a path is in a sensitive directory.
func ensurePathAllowed(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	for _, dir := range sensitiveDirs {
		sensitivePath := filepath.Join(home, dir)
		if strings.HasPrefix(absPath, sensitivePath+string(filepath.Separator)) || absPath == sensitivePath {
			return "path is in a sensitive directory and cannot be attached"
		}
	}

	return ""
}

// removeReferenceTokens removes @ reference tokens from the message.
func removeReferenceTokens(message string, refs []ContextReference) string {
	if len(refs) == 0 {
		return message
	}

	var parts []string
	cursor := 0
	for _, ref := range refs {
		parts = append(parts, message[cursor:ref.Start])
		cursor = ref.End
	}
	parts = append(parts, message[cursor:])

	result := strings.Join(parts, "")
	// Clean up extra whitespace left behind
	result = regexp.MustCompile(`\s{2,}`).ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

// stripTrailingPunctuation removes trailing punctuation from a reference value.
func stripTrailingPunctuation(value string) string {
	stripped := strings.TrimRight(value, ",.;!?")
	// Remove unbalanced closing brackets
	for strings.HasSuffix(stripped, ")") || strings.HasSuffix(stripped, "]") || strings.HasSuffix(stripped, "}") {
		closer := stripped[len(stripped)-1]
		var opener byte
		switch closer {
		case ')':
			opener = '('
		case ']':
			opener = '['
		case '}':
			opener = '{'
		}
		if strings.Count(stripped, string(closer)) > strings.Count(stripped, string(opener)) {
			stripped = stripped[:len(stripped)-1]
		} else {
			break
		}
	}
	return stripped
}

// isBinaryContent checks if content appears to be binary.
func isBinaryContent(content []byte) bool {
	// Check for null bytes in first 4KB
	checkLen := len(content)
	if checkLen > 4096 {
		checkLen = 4096
	}
	for _, b := range content[:checkLen] {
		if b == 0 {
			return true
		}
	}
	return false
}

// codeFenceLanguage returns the markdown code fence language for a file extension.
func codeFenceLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	langMap := map[string]string{
		".go":   "go",
		".rs":   "rust",
		".py":   "python",
		".js":   "javascript",
		".ts":   "typescript",
		".tsx":  "tsx",
		".jsx":  "jsx",
		".json": "json",
		".md":   "markdown",
		".sh":   "bash",
		".yml":  "yaml",
		".yaml": "yaml",
		".toml": "toml",
		".c":    "c",
		".cpp":  "cpp",
		".h":    "c",
		".hpp":  "cpp",
		".java": "java",
		".rb":   "ruby",
		".php":  "php",
		".sql":  "sql",
		".html": "html",
		".css":  "css",
	}
	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return ""
}

// buildFolderListing creates a tree listing of a directory.
func buildFolderListing(path string, cwd string, limit int) string {
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		rel = path
	}
	var lines []string
	lines = append(lines, rel+"/")

	count := 0
	filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil || count >= limit {
			return nil
		}
		// Skip hidden files/dirs
		base := filepath.Base(walkPath)
		if strings.HasPrefix(base, ".") && base != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		walkRel, err := filepath.Rel(cwd, walkPath)
		if err != nil {
			return nil
		}
		indent := strings.Repeat("  ", strings.Count(walkRel, string(filepath.Separator))-strings.Count(rel, string(filepath.Separator)))

		if info.IsDir() {
			lines = append(lines, indent+"- "+base+"/")
		} else {
			lines = append(lines, indent+"- "+base)
		}
		count++
		return nil
	})

	if count >= limit {
		lines = append(lines, "- ...")
	}
	return strings.Join(lines, "\n")
}

// estimateMessageTokens is in agent_loop.go

// isLocalEndpoint checks if the base URL points to a local endpoint.
// (Declared in agent_loop.go — this is a duplicate kept for standalone use.)
// isLocalEndpoint is not redeclared here; see agent_loop.go.

// parseFileTarget parses @file target with optional line range: "path:10-50" -> (path, 10, 50)
func parseFileTarget(value string) (target string, lineStart int, lineEnd int) {
	lineRangeRe := regexp.MustCompile(`^(.+):(\d+)(?:-(\d+))?$`)
	m := lineRangeRe.FindStringSubmatch(value)
	if m == nil {
		return value, 0, 0
	}

	path := m[1]
	start := 0
	end := 0
	fmt.Sscanf(m[2], "%d", &start)
	if m[3] != "" {
		fmt.Sscanf(m[3], "%d", &end)
	}

	// Only treat as line range if the "path" part looks like a real file
	// (has an extension or is a known filename). Otherwise it's a Windows path like C:\...
	looksLikeFile := strings.Contains(path, ".") ||
		strings.Contains(path, "/") ||
		strings.Contains(path, "\\") ||
		!strings.Contains(path, ":")

	if looksLikeFile && start > 0 {
		return path, start, end
	}

	return value, 0, 0
}

// stripHTMLTags strips HTML tags for rough text extraction from fetched URLs.
func stripHTMLTags(html string) string {
	tagRe := regexp.MustCompile(`<[^>]+>`)
	text := tagRe.ReplaceAllString(html, "")

	// Collapse excessive whitespace
	wsRe := regexp.MustCompile(`\n{3,}`)
	text = wsRe.ReplaceAllString(text, "\n\n")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	return strings.TrimSpace(text)
}
