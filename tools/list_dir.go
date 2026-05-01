package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListDirTool lists directory contents with optional recursion.
type ListDirTool struct{}

func (*ListDirTool) Name() string        { return "list_dir" }
func (*ListDirTool) Description() string {
	return "List directory contents. Shows files and subdirectories. " +
		"ALWAYS use list_dir to explore directories. Prefer over exec('ls') or exec('dir'). " +
		"Supports recursive listing with ignored directories (.git, node_modules, etc.)."
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
		},
		"required": []string{},
	}
}

func (*ListDirTool) CheckPermissions(params map[string]any) string { return "" }

func (*ListDirTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		pathStr = "."
	}
	recursive, _ := params["recursive"].(bool)

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
		entries, total = listDirRecursive(dir, maxEntries)
	} else {
		entries, total = listDirSimple(dir, maxEntries)
	}

	if len(entries) == 0 && total == 0 {
		return ToolResult{Output: fmt.Sprintf("Directory %s is empty", pathStr)}
	}

	result := strings.Join(entries, "\n")
	if total > maxEntries {
		result += fmt.Sprintf("\n\n(truncated, showing first %d of %d entries)", maxEntries, total)
	}

	return ToolResult{Output: result}
}

func listDirSimple(dir string, maxEntries int) ([]string, int) {
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
			total++
			if len(entries) >= maxEntries {
				continue
			}
			fullPath := filepath.Join(dir, name)
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			if info.IsDir() {
				entries = append(entries, name+"/")
			} else {
				entries = append(entries, name)
			}
		}
		if len(names) < 100 {
			break
		}
	}
	return entries, total
}

func listDirRecursive(root string, maxEntries int) ([]string, int) {
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
				entries = append(entries, rel+"/")
			} else {
				entries = append(entries, rel)
			}
		}
		return nil
	})
	return entries, total
}

func isIgnoredDir(name string) bool {
	ignored := map[string]bool{
		".git":         true,
		"node_modules": true,
		"__pycache__":  true,
		".venv":        true,
		"venv":         true,
		"dist":         true,
		"build":        true,
		".DS_Store":    true,
		".tox":         true,
		".mypy_cache":  true,
		".pytest_cache":true,
		".ruff_cache":  true,
		".coverage":    true,
		"htmlcov":      true,
	}
	return ignored[name]
}
