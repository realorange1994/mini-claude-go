package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const maxGrepMatches = 250
const maxGrepLineLen = 500

// GrepTool searches file contents using regex. Uses ripgrep if available, otherwise Go regexp.
type GrepTool struct{}

func (*GrepTool) Name() string { return "grep" }
func (*GrepTool) Description() string {
	return "Search file contents using regex in a codebase. Uses ripgrep (rg) if available, otherwise falls back to Go regexp. " +
		"Supports glob and language type filters, context lines, and output modes. " +
		"For advanced ripgrep features (multiline, PCRE2, JSON output, etc.) use the exec tool to call rg directly."
}

func (*GrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for. For literal text, use fixed_strings=true instead of escaping special regex characters.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in. Defaults to current directory. To avoid scanning too many files, use max_depth to limit directory traversal.",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob to filter files (e.g. '*.py'). Only files matching this pattern are searched.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Language type filter. Common values: go, py, js, ts, rust, java, sh, yaml, json, md, html, css.",
			},
			"ignore_case": map[string]any{
				"type":        "boolean",
				"description": "Case insensitive search (default: false).",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "Alias for ignore_case. Case insensitive search (default: false).",
			},
			"fixed_strings": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as a literal string, not regex (default: false).",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": "Output mode: 'content' (default) shows matching lines, 'files_with_matches' shows file paths, 'count' shows per-file match counts.",
			},
			"context": map[string]any{
				"type":        "integer",
				"description": "Lines of context before and after each match (default: 0).",
			},
			"context_before": map[string]any{
				"type":        "integer",
				"description": "Lines of context before each match (rg only, default: 0). Overrides context if set.",
			},
			"context_after": map[string]any{
				"type":        "integer",
				"description": "Lines of context after each match (rg only, default: 0). Overrides context if set.",
			},
			"max_depth": map[string]any{
				"type":        "integer",
				"description": "Maximum directory depth to search. Limits how many levels of subdirectories to traverse. Useful for avoiding scanning too many files (default: unlimited).",
			},
			"max_filesize": map[string]any{
				"type":        "string",
				"description": "Maximum file size to search (e.g. '1M', '500K', '100B'). Files larger than this are skipped. Only applies when ripgrep is available.",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 250).",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip the first N results for pagination (default: 0).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (*GrepTool) CheckPermissions(params map[string]any) string { return "" }

func (*GrepTool) Execute(params map[string]any) ToolResult {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return ToolResult{Output: "Error: pattern is required", IsError: true}
	}

	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		pathStr = "."
	}
	searchPath := expandPath(pathStr)

	include, _ := params["glob"].(string)
	typeFilter, _ := params["type"].(string)

	// Support both ignore_case and case_insensitive
	caseInsensitive, _ := params["ignore_case"].(bool)
	if !caseInsensitive {
		if ci, _ := params["case_insensitive"].(bool); ci {
			caseInsensitive = true
		}
	}
	fixedStrings, _ := params["fixed_strings"].(bool)
	countMatches, _ := params["count_matches"].(bool)
	outputMode, _ := params["output_mode"].(string)
	if outputMode == "" {
		outputMode = "content"
	}
	headLimit := maxGrepMatches
	if hl, ok := params["head_limit"]; ok {
		switch v := hl.(type) {
		case float64:
			headLimit = int(v)
		case int:
			headLimit = v
		}
	}
	if headLimit <= 0 {
		headLimit = maxGrepMatches
	}

	// Parse max_depth
	maxDepth := 0
	if md, ok := params["max_depth"]; ok {
		switch v := md.(type) {
		case float64:
			maxDepth = int(v)
		case int:
			maxDepth = v
		}
	}

	// Parse max_filesize
	maxFilesize, _ := params["max_filesize"].(string)

	// Parse offset
	offset := 0
	if o, ok := params["offset"]; ok {
		switch v := o.(type) {
		case float64:
			offset = int(v)
		case int:
			offset = v
		}
	}

	// Parse context params (combined takes priority if before/after not set)
	ctxBefore := parseIntParam(params, "context_before")
	ctxAfter := parseIntParam(params, "context_after")
	ctxCombined := parseIntParam(params, "context")
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

	if _, err := exec.LookPath("rg"); err == nil {
		return rgSearch(pattern, searchPath, include, typeFilter, caseInsensitive, fixedStrings, outputMode, ctxBefore, ctxAfter, headLimit, offset, maxDepth, maxFilesize)
	}
	return goSearch(pattern, searchPath, include, typeFilter, caseInsensitive, fixedStrings, outputMode, headLimit, offset, ctxCombined, countMatches, maxDepth)
}

func parseIntParam(params map[string]any, key string) int {
	if v, ok := params[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		}
	}
	return 0
}

