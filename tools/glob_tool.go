package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const maxResults = 100

// GlobTool finds files matching a glob pattern.
type GlobTool struct{}

func (*GlobTool) Name() string { return "glob" }
func (*GlobTool) Description() string {
	return "Fast file pattern matching tool that works with any codebase size. " +
		"Supports glob patterns like \"**/*.js\" or \"src/**/*.ts\". " +
		"Returns matching file paths sorted by modification time. " +
		"Use this tool when you need to find files by name patterns. " +
		"When you are doing an open ended search that may require multiple rounds of globbing and grepping, use the Agent tool instead."
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
			"type": map[string]any{
				"type":        "string",
				"description": "File type filter: 'file' (default, regular files only), 'dir' (directories only), 'all' (both files and dirs).",
				"enum":        []string{"file", "dir", "all"},
			},
		},
		"required": []string{"pattern"},
	}
}

func (*GlobTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

func (t *GlobTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	// Check context early
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: glob timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	pattern, _ := params["pattern"].(string)

	dirStr, _ := params["path"].(string)
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

	// Parse type filter (P2-11: glob type filter)
	typeFilter, _ := params["type"].(string)
	if typeFilter == "" {
		typeFilter = "file"
	}

	// SECURITY: Skip filesystem operations for UNC paths to prevent NTLM credential leaks.
	if isUncPath(dir) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", dirStr), IsError: true}
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

	var walkErr error
	var matches []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		// Check for context cancellation on every entry
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			// Skip hidden directories (dot-prefixed, excluding "." itself)
			if len(d.Name()) > 1 && d.Name()[0] == '.' {
				return filepath.SkipDir
			}
			if isIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			for _, excl := range excludes {
				if matched, _ := filepath.Match(excl, d.Name()); matched {
					return filepath.SkipDir
				}
			}
			// type filter: include dirs if "dir" or "all"
			if typeFilter == "dir" || typeFilter == "all" {
				matched, _ := doublestar.Match(pattern, rel)
				if matched {
					matches = append(matches, path)
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
		// type filter: include files if "file" or "all"
		if typeFilter != "file" && typeFilter != "all" {
			return nil
		}
		matched, _ := doublestar.Match(pattern, rel)
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return ToolResult{Output: fmt.Sprintf("Error: glob timed out scanning %s", dir), IsError: true}
		}
		walkErr = err
	}
	if walkErr != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", walkErr), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{Output: "No files matched."}
	}

	// Deduplicate matches (can happen when directory matches pattern
	// and walk continues into it with overlapping patterns)
	seen := make(map[string]bool)
	unique := matches[:0]
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	matches = unique

	// Sort by modification time (newest first)
	type fileInfo struct {
		path     string
		modified int64
	}
	files := make([]fileInfo, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err == nil {
			files = append(files, fileInfo{m, info.ModTime().Unix()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified < files[j].modified })

	// Output absolute paths (matching upstream)
	lines := make([]string, 0, len(files))
	for _, f := range files {
		absPath, err := filepath.Abs(f.path)
		if err != nil {
			absPath = f.path
		}
		lines = append(lines, absPath)
	}

	totalMatches := len(lines)
	if len(lines) > headLimit {
		lines = lines[:headLimit]
		lines = append(lines, fmt.Sprintf("(showing first %d of %d matches)", headLimit, totalMatches))
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}

func (t *GlobTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}
