package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// FileOpsTool provides file operations (mkdir, rm, mv, cp, chmod, ln).
type FileOpsTool struct{}

func (*FileOpsTool) Name() string        { return "fileops" }
func (*FileOpsTool) Description() string { return "File and directory operations. Supports mkdir, rm, rmrf (recursive remove), mv, cp, cpdir (recursive copy), chmod, and ln (symbolic/hard links)." }

func (*FileOpsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation: mkdir, rm, rmrf, mv, cp, cpdir, chmod, ln",
				"enum":        []string{"mkdir", "rm", "rmrf", "mv", "cp", "cpdir", "chmod", "ln"},
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Path for the operation.",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "Destination path (for mv, cp, ln).",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Permission mode (for mkdir/chmod, e.g. 755, 644).",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Create parent directories (for mkdir).",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force operation (for ln).",
			},
			"symbolic": map[string]any{
				"type":        "boolean",
				"description": "Create symbolic link instead of hard link (for ln, default: true).",
			},
		},
		"required": []string{"operation", "path"},
	}
}

func (*FileOpsTool) CheckPermissions(params map[string]any) string { return "" }

func (*FileOpsTool) Execute(params map[string]any) ToolResult {
	operation, _ := params["operation"].(string)
	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		return ToolResult{Output: "Error: path is required", IsError: true}
	}

	fp := expandPath(pathStr)

	switch operation {
	case "mkdir":
		return opMkdir(fp, params)
	case "rm":
		return opRemove(fp)
	case "rmrf":
		return opRemoveAll(fp)
	case "mv":
		return opMove(fp, params)
	case "cp":
		return opCopy(fp, params)
	case "cpdir":
		return opCopyDir(fp, params)
	case "chmod":
		return opChmod(fp, params)
	case "ln":
		return opLink(fp, params)
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation: %s", operation), IsError: true}
	}
}

func opMkdir(path string, params map[string]any) ToolResult {
	mode := os.FileMode(0o755)
	if m, ok := params["mode"].(string); ok && m != "" {
		var mval int
		_, err := fmt.Sscanf(m, "%o", &mval)
		if err == nil {
			mode = os.FileMode(mval)
		}
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error creating directory: %v", err), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Created directory: %s", path)}
}

func opRemove(path string) ToolResult {
	if err := os.Remove(path); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error removing: %v", err), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Removed: %s", path)}
}

func opRemoveAll(path string) ToolResult {
	if err := os.RemoveAll(path); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error removing recursively: %v", err), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Removed recursively: %s", path)}
}

func opMove(src string, params map[string]any) ToolResult {
	destStr, _ := params["destination"].(string)
	if destStr == "" {
		return ToolResult{Output: "Error: destination is required for mv", IsError: true}
	}
	dest := expandPath(destStr)
	if err := os.Rename(src, dest); err != nil {
		// Cross-device: fall back to copy + remove
		if err2 := copyPath(src, dest); err2 != nil {
			return ToolResult{Output: fmt.Sprintf("Error moving: %v", err), IsError: true}
		}
		os.RemoveAll(src)
	}
	return ToolResult{Output: fmt.Sprintf("Moved %s to %s", src, dest)}
}

func opCopy(src string, params map[string]any) ToolResult {
	destStr, _ := params["destination"].(string)
	if destStr == "" {
		return ToolResult{Output: "Error: destination is required for cp", IsError: true}
	}
	dest := expandPath(destStr)
	if err := copyFile(src, dest); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error copying: %v", err), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Copied %s to %s", src, dest)}
}

func opCopyDir(src string, params map[string]any) ToolResult {
	destStr, _ := params["destination"].(string)
	if destStr == "" {
		return ToolResult{Output: "Error: destination is required for cpdir", IsError: true}
	}
	dest := expandPath(destStr)
	if err := copyPath(src, dest); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error copying directory: %v", err), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Copied directory %s to %s", src, dest)}
}

func opChmod(path string, params map[string]any) ToolResult {
	modeStr, _ := params["mode"].(string)
	mode := os.FileMode(0o644) // default mode if not specified
	if modeStr != "" {
		var mval int
		_, err := fmt.Sscanf(modeStr, "%o", &mval)
		if err != nil {
			return ToolResult{Output: fmt.Sprintf("Error: invalid mode: %s", modeStr), IsError: true}
		}
		mode = os.FileMode(mval)
	}
	if err := os.Chmod(path, mode); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error changing mode: %v", err), IsError: true}
	}
	if modeStr == "" {
		return ToolResult{Output: fmt.Sprintf("Changed mode of %s to %s (default)", path, "0644")}
	}
	return ToolResult{Output: fmt.Sprintf("Changed mode of %s to %s", path, modeStr)}
}

func opLink(path string, params map[string]any) ToolResult {
	destStr, _ := params["destination"].(string)
	if destStr == "" {
		return ToolResult{Output: "Error: destination is required for ln", IsError: true}
	}
	dest := expandPath(destStr)
	symlink, _ := params["symbolic"].(bool)
	// Default to symlink
	if _, ok := params["symbolic"]; !ok {
		symlink = true
	}
	force, _ := params["force"].(bool)

	// Remove existing destination if force
	if force {
		os.Remove(dest)
	}

	var err error
	if symlink {
		err = os.Symlink(path, dest)
	} else {
		err = os.Link(path, dest)
	}
	if err != nil {
		// On Windows, symlink may fail for non-admin users
		if runtime.GOOS == "windows" && symlink {
			return ToolResult{Output: fmt.Sprintf("Error creating symlink: %v (may require administrator privileges on Windows)", err), IsError: true}
		}
		return ToolResult{Output: fmt.Sprintf("Error creating link: %v", err), IsError: true}
	}
	linkType := "symbolic link"
	if !symlink {
		linkType = "hard link"
	}
	return ToolResult{Output: fmt.Sprintf("Created %s %s -> %s", linkType, dest, path)}
}

// copyFile copies a single file.
func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// copyPath recursively copies a directory tree.
func copyPath(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dest)
	}
	if err := os.MkdirAll(dest, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyPath(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}
