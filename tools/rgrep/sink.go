package rgrep

import (
	"fmt"
	"strings"
)

// FormatResult formats a SearchResult into a human-readable string.
// This mirrors the output format of the original grep_tool.go.
func FormatResult(sr SearchResult, cfg SearchConfig) string {
	if sr.Err != nil {
		return fmt.Sprintf("Error: %v", sr.Err)
	}

	switch cfg.OutputMode {
	case OutputFilesWithMatch:
		return formatFilesWithMatch(sr, cfg)
	case OutputCount:
		return formatCount(sr, cfg)
	default:
		return formatContent(sr, cfg)
	}
}

func formatFilesWithMatch(sr SearchResult, cfg SearchConfig) string {
	if len(sr.Results) == 0 {
		return fmt.Sprintf("No matches found. (Searched %d files)", sr.FilesSearched)
	}

	var lines []string
	for _, r := range sr.Results {
		lines = append(lines, r.Path)
	}

	if sr.Truncated && cfg.HeadLimit > 0 {
		lines = append(lines, fmt.Sprintf("(showing first %d matches, truncated)", cfg.HeadLimit))
	}

	summary := fmt.Sprintf("\n(Searched %d files, %d matches)", sr.FilesSearched, sr.TotalMatches)
	return strings.Join(lines, "\n") + summary
}

func formatCount(sr SearchResult, cfg SearchConfig) string {
	if len(sr.Results) == 0 {
		return fmt.Sprintf("No matches found. (Searched %d files)", sr.FilesSearched)
	}

	var lines []string
	for _, r := range sr.Results {
		// Output format: path:count (matching ripgrep --count)
		lines = append(lines, fmt.Sprintf("%s:%d", r.Path, r.LineNum))
	}

	if sr.Truncated && cfg.HeadLimit > 0 {
		lines = append(lines, fmt.Sprintf("(showing first %d matches, truncated)", cfg.HeadLimit))
	}

	summary := fmt.Sprintf("\n(Searched %d files, %d matches total)", sr.FilesSearched, sr.TotalMatches)
	return strings.Join(lines, "\n") + summary
}

func formatContent(sr SearchResult, cfg SearchConfig) string {
	if len(sr.Results) == 0 {
		if cfg.Offset > 0 {
			return fmt.Sprintf("No matches after skipping first %d results. (Searched %d files, %d matches total)", cfg.Offset, sr.FilesSearched, sr.TotalMatches)
		}
		return fmt.Sprintf("No matches found. (Searched %d files)", sr.FilesSearched)
	}

	var lines []string
	for _, r := range sr.Results {
		if cfg.ShowLineNums {
			lines = append(lines, fmt.Sprintf("%s:%d:%s", r.Path, r.LineNum, r.Line))
		} else {
			lines = append(lines, fmt.Sprintf("%s:%s", r.Path, r.Line))
		}
	}

	if sr.Truncated && cfg.HeadLimit > 0 {
		lines = append(lines, fmt.Sprintf("(showing first %d matches, truncated)", cfg.HeadLimit))
	}

	summary := fmt.Sprintf("\n(Searched %d files, %d matches", sr.FilesSearched, sr.TotalMatches)
	if len(sr.Results) < sr.TotalMatches {
		summary += fmt.Sprintf(", showing first %d", len(sr.Results))
	}
	summary += ")"

	return strings.Join(lines, "\n") + summary
}