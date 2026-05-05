package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileEditTool edits a file by replacing an exact string with a new string.
type FileEditTool struct {
	registry *Registry // may be nil if tracker is not available
}

func NewFileEditTool(registry *Registry) *FileEditTool {
	return &FileEditTool{registry: registry}
}

func (*FileEditTool) Name() string { return "edit_file" }
func (*FileEditTool) Description() string {
	return "Performs exact string replacements in files.\n\n" +
		"Usage:\n" +
		"- You must use read_file at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.\n" +
		"- When editing text from read_file output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: line number + tab. Everything after that is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.\n" +
		"- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.\n" +
		"- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.\n" +
		"- The edit will FAIL if old_string is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use replace_all to change every instance of old_string.\n" +
		"- Use replace_all for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance."
}

func (*FileEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to find. Use empty string to create a new file.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The text to replace it with (must be different from old_string).",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences (default: false).",
			},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

func (*FileEditTool) CheckPermissions(params map[string]any) string { return "" }

func (e *FileEditTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		pathStr, _ = params["path"].(string)
	}
	oldStr, _ := params["old_string"].(string)
	newStr, _ := params["new_string"].(string)
	replaceAll, _ := params["replace_all"].(bool)

	fp := expandPath(pathStr)

	// SECURITY: Block UNC paths before any filesystem I/O to prevent NTLM credential leaks.
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

	// Read-before-write validation and concurrent modification detection.
	if e.registry != nil {
		if staleMsg := e.registry.CheckFileStale(fp); staleMsg != "" {
			return ToolResult{Output: staleMsg, IsError: true}
		}
	}

	// Check for identical old/new strings (matching official behavior)
	if oldStr == newStr {
		return ToolResult{Output: fmt.Sprintf("Error: old_string and new_string must be different"), IsError: true}
	}

	if oldStr == "" {
		// Official: allows creating a new file when old_string is empty
		exists := true
		if _, err := os.Stat(fp); os.IsNotExist(err) {
			exists = false
		}
		if exists {
			// Allow writing to an existing empty file (matching upstream behavior)
			existingData, readErr := os.ReadFile(fp)
			if readErr != nil || strings.TrimSpace(string(existingData)) != "" {
				return ToolResult{Output: "Error: cannot create new file - file already exists with content", IsError: true}
			}
		}
		dir := filepath.Dir(fp)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
		}
		if err := os.WriteFile(fp, []byte(newStr), 0o644); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
		}
		// Update registry so subsequent writes are allowed without re-reading
		if e.registry != nil {
			e.registry.MarkFileReadWithContent(fp, newStr)
		}
		return ToolResult{Output: fmt.Sprintf("Successfully created %s", fp)}
	}

	const maxEditSize = 1 << 30 // 1 GiB
	if info, err := os.Stat(fp); err == nil && info.Size() > maxEditSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (%d bytes, max %d bytes). Use offset/limit to read portions.", info.Size(), maxEditSize), IsError: true}
	}

	// Reject .ipynb files — they must be edited via notebook tool, not raw file edit.
	// Matching upstream behavior: file_edit cannot reliably edit JSON-based notebook format.
	if strings.HasSuffix(strings.ToLower(fp), ".ipynb") {
		return ToolResult{Output: "Error: file is a Jupyter Notebook (.ipynb). Jupyter notebooks cannot be edited with the edit_file tool — use the notebook tool instead.", IsError: true}
	}

	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", pathStr), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	content := string(data)
	hasCRLF := strings.Contains(content, "\r\n")

	// Strip trailing whitespace from new_string (except .md/.mdx) matching official
	ext := strings.ToLower(filepath.Ext(fp))
	if ext != ".md" && ext != ".mdx" {
		newStr = stripTrailingWhitespace(newStr)
	}

	// Normalize curly quotes to straight quotes for matching (matching official Claude Code).
	// LLMs often output curly quotes ("") but files use straight quotes ("").
	contentNorm := normalizeQuotes(content)
	oldStrNorm := normalizeQuotes(oldStr)
	newStrNorm := normalizeQuotes(newStr)

	// Normalize CRLF for matching
	if hasCRLF {
		contentNorm = strings.ReplaceAll(contentNorm, "\r\n", "\n")
		oldStrNorm = strings.ReplaceAll(oldStrNorm, "\r\n", "\n")
		newStrNorm = strings.ReplaceAll(newStrNorm, "\r\n", "\n")
	}

	count := strings.Count(contentNorm, oldStrNorm)
	if count == 0 {
		// Try desanitized version (matching official: reverse sanitized tokens like <fnr> -> <function_results>)
		desanitizedOld := desanitize(oldStrNorm)
		if desanitizedOld != oldStrNorm {
			count = strings.Count(contentNorm, desanitizedOld)
			if count > 0 {
				oldStrNorm = desanitizedOld
				newStrNorm = desanitize(newStrNorm)
			}
		}
	}
	if count == 0 {
		return ToolResult{Output: fmt.Sprintf("Error: old_text not found in %s. Verify the file content.", pathStr), IsError: true}
	}
	if count > 1 && !replaceAll {
		return ToolResult{
			Output: fmt.Sprintf("Warning: old_text appears %d times. Provide more context or set replace_all=true.", count),
			IsError: true,
		}
	}

	// Find positions in normalized content and apply replacement to original
	if newStrNorm == "" && !strings.HasSuffix(oldStrNorm, "\n") {
		// When deleting a line (newStr is empty), also strip a trailing \n
		// that follows the oldString in the file (matching upstream).
		// E.g. deleting "  let x = 1;" from "  let x = 1;\n" should remove the orphan \n too.
		oldWithLF := oldStrNorm + "\n"
		if replaceAll {
			contentNorm = strings.ReplaceAll(contentNorm, oldWithLF, newStrNorm)
		} else if idx := strings.Index(contentNorm, oldWithLF); idx >= 0 {
			contentNorm = contentNorm[:idx] + newStrNorm + contentNorm[idx+len(oldWithLF):]
		} else {
			contentNorm = strings.Replace(contentNorm, oldStrNorm, newStrNorm, 1)
		}
	} else {
		contentNorm = applyReplacement(contentNorm, oldStrNorm, newStrNorm, replaceAll)
	}

	// Restore original quote style
	contentNorm = preserveQuoteStyle(contentNorm, oldStr, newStr, oldStrNorm, newStrNorm, replaceAll)

	// Restore CRLF
	if hasCRLF {
		contentNorm = restoreCRLF(contentNorm)
	}

	if err := os.WriteFile(fp, []byte(contentNorm), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}
	// Update registry so subsequent writes are allowed without re-reading
	if e.registry != nil {
		e.registry.MarkFileReadWithContent(fp, contentNorm)
	}

	return ToolResult{Output: fmt.Sprintf("Successfully edited %s", fp)}
}

