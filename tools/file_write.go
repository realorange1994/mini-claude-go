package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileWriteTool writes content to a file, creating parent directories as needed.
// It enforces read-before-write validation and concurrent modification detection.
type FileWriteTool struct {
	registry *Registry // nil if tracker is not available
}

func NewFileWriteTool(registry *Registry) *FileWriteTool {
	return &FileWriteTool{registry: registry}
}

func (*FileWriteTool) Name() string { return "write_file" }
func (*FileWriteTool) Description() string {
	return "Writes a file to the local filesystem.\n\n" +
		"Usage:\n" +
		"- This tool will overwrite the existing file if there is one at the provided path.\n" +
		"- If this is an existing file, you MUST use the read_file tool first to read the file's contents. This tool will fail if you did not read the file first.\n" +
		"- Prefer the edit_file tool for modifying existing files — it only sends the diff. Only use this tool to create new files or for complete rewrites.\n" +
		"- NEVER create documentation files (*.md) or README files unless explicitly requested by the User.\n" +
		"- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked."
}

func (*FileWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to write (must be absolute, not relative)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"file_path", "content"},
	}
}

func (*FileWriteTool) CheckPermissions(params map[string]any) string { return "" }

func (w *FileWriteTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		pathStr, _ = params["path"].(string)
	}
	if pathStr == "" {
		return ToolResult{Output: "Error: file_path is required", IsError: true}
	}
	content, _ := params["content"].(string)

	const maxWriteSize = 10 * 1024 * 1024 // 10MB
	if len(content) > maxWriteSize {
		return ToolResult{Output: fmt.Sprintf("Error: content too large (%d bytes, max %d bytes)", len(content), maxWriteSize), IsError: true}
	}

	fp := expandPath(pathStr)

	// SECURITY: Block UNC paths before any filesystem I/O to prevent NTLM credential leaks.
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

	// Read-before-write validation and concurrent modification detection.
	if w.registry != nil {
		if staleMsg := w.registry.CheckFileStale(fp); staleMsg != "" {
			return ToolResult{Output: staleMsg, IsError: true}
		}
	}

	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error creating directory: %v", err), IsError: true}
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}
	// Update registry so subsequent writes are allowed without re-reading
	if w.registry != nil {
		w.registry.MarkFileReadWithContent(fp, content)
	}
	return ToolResult{Output: fmt.Sprintf("Wrote %d chars to %s", len(content), fp)}
}
