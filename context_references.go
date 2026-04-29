package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MaxLineLimit is the maximum number of lines to read from a file.
const MaxLineLimit = 1000

// MaxFolderDepth is the default maximum depth for folder listings.
const MaxFolderDepth = 3

// MaxURLSize is the maximum response body size for URL fetching (500KB).
const MaxURLSize = 500 * 1024

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
	Message         string             // final message with context injected
	OriginalMessage string             // original message before expansion
	References      []ContextReference // parsed references
	Warnings        []string           // warnings (soft limit exceeded, etc.)
	InjectedTokens  int                // estimated tokens injected
	Expanded        bool               // whether any references were expanded
	Blocked         bool               // whether injection was blocked (hard limit)
}

// fileCache caches file content to avoid re-reading the same file.
var fileCache struct {
	mu      sync.RWMutex
	entries map[string]string // absPath -> content
}

func init() {
	fileCache.entries = make(map[string]string)
}

// referencePattern matches @file:path, @folder:path, @diff, @staged, @git:N, @url:url
// Uses negative lookbehind-like check: @ must not be preceded by a word character
// (to exclude email addresses like user@domain.com and social handles like @username).
// The regex requires either a known simple keyword (diff/staged) or a colon after the kind.
// Go's regexp doesn't support lookbehind, so email/social exclusion is done in ParseContextReferences.
var referencePattern = regexp.MustCompile(`@(?:(?P<simple>diff|staged)\b|(?P<kind>file|folder|git|url):(?P<value>"[^"]+"|\S+))`)

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

	// Pre-filter: skip @ that appear inside email addresses or social handles.
	// The regex already handles this via (?<![a-zA-Z0-9_/]) but Go's regexp
	// doesn't support lookbehind, so we do a manual pre-filter.
	matches := referencePattern.FindAllStringSubmatchIndex(message, -1)
	var refs []ContextReference

	for _, m := range matches {
		// Check that @ is not preceded by a word character (email/social exclusion)
		atPos := m[0]
		if atPos > 0 {
			prev := message[atPos-1]
			if isWordChar(prev) {
				continue // skip: this @ is part of an email or similar
			}
		}

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

		// Strip surrounding quotes from value
		value = stripQuotes(value)

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

// isWordChar checks if a byte is a word character (letter, digit, underscore, or slash).
// Used to exclude email addresses (user@domain) and similar patterns.
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_' || b == '/'
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
			// Inject the error as a context block so the model understands what happened
			// instead of just seeing a stripped message + cryptic warning
			errorBlock := fmt.Sprintf("## %s (error)\n%s", ref.Raw, warning)
			blocks = append(blocks, errorBlock)
			warnings = append(warnings, warning)
		}
		if block != "" {
			blocks = append(blocks, block)
			injectedTokens += len(block) / 4
		}
	}

	// Token budget guardrails
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
		return expandGitReference(ref, cwd, []string{"diff"}, "@diff")
	case "staged":
		return expandGitReference(ref, cwd, []string{"diff", "--staged"}, "@staged")
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
		return expandGitReference(ref, cwd, []string{"log", fmt.Sprintf("-%d", count), "-p"}, fmt.Sprintf("@git:%d", count))
	case "url":
		return expandURLReference(ref)
	default:
		return "", ref.Raw + ": unsupported reference type"
	}
}