var typeMap = map[string][]string{
	"py":     {".py", ".pyi"},
	"python": {".py", ".pyi"},
	"js":     {".js", ".jsx", ".mjs", ".cjs"},
	"ts":     {".ts", ".tsx", ".mts", ".cts"},
	"go":     {".go"},
	"rust":   {".rs"},
	"java":   {".java"},
	"sh":     {".sh", ".bash"},
	"yaml":   {".yaml", ".yml"},
	"json":   {".json"},
	"md":     {".md", ".mdx"},
	"html":   {".html", ".htm"},
	"css":    {".css", ".scss", ".sass"},
}

func rgSearch(pattern, path, include, typeFilter string, caseInsensitive, fixedStrings bool, outputMode string, ctxBefore, ctxAfter, headLimit, offset int, maxDepth int, maxFilesize string) ToolResult {
	args := []string{"--no-heading", "--line-number"}

	switch outputMode {
	case "files_with_matches":
		args = append(args, "--files-with-matches")
	case "count":
		args = append(args, "--count")
	}

	if caseInsensitive {
		args = append(args, "-i")
	}
	if fixedStrings {
		args = append(args, "-F")
	}
	if ctxBefore > 0 {
		args = append(args, "-B", fmt.Sprintf("%d", ctxBefore))
	}
	if ctxAfter > 0 {
		args = append(args, "-A", fmt.Sprintf("%d", ctxAfter))
	}

	// Limit directory traversal depth
	if maxDepth > 0 {
		args = append(args, "--max-depth", fmt.Sprintf("%d", maxDepth))
	}
	// Skip large files
	if maxFilesize != "" {
		args = append(args, "--max-filesize", maxFilesize)
	}

	args = append(args, "-m", fmt.Sprintf("%d", headLimit))
	args = append(args, pattern, path)

	if include != "" {
		args = append(args, "--glob", include)
	}
	if typeFilter != "" {
		if exts, ok := typeMap[strings.ToLower(typeFilter)]; ok {
			for _, e := range exts {
				glob := e
				if !strings.HasPrefix(glob, "*") {
					glob = "*" + glob
				}
				args = append(args, "--type-add", "mytype:"+glob)
			}
			args = append(args, "--type", "mytype")
		}
	}

	cmd := exec.Command("rg", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if output == "" {
		// rg exits with code 1 when no matches found -- not a real error
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return ToolResult{Output: "No matches found."}
			}
			return ToolResult{Output: fmt.Sprintf("Error running rg: %v", err), IsError: true}
		}
		return ToolResult{Output: "No matches found."}
	}

	lines := strings.Split(output, "\n")

	// Apply offset
	if offset > 0 && offset < len(lines) {
		lines = lines[offset:]
	}
	if len(lines) > headLimit {
		lines = lines[:headLimit]
		lines = append(lines, fmt.Sprintf("(showing first %d matches, truncated)", headLimit))
	}
	return ToolResult{Output: strings.Join(lines, "\n")}
}

func goSearch(pattern, path, include, typeFilter string, caseInsensitive, fixedStrings bool, outputMode string, headLimit, offset, ctxLines int, countMatches bool, maxDepth int) ToolResult {
	searchPattern := pattern
	if caseInsensitive {
		searchPattern = "(?i)" + searchPattern
	}
	if fixedStrings {
		searchPattern = regexp.QuoteMeta(searchPattern)
	}

	re, err := regexp.Compile(searchPattern)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Invalid regex: %v", err), IsError: true}
	}

	var allowedExts []string
	if typeFilter != "" {
		if exts, ok := typeMap[strings.ToLower(typeFilter)]; ok {
			allowedExts = exts
		}
	}

	var files []string
	info, err := os.Stat(path)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	if info.Mode().IsRegular() {
		files = []string{path}
	} else {
		baseDepth := strings.Count(filepath.Clean(path), string(filepath.Separator))
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				// Enforce max_depth
				if maxDepth > 0 {
					curDepth := strings.Count(filepath.Clean(p), string(filepath.Separator)) - baseDepth
					if curDepth >= maxDepth {
						return filepath.SkipDir
					}
				}
				if isIgnoredDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if include != "" {
				matched, _ := filepath.Match(include, d.Name())
				if !matched {
					return nil
				}
			}
			if len(allowedExts) > 0 {
				ext := strings.ToLower(filepath.Ext(p))
				allowed := false
				for _, e := range allowedExts {
					if ext == e {
						allowed = true
						break
					}
				}
				if !allowed {
					return nil
				}
			}
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".exe" || ext == ".dll" || ext == ".so" || ext == ".bin" {
				return nil
			}
			files = append(files, p)
			return nil
		})
	}

	filesSearched := len(files)

	switch outputMode {
	case "files_with_matches":
		return goSearchFilesOnly(re, files, headLimit, offset, filesSearched)
	case "count":
		return goSearchCount(re, files, filesSearched)
	default:
		return goSearchContent(re, files, headLimit, offset, ctxLines, countMatches, filesSearched)
	}
}

