package tools

import (
	"fmt"
	"os"
	"strings"
)

// MultiEditTool applies multiple search/replace edits atomically.
// If any old_string is not found, the entire operation is aborted.
type MultiEditTool struct{}

func (*MultiEditTool) Name() string        { return "multi_edit" }
func (*MultiEditTool) Description() string { return "Apply multiple search/replace edits to a file atomically. If any edit fails, all are rolled back. You must read the file first with read_file before editing. Accepts a list of {old_string, new_string} pairs." }

func (*MultiEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"edits": map[string]any{
				"type":        "array",
				"description": "List of {old_string, new_string} edit operations.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old_string": map[string]any{
							"type":        "string",
							"description": "Exact text to find.",
						},
						"new_string": map[string]any{
							"type":        "string",
							"description": "Text to replace it with.",
						},
					},
					"required": []string{"old_string", "new_string"},
				},
			},
		},
		"required": []string{"path", "edits"},
	}
}

func (*MultiEditTool) CheckPermissions(params map[string]any) string { return "" }

func (*MultiEditTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		return ToolResult{Output: "Error: path is required", IsError: true}
	}

	editsRaw, ok := params["edits"]
	if !ok {
		return ToolResult{Output: "Error: edits is required", IsError: true}
	}
	editsSlice, ok := editsRaw.([]any)
	if !ok {
		return ToolResult{Output: "Error: edits must be an array", IsError: true}
	}
	if len(editsSlice) == 0 {
		return ToolResult{Output: "Error: edits must not be empty", IsError: true}
	}

	type edit struct{ old, new string }
	var edits []edit
	for i, e := range editsSlice {
		m, ok := e.(map[string]any)
		if !ok {
			return ToolResult{Output: fmt.Sprintf("Error: edit %d must be an object", i+1), IsError: true}
		}
		oldStr, _ := m["old_string"].(string)
		newStr, _ := m["new_string"].(string)
		if oldStr == "" {
			return ToolResult{Output: fmt.Sprintf("Error: edit %d: old_string must not be empty", i+1), IsError: true}
		}
		edits = append(edits, edit{old: oldStr, new: newStr})
	}

	fp := expandPath(pathStr)
	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", fp), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// Normalize CRLF
	content := string(data)
	hasCRLF := strings.Contains(content, "\r\n")
	if hasCRLF {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		for i := range edits {
			edits[i].old = strings.ReplaceAll(edits[i].old, "\r\n", "\n")
			edits[i].new = strings.ReplaceAll(edits[i].new, "\r\n", "\n")
		}
	}

	// Dry run: validate all edits
	for i, e := range edits {
		if !strings.Contains(content, e.old) {
			return ToolResult{
				Output: fmt.Sprintf("Error: edit %d failed: old_text not found: %q", i+1, truncate(e.old, 80)),
				IsError: true,
			}
		}
		content = strings.Replace(content, e.old, e.new, 1)
	}

	// Apply atomically
	if hasCRLF {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}

	return ToolResult{Output: fmt.Sprintf("Applied %d edits to %s", len(edits), fp)}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
