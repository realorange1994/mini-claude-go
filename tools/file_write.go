package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// WriteFileAtomically writes content to a file using a temp-file-then-rename pattern.
// This prevents partial/corrupt files if the process crashes mid-write.
// Callers should clean up the target file after a successful write if they need
// to ensure no stale .tmp files remain (the rename handles this automatically).
func WriteFileAtomically(path string, content []byte) error {
	tmpName := path + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmpName, content, 0o644); err != nil {
		os.Remove(tmpName) // cleanup on write failure
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName) // cleanup on rename failure
		// On Windows, rename may fail for locked files; fall back to direct write
		return os.WriteFile(path, content, 0o644)
	}
	return nil
}

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
		"- To modify an existing file: use read_file first, then use edit_file (preferred for small changes) or write_file (for complete rewrites).\n" +
		"- To create a new file: write_file works directly — no read needed.\n" +
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

func (*FileWriteTool) CheckPermissions(params map[string]any) PermissionResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		return PermissionResultPassthrough()
	}
	return CheckPathSafetyForAutoEdit(pathStr)
}

func (w *FileWriteTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: write_file timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	pathStr, _ := params["file_path"].(string)
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
	if err := WriteFileAtomically(fp, []byte(content)); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}
	// Update registry so subsequent writes are allowed without re-reading
	if w.registry != nil {
		w.registry.MarkFileReadWithContent(fp, content)
	}

	out := fmt.Sprintf("Wrote %d chars to %s", len(content), fp)
	// Warn on large writes (P2-14: large file confirmation).
	// Upstream asks user to confirm files > 1MB. In REPL mode, the model
	// should present this warning to the user for confirmation.
	const warnThreshold = 1024 * 1024 // 1MB
	if len(content) > warnThreshold {
		sizeMB := float64(len(content)) / (1024 * 1024)
		out += fmt.Sprintf("\n[WARN] Large file written (%.1f MB). Confirm with the user before proceeding.", sizeMB)
	}
	return ToolResult{Output: out}
}

func (w *FileWriteTool) Execute(params map[string]any) ToolResult {
	return w.ExecuteContext(context.Background(), params)
}