// truncateLine truncates a line to maxGrepLineLen characters.
func truncateLine(line string) string {
	if len(line) <= maxGrepLineLen {
		return line
	}
	return line[:maxGrepLineLen] + "..."
}

func goSearchContent(re *regexp.Regexp, files []string, headLimit, offset, ctxLines int, countMatches bool, filesSearched int) ToolResult {
	var matches []string
	skipped := 0
	totalMatchCount := 0
	for _, fp := range files {
		data, err := os.ReadFile(fp)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			loc := re.FindStringIndex(trimmed)
			if loc == nil {
				continue
			}
			totalMatchCount++
			if skipped < offset {
				skipped++
				continue
			}
			relPath, _ := filepath.Rel(".", fp)
			if countMatches {
				count := len(re.FindAllStringIndex(trimmed, -1))
				if ctxLines > 0 {
					start := max(0, i-ctxLines)
					end := min(len(lines)-1, i+ctxLines)
					for j := start; j <= end; j++ {
						prefix := "    "
						if j == i {
							prefix = ">>> "
						}
						matches = append(matches, fmt.Sprintf("%s:%d: %s%s", relPath, j+1, prefix, truncateLine(strings.TrimSpace(lines[j]))))
					}
					matches = append(matches, fmt.Sprintf("  [%d match(es) on this line]", count))
				} else {
					matches = append(matches, fmt.Sprintf("%s:%d:[%d] %s", relPath, i+1, count, truncateLine(trimmed)))
				}
			} else {
				if ctxLines > 0 {
					start := max(0, i-ctxLines)
					end := min(len(lines)-1, i+ctxLines)
					for j := start; j <= end; j++ {
						prefix := "    "
						if j == i {
							prefix = ">>> "
						}
						matches = append(matches, fmt.Sprintf("%s:%d: %s%s", relPath, j+1, prefix, truncateLine(strings.TrimSpace(lines[j]))))
					}
				} else {
					matches = append(matches, fmt.Sprintf("%s:%d:%s", relPath, i+1, truncateLine(trimmed)))
				}
			}
			if len(matches) >= headLimit {
				matches = append(matches, fmt.Sprintf("(showing first %d matches, truncated)", headLimit))
				return ToolResult{Output: strings.Join(matches, "\n")}
			}
		}
	}

	if len(matches) == 0 {
		if offset > 0 && skipped > 0 {
			return ToolResult{Output: fmt.Sprintf("No matches after skipping first %d results. (Searched %d files, %d matches total)", offset, filesSearched, totalMatchCount)}
		}
		return ToolResult{Output: fmt.Sprintf("No matches found. (Searched %d files)", filesSearched)}
	}

	summary := fmt.Sprintf("(Searched %d files, %d matches", filesSearched, totalMatchCount)
	if len(matches) < totalMatchCount {
		summary += fmt.Sprintf(", showing first %d", len(matches))
	}
	summary += ")"

	return ToolResult{Output: strings.Join(matches, "\n") + "\n" + summary}
}

func goSearchFilesOnly(re *regexp.Regexp, files []string, headLimit, offset int, filesSearched int) ToolResult {
	var found []string
	skipped := 0
	for _, fp := range files {
		if len(found) >= headLimit {
			break
		}
		data, err := os.ReadFile(fp)
		if err != nil {
			continue
		}
		if re.Match(data) {
			if skipped < offset {
				skipped++
				continue
			}
			relPath, _ := filepath.Rel(".", fp)
			found = append(found, relPath)
		}
	}
	if len(found) == 0 {
		return ToolResult{Output: fmt.Sprintf("No matches found. (Searched %d files)", filesSearched)}
	}
	return ToolResult{Output: strings.Join(found, "\n") + fmt.Sprintf("\n(Searched %d files, %d matches)", filesSearched, len(found))}
}

func goSearchCount(re *regexp.Regexp, files []string, filesSearched int) ToolResult {
	var lines []string
	totalMatches := 0
	for _, fp := range files {
		data, err := os.ReadFile(fp)
		if err != nil {
			continue
		}
		count := 0
		for _, l := range strings.Split(string(data), "\n") {
			if re.MatchString(l) {
				count++
			}
		}
		if count > 0 {
			relPath, _ := filepath.Rel(".", fp)
			lines = append(lines, fmt.Sprintf("%s:%d", relPath, count))
			totalMatches += count
		}
	}
	if len(lines) == 0 {
		return ToolResult{Output: fmt.Sprintf("No matches found. (Searched %d files)", filesSearched)}
	}
	return ToolResult{Output: strings.Join(lines, "\n") + fmt.Sprintf("\n(Searched %d files, %d matching lines)", filesSearched, totalMatches)}
}
