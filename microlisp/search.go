package microlisp

import (
	"fmt"

	"miniclaudecode-go/tools/rgrep"
)

// builtinGoSearch delegates to the rgrep search engine.
// Args: pattern path output-mode case-insensitive head-limit glob
func builtinGoSearch(args []*Value) (*Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("go-search: need at least a pattern")
	}

	pattern := primaryValue(args[0]).str
	root := "."
	if len(args) > 1 {
		root = primaryValue(args[1]).str
	}

	outputMode := rgrep.OutputContent
	if len(args) > 2 {
		switch primaryValue(args[2]).str {
		case "count":
			outputMode = rgrep.OutputCount
		case "files_with_matches":
			outputMode = rgrep.OutputFilesWithMatch
		default:
			outputMode = rgrep.OutputContent
		}
	}

	caseInsensitive := false
	if len(args) > 3 && !isNil(primaryValue(args[3])) {
		caseInsensitive = true
	}

	headLimit := 100
	if len(args) > 4 {
		headLimit = int(primaryValue(args[4]).num)
		if headLimit <= 0 {
			headLimit = 100
		}
	}

	glob := ""
	if len(args) > 5 {
		v := primaryValue(args[5])
		if v.typ == VStr && v.str != "" {
			glob = v.str
		}
	}

	cfg := rgrep.SearchConfig{
		Pattern:         pattern,
		Path:            root,
		OutputMode:      outputMode,
		CaseInsensitive: caseInsensitive,
		FixedStrings:    true, // literal substring search, not regex
		HeadLimit:       headLimit,
		Glob:            glob,
	}

	result := rgrep.Search(cfg)

	if result.Err != nil {
		return vstr(fmt.Sprintf("Error: %v", result.Err)), nil
	}

	switch outputMode {
	case rgrep.OutputCount:
		// In count mode, TotalMatches is the total across all files
		total := 0
		for _, r := range result.Results {
			total += r.LineNum // LineNum holds the count per file
		}
		return vstr(fmt.Sprintf("%d", total)), nil

	case rgrep.OutputFilesWithMatch:
		lines := make([]string, len(result.Results))
		for i, r := range result.Results {
			lines[i] = r.Path
		}
		out := joinWithNL(lines)
		if result.Truncated {
			out += "\n... (output truncated, try a more specific pattern, use glob filter, or use count mode for complete results)"
		}
		return vstr(out), nil

	default:
		// Content mode: format as "path\tlineNum\tline"
		lines := make([]string, len(result.Results))
		for i, r := range result.Results {
			lines[i] = fmt.Sprintf("%s\t%d\t%s", r.Path, r.LineNum, r.Line)
		}
		out := joinWithNL(lines)
		if result.Truncated {
			out += "\n... (output truncated, try a more specific pattern, use glob filter, or use count mode for complete results)"
		}
		return vstr(out), nil
	}
}

func joinWithNL(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += "\n"
		}
		result += s
	}
	return result
}
