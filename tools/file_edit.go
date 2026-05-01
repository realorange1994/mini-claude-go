package tools

import (
	"fmt"
	"os"
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
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to find (must be unique in the file).",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Text to replace it with.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences (default: false).",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (*FileEditTool) CheckPermissions(params map[string]any) string { return "" }

func (*FileEditTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	oldStr, _ := params["old_string"].(string)
	newStr, _ := params["new_string"].(string)
	replaceAll, _ := params["replace_all"].(bool)

	fp := expandPath(pathStr)

	if oldStr == "" {
		return ToolResult{Output: "Error: old_string must not be empty", IsError: true}
	}

	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", pathStr), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// Normalize CRLF
	content := string(data)
	hasCRLF := strings.Contains(content, "\r\n")
	if hasCRLF {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		oldStr = strings.ReplaceAll(oldStr, "\r\n", "\n")
		newStr = strings.ReplaceAll(newStr, "\r\n", "\n")
	}

	count := strings.Count(content, oldStr)
	if count == 0 {
		return ToolResult{Output: fmt.Sprintf("Error: old_text not found in %s. Verify the file content.", pathStr), IsError: true}
	}
	if count > 1 && !replaceAll {
		return ToolResult{
			Output: fmt.Sprintf("Warning: old_text appears %d times. Provide more context or set replace_all=true.", count),
			IsError: true,
		}
	}

	if replaceAll {
		content = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		content = strings.Replace(content, oldStr, newStr, 1)
	}

	// Restore CRLF if original had it - only replace \n not preceded by \r
	if hasCRLF {
		content = restoreCRLF(content)
	}

	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}

	return ToolResult{Output: fmt.Sprintf("Successfully edited %s", fp)}
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
