package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const maxResults = 500

// GlobTool finds files matching a glob pattern.
type GlobTool struct{}

func (*GlobTool) Name() string        { return "glob" }
func (*GlobTool) Description() string { return "Find files matching a glob pattern. Returns matching file paths sorted by modification time." }

func (*GlobTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern (e.g. '**/*.py'). Patterns without '**/' are auto-prefixed.",
			},
			"directory": map[string]any{
				"type":        "string",
				"description": "Directory to search in (default: current directory).",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 500).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (*GlobTool) CheckPermissions(params map[string]any) string { return "" }

func (*GlobTool) Execute(params map[string]any) ToolResult {
	pattern, _ := params["pattern"].(string)

	dirStr, _ := params["directory"].(string)
	if dirStr == "" {
		dirStr = "."
	}
	dir := expandPath(dirStr)

	headLimit := maxResults
	if hl, ok := params["head_limit"]; ok {
		switch v := hl.(type) {
		case float64:
			headLimit = int(v)
		case int:
			headLimit = v
		}
	}
	if headLimit <= 0 {
		headLimit = maxResults
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ToolResult{Output: fmt.Sprintf("Error: directory not found: %s", dir), IsError: true}
	}

	// Auto-prefix with **/ if pattern has no slash
	hasDoubleStar := strings.HasPrefix(pattern, "**/")
	if !strings.Contains(pattern, "/") && !hasDoubleStar {
		pattern = "**/" + pattern
		hasDoubleStar = true
	}

	var matches []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		matched, _ := doublestar.Match(pattern, rel)
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{Output: "No files matched."}
	}

	// Sort by modification time (newest first) -- need to re-stat
	type fileInfo struct {
		path string
		mt   int64
	}
	files := make([]fileInfo, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil {
			files = append(files, fileInfo{m, info.ModTime().Unix()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mt > files[j].mt })

	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.path)
	}

	if len(paths) > headLimit {
		paths = paths[:headLimit]
		paths = append(paths, fmt.Sprintf("(showing first %d of %d matches)", headLimit, len(matches)))
	}

	return ToolResult{Output: strings.Join(paths, "\n")}
}