// expandFileReference reads a file and returns it as a code block.
func expandFileReference(ref ContextReference, cwd string) (block string, warning string) {
	path := resolvePath(cwd, ref.Target)

	if err := ensurePathAllowed(path, cwd); err != "" {
		return "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		hint := ""
		if os.IsPermission(err) {
			hint = " (permission denied)"
		}
		return "", fmt.Sprintf("File not found: %s%s", ref.Target, hint)
	}
	if info.IsDir() {
		return "", fmt.Sprintf("%s: path is a directory, use @folder: instead", ref.Raw)
	}

	// Check file size — reject files over 10MB
	const maxFileSize int64 = 10 * 1024 * 1024
	if info.Size() > maxFileSize {
		return "", fmt.Sprintf("%s: file is too large (%d bytes, max %d MB)", ref.Raw, info.Size(), maxFileSize/(1024*1024))
	}

	// Check cache first
	fileCache.mu.RLock()
	text, cached := fileCache.entries[path]
	fileCache.mu.RUnlock()

	if !cached {
		// Stream-read file with line limit (avoids loading huge files into memory)
		lines, truncated, err := readFileLines(path, MaxLineLimit)
		if err != nil {
			hint := ""
			if os.IsPermission(err) {
				hint = " (permission denied)"
			}
			return "", fmt.Sprintf("Cannot read file: %s%s", ref.Target, hint)
		}

		if isBinaryLines(lines) {
			return "", fmt.Sprintf("%s: binary files are not supported", ref.Raw)
		}

		text = strings.Join(lines, "\n")
		if truncated {
			text += fmt.Sprintf("\n... (truncated at %d lines)", MaxLineLimit)
		}

		// Cache the full content
		fileCache.mu.Lock()
		fileCache.entries[path] = text
		fileCache.mu.Unlock()
	}

	lang := codeFenceLanguage(path)

	// Apply line range if specified
	var displayText string
	var linesHint string
	if ref.LineStart > 0 {
		allLines := strings.Split(text, "
")
		totalLines := len(allLines)

		// If requested start is beyond file length, return a clear message
		if ref.LineStart > totalLines {
			return "", fmt.Sprintf("%s: file has %d lines, but line %d was requested", ref.Raw, totalLines, ref.LineStart)
		}

		startIdx := ref.LineStart - 1
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := len(allLines)
		if ref.LineEnd > 0 && ref.LineEnd <= len(allLines) {
			endIdx = ref.LineEnd
		}
		if endIdx < startIdx {
			endIdx = startIdx
		}

		// Cap to MaxLineLimit
		lineCount := endIdx - startIdx
		if lineCount > MaxLineLimit {
			endIdx = startIdx + MaxLineLimit
			lineCount = MaxLineLimit
		}

		var selected []string
		for i := startIdx; i < endIdx; i++ {
			selected = append(selected, fmt.Sprintf("%4d | %s", i+1, allLines[i]))
		}
		displayText = strings.Join(selected, "
")
		if lineCount >= MaxLineLimit {
			displayText += fmt.Sprintf("
... (truncated at %d lines)", MaxLineLimit)
		}

		linesHint = fmt.Sprintf(" (lines %d-%d, file has %d lines)", ref.LineStart, ref.LineEnd, totalLines)
		if ref.LineEnd > totalLines {
			linesHint = fmt.Sprintf(" (lines %d-%d, file has %d lines - requested end %d adjusted)", ref.LineStart, totalLines, totalLines, ref.LineEnd)
		}
	} else {
		displayText = text
	}

	tokens := len(displayText) / 4

	return fmt.Sprintf("## @file:%s%s (%d tokens)\n```%s\n%s\n```", ref.Target, linesHint, tokens, lang, displayText), ""
}

// expandFolderReference lists a directory tree.
func expandFolderReference(ref ContextReference, cwd string) (block string, warning string) {
	path := resolvePath(cwd, ref.Target)

	if err := ensurePathAllowed(path, cwd); err != "" {
		return "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		hint := ""
		if os.IsPermission(err) {
			hint = " (permission denied)"
		}
		return "", fmt.Sprintf("Folder not found: %s%s", ref.Target, hint)
	}
	if !info.IsDir() {
		return "", fmt.Sprintf("%s: path is not a directory, use @file: instead", ref.Raw)
	}

	// Check if folder is empty
	if dir, err := os.Open(path); err == nil {
		if names, err := dir.Readdirnames(1); len(names) == 0 && err == nil {
			return fmt.Sprintf("## @folder:%s (0 tokens)
(empty directory - no files or subdirectories)", ref.Target), ""
		}
		dir.Close()
	}

	listing := buildFolderListing(path, cwd, 200, MaxFolderDepth)
	tokens := len(listing) / 4

	return fmt.Sprintf("## @folder:%s (%d tokens)\n%s", ref.Target, tokens, listing), ""
}

// expandGitReference runs a git command and returns its output.
func expandGitReference(ref ContextReference, cwd string, args []string, label string) (block string, warning string) {
	// First check if we're in a git repository
	gitCheck := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	gitCheck.Dir = cwd
	gitCheck.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := gitCheck.CombinedOutput(); err != nil {
		if strings.Contains(string(output), "not a git repository") {
			return "", fmt.Sprintf("%s: not a git repository - git references require a git repo", label)
		}
		return "", fmt.Sprintf("%s: git is not installed or not available in PATH", label)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(output))
		if errMsg == "" {
			errMsg = "unknown git error"
		}
		return "", fmt.Sprintf("%s: %s", label, errMsg)
	}

	content := strings.TrimSpace(string(output))
	isEmpty := content == ""
	if isEmpty {
		// Provide context-specific empty messages so the model understands what "empty" means
		switch label {
		case "@diff":
			content = "(working tree is clean — no unstaged changes)"
		case "@staged":
			content = "(nothing staged — no staged changes to commit)"
		default:
			if strings.HasPrefix(label, "@git:") {
				content = "(no commits found in this repository)"
			} else {
				content = "(no output)"
			}
		}
	}
	tokens := len(content) / 4

	if isEmpty {
		return fmt.Sprintf("## %s (0 tokens)\n%s", label, content), ""
	}
	return fmt.Sprintf("## %s (%d tokens)\n```diff\n%s\n```", label, tokens, content), ""
}

// expandURLReference fetches a URL and returns its content.
func expandURLReference(ref ContextReference) (block string, warning string) {
	url := ref.Target

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", "Invalid URL: only http/https URLs are supported"
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
		return "", fmt.Sprintf("Invalid URL: %s", err.Error())
	}
	req.Header.Set("User-Agent", "miniClaudeCode/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Sprintf("Fetch failed: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Sprintf("Fetch failed with status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxURLSize))
	if err != nil {
		return "", fmt.Sprintf("Read failed: %s", err.Error())
	}

	content := string(body)
	if content == "" {
		return "", "No content returned"
	}

	// Extract content from HTML
	text := extractHTMLContent(content)
	tokens := len(text) / 4

	// Extract title if available
	title := extractHTMLTitle(content)
	titleHint := ""
	if title != "" {
		titleHint = fmt.Sprintf(" — %s", title)
	}

	return fmt.Sprintf("## @url:%s%s (%d tokens)\n%s", url, titleHint, tokens, text), ""
}

// resolvePath resolves a path relative to cwd.
func resolvePath(cwd, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(cwd, target))
}

// ensurePathAllowed checks if a path is allowed:
// 1. Not in a sensitive directory
// 2. Not outside the CWD (path traversal protection)
func ensurePathAllowed(path string, cwd string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	// Check sensitive directories
	for _, dir := range sensitiveDirs {
		sensitivePath := filepath.Join(home, dir)
		if strings.HasPrefix(absPath, sensitivePath+string(filepath.Separator)) || absPath == sensitivePath {
			return "path is in a sensitive directory and cannot be attached"
		}
	}

	// Check path traversal: path must be within cwd
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(absPath, absCwd+string(filepath.Separator)) && absPath != absCwd {
		return "path traversal outside working directory is not allowed"
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
	result = regexp.MustCompile(`\s{2,}`).ReplaceAllString(result, " ")
	return strings.TrimSpace(result)
}

// stripTrailingPunctuation removes trailing punctuation from a reference value.
func stripTrailingPunctuation(value string) string {
	stripped := strings.TrimRight(value, ",.;!?")
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

// stripQuotes removes surrounding quotes from a value.
func stripQuotes(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1:len(value)-1]
		}
	}
	return value
}

// readFileLines reads a file line-by-line up to maxLines, returning the lines
// and a boolean indicating whether the file was truncated.
func readFileLines(path string, maxLines int) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line length

	count := 0
	truncated := false
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		count++
		if count >= maxLines {
			truncated = true
			break
		}
	}

	return lines, truncated, scanner.Err()
}