// applyReplacement performs string replacement on normalized content.
func applyReplacement(content, oldStr, newStr string, replaceAll bool) string {
	if replaceAll {
		return strings.Replace(content, oldStr, newStr, -1)
	}
	return strings.Replace(content, oldStr, newStr, 1)
}

// preserveQuoteStyle restores original curly quote characters in the replacement.
// Matching upstream's logic:
// 1. If oldStr === actualOldStr (no normalization happened), return newStr as-is.
// 2. If normalization happened, check if the ACTUAL matched text in the file
//    had curly quotes. If so, apply the same curly quote style to newStr.
func preserveQuoteStyle(content, oldStr, newStr, oldStrNorm, newStrNorm string, replaceAll bool) string {
	// If no normalization was needed, oldStr === oldStrNorm, return as-is
	if oldStr == oldStrNorm {
		return newStr
	}

	// Find the position in the normalized content where the match occurred
	idx := strings.Index(content, oldStrNorm)
	if idx < 0 {
		return newStr
	}

	// Extract the actual matched text from the normalized content
	actualMatched := content[idx : idx+len(oldStrNorm)]

	// Check if the actual matched text in the file has curly quotes
	hasCurlyDouble := strings.Contains(actualMatched, "\u201C") || strings.Contains(actualMatched, "\u201D")
	hasCurlySingle := strings.Contains(actualMatched, "\u2018") || strings.Contains(actualMatched, "\u2019")

	// Apply curly quote style to the new string
	result := newStr
	if hasCurlyDouble {
		result = curlyToStraightDouble(result)
		result = straightToCurlyDouble(result)
	}
	if hasCurlySingle {
		result = curlyToStraightSingle(result)
		result = straightToCurlySingle(result)
	}
	return result
}

