package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ListDirTool lists directory contents with optional recursion.
type ListDirTool struct{}

func (*ListDirTool) Name() string        { return "list_dir" }
func (*ListDirTool) Description() string {
	return "List directory contents with file details. " +
		"ALWAYS use list_dir to explore directories. Prefer over exec('ls') or exec('dir'). " +
		"Shows file type (file/directory/symlink), size in bytes, and last modification time. " +
		"Supports recursive listing with ignored directories (.git, node_modules, etc.). " +
		"Use show_hidden=true to include dotfiles and hidden entries."
}

func (*ListDirTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory path to list (default: current directory).",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Recursively list subdirectories (default: false).",
			},
			"max_entries": map[string]any{
				"type":        "integer",
				"description": "Maximum number of entries to return (default: 200).",
			},
			"show_hidden": map[string]any{
				"type":        "boolean",
				"description": "Include hidden entries (dotfiles/dotdirs like .gitignore). Default: false.",
			},
		},
		"required": []string{},
	}
}

func (*ListDirTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *ListDirTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: list_dir timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		pathStr = "."
	}
	recursive, _ := params["recursive"].(bool)
	showHidden, _ := params["show_hidden"].(bool)

	maxEntries := 200
	if me, ok := params["max_entries"]; ok {
		switch v := me.(type) {
		case float64:
			maxEntries = int(v)
		case int:
			maxEntries = v
		}
	}

	dir := expandPath(pathStr)
	info, err := os.Stat(dir)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	if !info.IsDir() {
		return ToolResult{Output: fmt.Sprintf("Error: not a directory: %s", dir), IsError: true}
	}

	var entries []string
	total := 0
	if recursive {
		entries, total = listDirRecursive(dir, maxEntries, showHidden)
	} else {
		entries, total = listDirSimple(dir, maxEntries, showHidden)
	}

	if len(entries) == 0 && total == 0 {
		return ToolResult{Output: fmt.Sprintf("Directory %s is empty", pathStr)}
	}

	result := strings.Join(entries, "\n")
	if total > 0 && maxEntries > 0 && total > maxEntries {
		result += fmt.Sprintf("\n\n(truncated, showing first %d of %d entries)", maxEntries, total)
	}

	return ToolResult{Output: result}
}

func (t *ListDirTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

func listDirSimple(dir string, maxEntries int, showHidden bool) ([]string, int) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, 0
	}
	defer f.Close()

	var entries []string
	total := 0
	for {
		names, err := f.Readdirnames(100)
		if err != nil {
			break
		}
		for _, name := range names {
			// Skip hidden entries unless showHidden is true
			if !showHidden && strings.HasPrefix(name, ".") {
				continue
			}
			total++
			if maxEntries > 0 && len(entries) >= maxEntries {
				continue
			}
			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			if info.IsDir() {
				entries = append(entries, fmt.Sprintf("%-40s  DIR  %s", name+"/", info.ModTime().Format(time.RFC3339)))
			} else {
				entries = append(entries, fmt.Sprintf("%-40s  %8d  %s", name, info.Size(), info.ModTime().Format(time.RFC3339)))
			}
		}
		if len(names) < 100 {
			break
		}
	}
	return entries, total
}

func listDirRecursive(root string, maxEntries int, showHidden bool) ([]string, int) {
	var entries []string
	total := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		// Skip hidden entries unless showHidden is true
		baseName := filepath.Base(path)
		if !showHidden && strings.HasPrefix(baseName, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip ignored directories
		if info.IsDir() {
			dirName := filepath.Base(path)
			if isIgnoredDir(dirName) {
				return filepath.SkipDir
			}
		}
		total++
		if len(entries) < maxEntries {
			if info.IsDir() {
				entries = append(entries, fmt.Sprintf("%-50s  DIR  %s", rel+"/", info.ModTime().Format(time.RFC3339)))
			} else {
				entries = append(entries, fmt.Sprintf("%-50s  %8d  %s", rel, info.Size(), info.ModTime().Format(time.RFC3339)))
			}
		}
		return nil
	})
	return entries, total
}

func isIgnoredDir(name string) bool {
	ignored := map[string]bool{
		".git":         true,
		".svn":         true,
		".hg":          true,
		".bzr":         true,
		".jj":          true,
		".sl":          true,
		".claude":      true,
		"node_modules": true,
		"__pycache__":  true,
		".venv":        true,
		"venv":         true,
		".tox":         true,
		".mypy_cache":  true,
		".pytest_cache":true,
		".ruff_cache":  true,
		".coverage":    true,
		"htmlcov":      true,
		".cargo":       true,
		".rustup":      true,
		"target":       true,
		".gradle":      true,
		".dart_tool":   true,
		"dist":         true,
		"build":        true,
		"out":          true,
		".DS_Store":    true,
	}
	return ignored[name]
}
