package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"miniclaudecode-go/tools/rgrep"
)

const maxGrepMatches = 250

// GrepTool searches file contents using regex. Uses ripgrep if available, otherwise Go regexp.
type GrepTool struct{}

func (*GrepTool) Name() string { return "grep" }
func (*GrepTool) Description() string {
	return "ALWAYS use grep for content search tasks. NEVER invoke grep or rg via exec. " +
		"A powerful search tool built on ripgrep — cheap operation, use liberally. " +
		"Supports full regex syntax. Filter files with glob parameter. " +
		"Output modes: 'content' shows matching lines, 'files_with_matches' shows file paths, 'count' shows match counts. " +
		"Case-insensitive: use -i, ignore_case, or case_insensitive (all equivalent). " +
		"Literal text search: use fixed_strings=true instead of escaping regex chars."
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
			"-i": map[string]any{
				"type":        "boolean",
				"description": "Case insensitive search (rg -i). Default: false.",
			},
			"ignore_case": map[string]any{
				"type":        "boolean",
				"description": "Alias for -i. Case insensitive search (default: false).",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "Alias for -i. Case insensitive search (default: false).",
			},
			"fixed_strings": map[string]any{
				"type":        "boolean",
				"description": "Treat pattern as a literal string, not regex (default: false).",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"enum":        []any{"content", "files_with_matches", "count"},
				"description": "Output mode (default: files_with_matches): 'content' shows matching lines, 'files_with_matches' shows file paths, 'count' shows per-file match counts.",
			},
			"-B": map[string]any{
				"type":        "integer",
				"description": "Number of lines to show before each match (rg -B). Requires output_mode: content, ignored otherwise.",
			},
			"-A": map[string]any{
				"type":        "integer",
				"description": "Number of lines to show after each match (rg -A). Requires output_mode: content, ignored otherwise.",
			},
			"-C": map[string]any{
				"type":        "integer",
				"description": "Alias for context. Number of lines to show before and after each match.",
			},
			"context": map[string]any{
				"type":        "integer",
				"description": "Number of lines to show before and after each match (rg -C). Requires output_mode: content, ignored otherwise.",
			},
			"context_before": map[string]any{
				"type":        "integer",
				"description": "Alias for -B. Lines of context before each match (default: 0).",
			},
			"context_after": map[string]any{
				"type":        "integer",
				"description": "Alias for -A. Lines of context after each match (default: 0).",
			},
			"-n": map[string]any{
				"type":        "boolean",
				"description": "Show line numbers in output (rg -n). Requires output_mode: content, ignored otherwise. Defaults to true.",
			},
			"multiline": map[string]any{
				"type":        "boolean",
				"description": "Enable multiline mode where . matches newlines and patterns can span lines (rg -U --multiline-dotall). Default: false.",
			},
			"max_depth": map[string]any{
				"type":        "integer",
				"description": "Maximum directory depth to search. Limits how many levels of subdirectories to traverse. Useful for avoiding scanning too many files (default: unlimited).",
			},
			"max_filesize": map[string]any{
				"type":        "string",
				"description": "Maximum file size to search (e.g. '1M', '500K', '100B'). Files larger than this are skipped.",
			},
			"excludes": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "List of file/directory patterns to exclude. Supports glob patterns like `**/.claude/**` or directory names like `node_modules`.",
			},
			"head_limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 250). Set to 0 for unlimited.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Skip the first N results for pagination (default: 0).",
			},
		},
		"required": []string{"pattern"},
	}
}