// normalizeQuotes converts curly/smart quotes to straight ASCII quotes.
func normalizeQuotes(s string) string {
	s = strings.ReplaceAll(s, "\u201C", "\"")  // left double curly quote
	s = strings.ReplaceAll(s, "\u201D", "\"")  // right double curly quote
	s = strings.ReplaceAll(s, "\u2018", "'")   // left single curly quote
	s = strings.ReplaceAll(s, "\u2019", "'")   // right single curly quote
	return s
}

// curlyToStraightDouble converts curly double quotes to straight double quotes.
func curlyToStraightDouble(s string) string {
	s = strings.ReplaceAll(s, "\u201C", "\"")
	s = strings.ReplaceAll(s, "\u201D", "\"")
	return s
}

// curlyToStraightSingle converts curly single quotes to straight single quotes.
func curlyToStraightSingle(s string) string {
	s = strings.ReplaceAll(s, "\u2018", "'")
	s = strings.ReplaceAll(s, "\u2019", "'")
	return s
}

// straightToCurlyDouble converts straight double quotes to curly double quotes,
// using context (preceding character) to distinguish opening vs closing.
func straightToCurlyDouble(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '"' {
			// Determine if this is an opening or closing quote based on preceding character
			if i == 0 || isOpeningQuoteContext(runes[i-1]) {
				sb.WriteRune('\u201C') // opening double curly quote
			} else {
				sb.WriteRune('\u201D') // closing double curly quote
			}
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

// isOpeningQuoteContext returns true if the preceding character indicates
// this quote should be an opening curly quote. Matches upstream's
// isOpeningContext exactly.
func isOpeningQuoteContext(prev rune) bool {
	return prev == '(' || prev == '[' || prev == '{' ||
		prev == ' ' || prev == '\t' || prev == '\n' || prev == '\r' ||
		prev == '\u2014' || // em dash
		prev == '\u2013' // en dash
}

// straightToCurlySingle converts straight single quotes to curly single quotes,
// using context to distinguish opening (apostrophe) vs closing.
func straightToCurlySingle(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\'' {
			// Check for contraction: letter-'letter pattern (don't, can't, it's, etc.)
			// These use RIGHT curly (closing) apostrophe
			if i > 0 && i < len(runes)-1 {
				prev := runes[i-1]
				next := runes[i+1]
				if isLetter(prev) && isLetter(next) {
					sb.WriteRune('\u2019') // right single curly (apostrophe)
					continue
				}
			}
			// Opening apostrophe: preceded by whitespace, paren, etc.
			if i == 0 || isOpeningQuoteContext(runes[i-1]) {
				sb.WriteRune('\u2018') // left single curly quote
			} else {
				sb.WriteRune('\u2019') // right single curly quote
			}
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// stripTrailingWhitespace removes trailing whitespace from each line.
func stripTrailingWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// restoreCRLF replaces \n with \r\n only where not already preceded by \r.
func restoreCRLF(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/10)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			b.WriteString("\r\n")
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
