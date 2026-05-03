package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileEditTool edits a file by replacing an exact string with a new string.
type FileEditTool struct{}

func (*FileEditTool) Name() string { return "edit_file" }
func (*FileEditTool) Description() string {
	return "Edit a file by replacing an exact string with a new string. " +
		"You MUST use read_file to read the file at least once before editing. " +
		"ALWAYS prefer edit_file for modifying existing files — it only sends the diff. " +
		"The edit will FAIL if old_string is not unique in the file. Provide enough context to uniquely match. " +
		"Use replace_all to change every instance of old_string."
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

func (*FileEditTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		pathStr, _ = params["path"].(string)
	}
	oldStr, _ := params["old_string"].(string)
	newStr, _ := params["new_string"].(string)
	replaceAll, _ := params["replace_all"].(bool)

	fp := expandPath(pathStr)

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
			return ToolResult{Output: "Error: cannot create new file - file already exists with content", IsError: true}
		}
		dir := filepath.Dir(fp)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
		}
		if err := os.WriteFile(fp, []byte(newStr), 0o644); err != nil {
			return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
		}
		return ToolResult{Output: fmt.Sprintf("Successfully created %s", fp)}
	}

	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", pathStr), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// 1 GiB file size guard (matching official Claude Code behavior):
	// Prevents OOM from loading huge files into memory for string replacement.
	const maxEditSize = 1 << 30 // 1 GiB
	if len(data) > maxEditSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (%d bytes, max %d bytes). Use offset/limit to read portions.", len(data), maxEditSize), IsError: true}
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
	contentNorm = applyReplacement(contentNorm, oldStrNorm, newStrNorm, replaceAll)

	// Restore original quote style — pass original (pre-normalized) content
	// so curly quotes can be detected in the actual file content
	contentNorm = preserveQuoteStyle(contentNorm, content, oldStr, newStr, replaceAll)

	// Restore CRLF
	if hasCRLF {
		contentNorm = restoreCRLF(contentNorm)
	}

	if err := os.WriteFile(fp, []byte(contentNorm), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
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
// If the matched text in the original file used curly quotes, the replacement
// also uses curly quotes to match the surrounding context style.
func preserveQuoteStyle(content, contentOrig, oldStr, newStr string, replaceAll bool) string {
	// Check if the file actually contains curly quotes
	hasCurlyDouble := strings.Contains(contentOrig, "\u201C") || strings.Contains(contentOrig, "\u201D")
	hasCurlySingle := strings.Contains(contentOrig, "\u2018") || strings.Contains(contentOrig, "\u2019")
	if !hasCurlyDouble && !hasCurlySingle {
		return content
	}

	// Detect quote style used in oldStr
	oldHasCurlyDouble := strings.Contains(oldStr, "\u201C") || strings.Contains(oldStr, "\u201D")
	oldHasCurlySingle := strings.Contains(oldStr, "\u2018") || strings.Contains(oldStr, "\u2019")

	if oldHasCurlyDouble {
		content = curlyToStraightDouble(content)
		content = straightToCurlyDouble(content)
	}
	if oldHasCurlySingle {
		content = curlyToStraightSingle(content)
		content = straightToCurlySingle(content)
	}
	return content
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

// straightToCurlyDouble converts straight double quotes to curly double quotes.
func straightToCurlyDouble(s string) string {
	s = strings.ReplaceAll(s, "\"", "\u201C")
	return s
}

// straightToCurlySingle converts straight single quotes to curly single quotes.
func straightToCurlySingle(s string) string {
	s = strings.ReplaceAll(s, "'", "\u2019")
	return s
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
