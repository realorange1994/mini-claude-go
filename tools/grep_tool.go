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

func (*GrepTool) Name() string        { return "grep" }
func (*GrepTool) Description() string {
	return "Search file contents using regex. Uses ripgrep (rg) if available, otherwise falls back to Go regexp."
}

func (*GrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search (default: current directory).",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob to filter files (e.g. '*.py').",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Language type filter (e.g. 'go', 'py', 'js', 'ts', 'rust', 'java').",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "Case insensitive search (default: false).",
			},
			"fixed_strings": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as literal string, not regex (default: false).",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"description": "Output mode: 'content' (default), 'files_with_matches', or 'count'.",
			},
			"count_matches": map[string]any{
				"type":        "boolean",
				"description": "Count per-line match occurrences (not just matching lines). Only with content mode.",
			},
			"context_before": map[string]any{
				"type":        "integer",
				"description": "Lines of context before each match (rg only, default: 0).",
			},
			"context_after": map[string]any{
				"type":        "integer",
				"description": "Lines of context after each match (rg only, default: 0).",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default: 250).",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip the first N results for pagination (default: 0).",
			},
			"context": map[string]any{
				"type":        "integer",
				"description": "Lines of context before and after each match (Go fallback only, default: 0, max: 3).",
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
	caseInsensitive, _ := params["case_insensitive"].(bool)
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
	ctxBefore := 0
	if cb, ok := params["context_before"]; ok {
		switch v := cb.(type) {
		case float64:
			ctxBefore = int(v)
		case int:
			ctxBefore = v
		}
	}
	ctxAfter := 0
	if ca, ok := params["context_after"]; ok {
		switch v := ca.(type) {
		case float64:
			ctxAfter = int(v)
		case int:
			ctxAfter = v
		}
	}

	// Combined 'context' param
	ctxCombined := 0
	if c, ok := params["context"]; ok {
		switch v := c.(type) {
		case float64:
			ctxCombined = int(v)
		case int:
			ctxCombined = v
		}
	}
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

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

	if _, err := exec.LookPath("rg"); err == nil {
		return rgSearch(pattern, searchPath, include, typeFilter, caseInsensitive, fixedStrings, outputMode, ctxBefore, ctxAfter, headLimit, offset)
	}
	return goSearch(pattern, searchPath, include, typeFilter, caseInsensitive, fixedStrings, outputMode, headLimit, offset, ctxCombined, countMatches)
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

func rgSearch(pattern, path, include, typeFilter string, caseInsensitive, fixedStrings bool, outputMode string, ctxBefore, ctxAfter, headLimit, offset int) ToolResult {
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

	args = append(args, "-m", fmt.Sprintf("%d", headLimit))
	args = append(args, pattern, path)

	if include != "" {
		args = append(args, "--glob", include)
	}
	if typeFilter != "" {
		if exts, ok := typeMap[strings.ToLower(typeFilter)]; ok {
			for _, e := range exts {
				args = append(args, "--type-add", "mytype:"+e)
			}
			args = append(args, "--type", "mytype")
		}
	}

	cmd := exec.Command("rg", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if output == "" {
		if err != nil {
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

func goSearch(pattern, path, include, typeFilter string, caseInsensitive, fixedStrings bool, outputMode string, headLimit, offset, ctxLines int, countMatches bool) ToolResult {
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
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
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