func (*GrepTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *GrepTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	// Check context early
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: grep timed out: %v", ctx.Err()), IsError: true}
	default:
	}

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

	// Support -i, ignore_case, and case_insensitive
	caseInsensitive, _ := params["-i"].(bool)
	if !caseInsensitive {
		if ci, _ := params["ignore_case"].(bool); ci {
			caseInsensitive = true
		}
	}
	if !caseInsensitive {
		if ci, _ := params["case_insensitive"].(bool); ci {
			caseInsensitive = true
		}
	}
	fixedStrings, _ := params["fixed_strings"].(bool)
	multiline, _ := params["multiline"].(bool)
	showLineNumbers, _ := params["-n"].(bool)
	// -n defaults to true, so only false if explicitly set
	if _, hasN := params["-n"]; !hasN {
		showLineNumbers = true
	}
	outputMode, _ := params["output_mode"].(string)
	if outputMode == "" {
		outputMode = "files_with_matches"
	}
	// Validate output_mode
	validModes := map[string]bool{"content": true, "files_with_matches": true, "count": true}
	if !validModes[outputMode] {
		return ToolResult{Output: fmt.Sprintf("Error: invalid output_mode '%s'. Must be one of: content, files_with_matches, count", outputMode), IsError: true}
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
	if headLimit < 0 {
		headLimit = maxGrepMatches
	}
	// head_limit=0 means unlimited (matching upstream behavior)
	if headLimit == 0 {
		headLimit = 1<<31 - 1
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
	maxFilesizeStr, _ := params["max_filesize"].(string)
	maxFilesizeBytes := int64(0)
	if maxFilesizeStr != "" {
		maxFilesizeBytes = parseFilesize(maxFilesizeStr)
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

	// Parse context params (official: -C/context takes precedence over -B/-A)
	// Support both official names (-B, -A, -C) and legacy aliases (context_before, context_after)
	ctxBefore := parseIntParam(params, "-B")
	if ctxBefore == 0 {
		ctxBefore = parseIntParam(params, "context_before")
	}
	ctxAfter := parseIntParam(params, "-A")
	if ctxAfter == 0 {
		ctxAfter = parseIntParam(params, "context_after")
	}
	ctxCombined := parseIntParam(params, "-C")
	if ctxCombined == 0 {
		ctxCombined = parseIntParam(params, "context")
	}
	if ctxCombined > 0 {
		if ctxBefore == 0 {
			ctxBefore = ctxCombined
		}
		if ctxAfter == 0 {
			ctxAfter = ctxCombined
		}
	}

	// Parse excludes
	var excludes []string
	if exclArr, ok := params["excludes"]; ok {
		if arr, ok := exclArr.([]any); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok {
					excludes = append(excludes, s)
				}
			}
		}
	}

	if _, err := exec.LookPath("rg"); err == nil {
		return rgSearch(ctx, pattern, searchPath, include, typeFilter, caseInsensitive, fixedStrings, outputMode, showLineNumbers, multiline, ctxBefore, ctxAfter, headLimit, offset, maxDepth, maxFilesizeStr, excludes)
	}

	// Pure Go fallback using rgrep engine
	rgrepCfg := rgrep.SearchConfig{
		Pattern:         pattern,
		Path:            searchPath,
		Glob:            include,
		TypeFilter:      typeFilter,
		CaseInsensitive: caseInsensitive,
		FixedStrings:    fixedStrings,
		OutputMode:      rgrep.OutputMode(outputMode),
		ShowLineNums:    showLineNumbers,
		Multiline:       multiline,
		ContextBefore:   ctxBefore,
		ContextAfter:    ctxAfter,
		HeadLimit:       headLimit,
		Offset:          offset,
		MaxDepth:        maxDepth,
		MaxFilesize:     maxFilesizeBytes,
		Ctx:             ctx,
	}
	sr := rgrep.Search(rgrepCfg)
	return ToolResult{Output: rgrep.FormatResult(sr, rgrepCfg)}
}

