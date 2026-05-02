package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

const maxResults = 100

// GlobTool finds files matching a glob pattern.
type GlobTool struct{}

func (*GlobTool) Name() string        { return "glob" }
func (*GlobTool) Description() string {
	return "Fast file pattern matching tool — use it liberally rather than guessing file paths. " +
		"ALWAYS use glob to find files by name pattern. NEVER use exec with 'find' command. " +
		"Returns matching file paths sorted by modification time. " +
		"Supports glob patterns like '**/*.go' or 'src/**/*.ts'."
}

func (*GlobTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern (e.g. '**/*.py'). Patterns without '**/' are auto-prefixed.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in (default: current directory).",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 100).",
			},
			"excludes": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Glob patterns to exclude (files/dirs matching any are skipped, e.g. ['*.test.go', 'vendor']).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (*GlobTool) CheckPermissions(params map[string]any) string { return "" }

func (*GlobTool) Execute(params map[string]any) ToolResult {
	pattern, _ := params["pattern"].(string)

	// Support path (official) and directory (legacy alias)
	dirStr, _ := params["path"].(string)
	if dirStr == "" {
		dirStr, _ = params["directory"].(string)
	}
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

	// Parse excludes
	var excludes []string
	if ex, ok := params["excludes"]; ok {
		switch v := ex.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					excludes = append(excludes, s)
				}
			}
		case []string:
			excludes = v
		}
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
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if isIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			for _, excl := range excludes {
				if matched, _ := filepath.Match(excl, d.Name()); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}
		for _, excl := range excludes {
			if matched, _ := filepath.Match(excl, rel); matched {
				return nil
			}
			if matched, _ := filepath.Match(excl, d.Name()); matched {
				return nil
			}
		}
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

	// Sort by modification time (newest first) and collect metadata
	type fileInfo struct {
		path     string
		size     int64
		modified int64
	}
	files := make([]fileInfo, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil {
			files = append(files, fileInfo{m, info.Size(), info.ModTime().Unix()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified < files[j].modified })

	lines := make([]string, 0, len(files))
	for _, f := range files {
		modStr := time.Unix(f.modified, 0).Format("2006-01-02 15:04")
		lines = append(lines, fmt.Sprintf("%s (%d bytes, modified %s)", f.path, f.size, modStr))
	}

	totalMatches := len(lines)
	if len(lines) > headLimit {
		lines = lines[:headLimit]
		lines = append(lines, fmt.Sprintf("(showing first %d of %d matches)", headLimit, totalMatches))
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}
