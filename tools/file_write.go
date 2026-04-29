package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteTool writes content to a file, creating parent directories as needed.
type FileWriteTool struct{}

func (*FileWriteTool) Name() string { return "write_file" }
func (*FileWriteTool) Description() string {
	return "Write content to a file. Creates parent directories if they don't exist. Overwrites if the file already exists. You must read the file first with read_file before writing to an existing file."
}

func (*FileWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (*FileWriteTool) CheckPermissions(params map[string]any) string { return "" }

func (*FileWriteTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	content, _ := params["content"].(string)

	const maxWriteSize = 10 * 1024 * 1024 // 10MB
	if len(content) > maxWriteSize {
		return ToolResult{Output: fmt.Sprintf("Error: content too large (%d bytes, max %d bytes)", len(content), maxWriteSize), IsError: true}
	}

	fp := expandPath(pathStr)
	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error creating directory: %v", err), IsError: true}
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Wrote %d chars to %s", len(content), fp)}
}