func (t *GrepTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
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

// splitGlobPatterns splits a glob string on commas and whitespace,
// respecting brace groups. E.g. "*.ts, *.js" -> ["*.ts", "*.js"].
func splitGlobPatterns(glob string) []string {
	var parts []string
	var current strings.Builder
	inBrace := false
	for _, c := range glob {
		switch c {
		case '{':
			inBrace = true
			current.WriteRune(c)
		case '}':
			inBrace = false
			current.WriteRune(c)
		case ',', ' ':
			if !inBrace && current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}


func rgSearch(ctx context.Context, pattern, path, include, typeFilter string, caseInsensitive, fixedStrings bool, outputMode string, showLineNumbers, multiline bool, ctxBefore, ctxAfter, headLimit, offset int, maxDepth int, maxFilesize string, excludes []string) ToolResult {
	args := []string{"--hidden", "--max-columns", "500"}

	// Exclude VCS directories (matching official Claude Code behavior)
	vcsDirs := []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}
	for _, dir := range vcsDirs {
		args = append(args, "--glob", "!"+dir)
	}

	// Add user exclude patterns
	for _, excl := range excludes {
		args = append(args, "--glob", "!"+excl)
	}

	// --no-heading is not used (we use -n for line numbers)

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
	if multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	// Context params only apply in content mode
	if outputMode == "content" {
		if ctxBefore > 0 {
			args = append(args, "-B", fmt.Sprintf("%d", ctxBefore))
		}
		if ctxAfter > 0 {
			args = append(args, "-A", fmt.Sprintf("%d", ctxAfter))
		}
		// Use null-data for multiline mode so records are NUL-delimited
		if multiline {
			args = append(args, "--null-data")
		}
	}

	// Show line numbers only in content mode (matching official behavior)
	if showLineNumbers && outputMode == "content" {
		args = append(args, "-n")
	}

	// Limit directory traversal depth
	if maxDepth > 0 {
		args = append(args, "--max-depth", fmt.Sprintf("%d", maxDepth))
	}
	// Skip large files
	if maxFilesize != "" {
		args = append(args, "--max-filesize", maxFilesize)
	}

	// Don't pass -m to rg. Upstream retrieves all results and applies
	// offset+head_limit in the TypeScript layer. Passing -m breaks
	// pagination because rg returns headLimit results but then offset
	// slices off the front, returning fewer than expected.
	if typeFilter != "" {
		if exts := rgrep.ExtensionsForType(strings.ToLower(typeFilter)); len(exts) > 0 {
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
	if include != "" {
		// Split glob on commas and spaces (matching upstream behavior)
		// E.g. "*.ts, *.js" becomes --glob *.ts --glob *.js
		for _, g := range splitGlobPatterns(include) {
			args = append(args, "--glob", strings.TrimSpace(g))
		}
	}

	// If pattern starts with dash, use -e flag to prevent rg from interpreting it as an option
	if strings.HasPrefix(pattern, "-") {
		args = append(args, "-e", pattern)
	} else {
		args = append(args, pattern)
	}
	args = append(args, path)

	cmd := exec.CommandContext(ctx, "rg", args...)
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

	var lines []string
	if multiline && outputMode == "content" {
		// NUL-delimited records when --null-data is used
		lines = strings.Split(output, "\x00")
		// Filter out empty entries
		n := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				lines[n] = l
				n++
			}
		}
		lines = lines[:n]
	} else {
		lines = strings.Split(output, "\n")
	}

	// Apply offset
	if offset > 0 && offset < len(lines) {
		lines = lines[offset:]
	}
	// Apply head_limit if set (0 means unlimited)
	if headLimit > 0 && len(lines) > headLimit {
		lines = lines[:headLimit]
		lines = append(lines, fmt.Sprintf("(showing first %d matches, truncated)", headLimit))
	}
	return ToolResult{Output: strings.Join(lines, "\n")}
}

// parseFilesize parses a file size string like "1M", "500K", "100B" into bytes.
func parseFilesize(s string) int64 {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0
	}

	var multiplier int64 = 1
	last := strings.ToLower(string(s[len(s)-1]))
	numStr := s

	switch last {
	case "b":
		multiplier = 1
		numStr = s[:len(s)-1]
	case "k":
		multiplier = 1024
		numStr = s[:len(s)-1]
	case "m":
		multiplier = 1024 * 1024
		numStr = s[:len(s)-1]
	case "g":
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-1]
	}

	var val int64
	fmt.Sscanf(numStr, "%d", &val)
	return val * multiplier
}
