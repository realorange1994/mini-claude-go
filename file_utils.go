package main

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// file utilities ported from upstream: src/utils/file.ts

// ConvertLeadingTabsToSpaces converts leading tabs to 2 spaces each.
// Only leading tabs on each line are converted; tabs within the line are preserved.
// Upstream: convertLeadingTabsToSpaces() in file.ts
func ConvertLeadingTabsToSpaces(content string) string {
	if !strings.Contains(content, "\t") {
		return content
	}

	var b strings.Builder
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		// Count leading tabs
		leading := 0
		for _, ch := range line {
			if ch == '\t' {
				leading++
			} else {
				break
			}
		}

		// Replace leading tabs with spaces (2 per tab)
		for j := 0; j < leading; j++ {
			b.WriteString("  ")
		}
		b.WriteString(line[leading:])
	}

	return b.String()
}

// AddLineNumbersOptions are options for AddLineNumbers.
type AddLineNumbersOptions struct {
	Content  string
	StartLine int // 1-indexed
}

// AddLineNumbers adds line numbers to content, starting from StartLine.
// Uses compact format: "N\tline" (tab-separated).
// Upstream: addLineNumbers() in file.ts
func AddLineNumbers(opts AddLineNumbersOptions) string {
	if opts.Content == "" {
		return ""
	}

	lines := strings.Split(opts.Content, "\n")
	var result []string
	for i, line := range lines {
		num := opts.StartLine + i
		result = append(result, strconv.Itoa(num)+"\t"+line)
	}

	return strings.Join(result, "\n")
}

// stripLineNumberRegex matches optional whitespace, a number, then an arrow (→) or tab separator.
// U+2192 is the rightwards arrow character used in some line-number formats.
var stripLineNumberRegex = regexp.MustCompile(`^\s*\d+[` + "\u2192" + `\t](.*)$`)

// StripLineNumberPrefix removes the line number prefix from a line.
// Supports formats: "N→line" or "N\tline" with optional leading whitespace.
// Returns the line unchanged if no prefix is found.
// Upstream: stripLineNumberPrefix() in file.ts
func StripLineNumberPrefix(line string) string {
	matches := stripLineNumberRegex.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return line
}

// PathsEqual compares two paths for equality, handling platform differences.
// On Windows, normalizes to forward slashes and lowercases for case-insensitive comparison.
// On Unix, only normalizes separators.
// Upstream: pathsEqual() in file.ts
func PathsEqual(path1, path2 string) bool {
	return NormalizePathForComparison(path1) == NormalizePathForComparison(path2)
}

// NormalizePathForComparison normalizes a path for comparison across platforms.
// Resolves dot segments, removes redundant separators, converts backslashes to slashes.
// On Windows, also lowercases the path.
// Upstream: normalizePathForComparison() in file.ts
func NormalizePathForComparison(filePath string) string {
	// Use filepath.Clean to normalize separators and resolve . and ..
	normalized := filepath.Clean(filePath)

	// Always convert to forward slashes for consistency
	normalized = strings.ReplaceAll(normalized, "\\", "/")

	// Lowercase for case-insensitive comparison on Windows
	if isWindowsPlatform() {
		normalized = strings.ToLower(normalized)
	}

	return normalized
}

// isWindowsPlatform returns true if running on Windows.
func isWindowsPlatform() bool {
	return filepath.Separator == '\\'
}