// isBinaryLines checks if the first few lines appear to be binary.
func isBinaryLines(lines []string) bool {
	checkLen := len(lines)
	if checkLen > 16 {
		checkLen = 16
	}
	for _, line := range lines[:checkLen] {
		for _, b := range []byte(line) {
			if b == 0 {
				return true
			}
		}
	}
	return false
}

// isBinaryContent checks if content appears to be binary (byte slice version).
func isBinaryContent(content []byte) bool {
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

// buildFolderListing creates a tree listing of a directory with depth control.
func buildFolderListing(path string, cwd string, limit int, maxDepth int) string {
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		rel = path
	}
	var lines []string
	lines = append(lines, rel+"/")

	count := 0
	buildFolderListingRecursive(path, cwd, rel, &lines, &count, limit, maxDepth, 0)

	if count >= limit {
		lines = append(lines, "- ...")
	}
	return strings.Join(lines, "\n")
}

func buildFolderListingRecursive(path string, cwd string, baseRel string, lines *[]string, count *int, limit int, maxDepth int, depth int) {
	if depth >= maxDepth || *count >= limit {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}

	indent := strings.Repeat("  ", depth)

	for _, entry := range entries {
		if *count >= limit {
			return
		}

		name := entry.Name()
		// Skip hidden files/dirs
		if strings.HasPrefix(name, ".") {
			continue
		}

		if entry.IsDir() {
			*lines = append(*lines, indent+"  - "+name+"/")
			*count++
			subPath := filepath.Join(path, name)
			buildFolderListingRecursive(subPath, cwd, baseRel, lines, count, limit, maxDepth, depth+1)
		} else {
			*lines = append(*lines, indent+"  - "+name)
			*count++
		}
	}
}

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
	looksLikeFile := strings.Contains(path, ".") ||
		strings.Contains(path, "/") ||
		strings.Contains(path, "\\") ||
		!strings.Contains(path, ":")

	if looksLikeFile && start > 0 {
		return path, start, end
	}

	return value, 0, 0
}

