package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ============================================================================
// Section: diff (from diff.go)
// ============================================================================

// StructuredDiff generates a unified diff between two strings.
// Falls back to a simple line-by-line comparison if diff tools are unavailable.
func StructuredDiff(oldContent, newContent, filePath string) string {
	// Try using git diff if available
	if _, err := exec.LookPath("git"); err == nil {
		return gitDiff(oldContent, newContent, filePath)
	}
	// Fallback: simple inline diff
	return simpleDiff(oldContent, newContent, filePath)
}

// gitDiff uses git's diff engine for proper unified diff output.
func gitDiff(oldContent, newContent, filePath string) string {
	// Write old content to a temp file, new content to another, then diff
	tmpDir, err := os.MkdirTemp("", "diff-*")
	if err != nil {
		return simpleDiff(oldContent, newContent, filePath)
	}
	defer os.RemoveAll(tmpDir)

	oldFile := tmpDir + "/old"
	newFile := tmpDir + "/new"

	if err := os.WriteFile(oldFile, []byte(oldContent), 0644); err != nil {
		return simpleDiff(oldContent, newContent, filePath)
	}
	if err := os.WriteFile(newFile, []byte(newContent), 0644); err != nil {
		return simpleDiff(oldContent, newContent, filePath)
	}

	cmd := exec.Command("git", "diff", "--no-index", "--unified=3", oldFile, newFile)
	out, _ := cmd.CombinedOutput()

	// Replace temp file paths with the actual file path
	result := string(out)
	result = strings.Replace(result, oldFile, "a/"+filePath, -1)
	result = strings.Replace(result, newFile, "b/"+filePath, -1)

	return result
}

// simpleDiff produces a basic line-by-line diff when git is unavailable.
func simpleDiff(oldContent, newContent, filePath string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n", filePath))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))

	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	for i := 0; i < maxLines; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if i < len(oldLines) {
				sb.WriteString(fmt.Sprintf("-%s\n", oldLine))
			}
			if i < len(newLines) {
				sb.WriteString(fmt.Sprintf("+%s\n", newLine))
			}
		}
	}

	return sb.String()
}

// ============================================================================
// Section: file_utils (from file_utils.go)
// ============================================================================

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
	Content   string
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

// ============================================================================
// Section: path_utils (from path_utils.go)
// ============================================================================

// ContainsPathTraversal checks if a path contains parent-directory references (..).
// Returns true if the path contains ".." as a path segment (not just part of a filename).
// Upstream: containsPathTraversal() in path.ts
func ContainsPathTraversal(path string) bool {
	// Check for .. preceded or followed by a path separator
	// This catches: ../foo, foo/../bar, foo/.., .., foo\..\bar
	pathTraversalRe := regexp.MustCompile(`(^|[/\\])\.\.([/\\]|$)`)
	return pathTraversalRe.MatchString(path)
}

// NormalizePathForConfigKey normalizes a path for use as a configuration key.
// Resolves dot segments, converts backslashes to forward slashes, and normalizes separators.
// Upstream: normalizePathForConfigKey() in path.ts
func NormalizePathForConfigKey(path string) string {
	// Convert backslashes to forward slashes
	result := strings.ReplaceAll(path, "\\", "/")
	// Use filepath.Clean to resolve . and .. segments, then convert back
	cleaned := filepath.Clean(result)
	// filepath.Clean may use backslashes on Windows; convert back
	return strings.ReplaceAll(cleaned, "\\", "/")
}

// ToRelativePath returns a relative path from the base directory if the path
// is inside it, or the absolute path otherwise.
// Upstream: toRelativePath() in path.ts
func ToRelativePath(absPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return absPath
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return absPath
	}
	// If the relative path goes above cwd, return the absolute path instead
	if strings.HasPrefix(rel, "..") {
		return absPath
	}
	return rel
}

// GetDirectoryForPath returns the directory containing the given path.
// If the path is a directory, returns it unchanged.
// If the path is a file, returns its parent directory.
// If the path does not exist, returns its parent directory.
// Upstream: getDirectoryForPath() in path.ts
func GetDirectoryForPath(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		// Path doesn't exist; return parent directory
		return filepath.Dir(path)
	}
	if info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}