// extractHTMLContent extracts meaningful content from HTML, removing scripts/styles.
func extractHTMLContent(html string) string {
	// Remove <script> and <style> blocks entirely (including content)
	scriptRe := regexp.MustCompile(`<script[^>]*>.*?</script>`)
	styleRe := regexp.MustCompile(`<style[^>]*>.*?</style>`)
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")

	// Try to extract <article>, <main>, or <body> content first
	articleRe := regexp.MustCompile(`<article[^>]*>(.*?)</article>`)
	if m := articleRe.FindStringSubmatch(text); len(m) > 1 {
		text = m[1]
	} else {
		mainRe := regexp.MustCompile(`<main[^>]*>(.*?)</main>`)
		if m := mainRe.FindStringSubmatch(text); len(m) > 1 {
			text = m[1]
		} else {
			bodyRe := regexp.MustCompile(`<body[^>]*>(.*?)</body>`)
			if m := bodyRe.FindStringSubmatch(text); len(m) > 1 {
				text = m[1]
			}
		}
	}

	// Remove remaining HTML tags
	tagRe := regexp.MustCompile(`<[^>]+>`)
	text = tagRe.ReplaceAllString(text, "")

	// Collapse excessive whitespace
	wsRe := regexp.MustCompile(`\n{3,}`)
	text = wsRe.ReplaceAllString(text, "\n\n")
	spaceRe := regexp.MustCompile(`  +`)
	text = spaceRe.ReplaceAllString(text, " ")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	return strings.TrimSpace(text)
}

// extractHTMLTitle extracts the <title> from HTML.
func extractHTMLTitle(html string) string {
	titleRe := regexp.MustCompile(`<title[^>]*>(.*?)</title>`)
	m := titleRe.FindStringSubmatch(html)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}