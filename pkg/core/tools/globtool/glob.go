// Package globtool provides the Glob tool implementation with pluggable operations.
// Aligned to pi's find.ts (Glob mode).
package globtool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"miniclaudecode-go/pkg/core/tools/truncate"
)

// GlobInput is the input for the Glob tool.
type GlobInput struct {
	Pattern string `json:"pattern"`
	Cwd     string `json:"cwd,omitempty"`
}

// GlobDetails holds metadata about a glob execution.
type GlobDetails struct {
	Truncated bool `json:"truncated,omitempty"`
}

// GlobResult holds the result of a glob operation.
type GlobResult struct {
	Matches []string    `json:"matches"`
	Output  string      `json:"output"`
	Details GlobDetails `json:"details"`
}

// GlobOperations allows pluggable operations for remote/SSH.
type GlobOperations interface {
	Glob(pattern string) ([]string, error)
}

// LocalGlobOperations implements GlobOperations for local filesystem.
type LocalGlobOperations struct{}

func (LocalGlobOperations) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// Execute performs the Glob operation.
func Execute(input GlobInput, cwd string, ops GlobOperations) (*GlobResult, error) {
	if ops == nil {
		ops = LocalGlobOperations{}
	}

	// Resolve pattern relative to cwd
	pattern := input.Pattern
	if !filepath.IsAbs(pattern) && cwd != "" {
		pattern = filepath.Join(cwd, pattern)
	}

	matches, err := ops.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob error: %w", err)
	}

	// Sort matches alphabetically, case-insensitive
	sort.Slice(matches, func(i, j int) bool {
		return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
	})

	// Build output
	var rawOutput string
	if len(matches) == 0 {
		rawOutput = "No files found"
	} else {
		rawOutput = strings.Join(matches, "\n")
	}

	// Apply byte truncation
	truncResult := truncate.TruncateHead(rawOutput, truncate.TruncationOptions{
		MaxBytes: truncate.DefaultMaxBytes,
	})

	output := truncResult.Content
	details := GlobDetails{}
	if truncResult.Truncated {
		details.Truncated = true
		output += fmt.Sprintf("\n\n[%d bytes limit reached]", truncResult.MaxBytes)
	}

	return &GlobResult{
		Matches: matches,
		Output:  output,
		Details: details,
	}, nil
}

// FormatGlobOutput formats a glob result for display.
func FormatGlobOutput(result *GlobResult) string {
	if len(result.Matches) == 0 {
		return "No files found"
	}
	output := fmt.Sprintf("Found %d files:\n%s", len(result.Matches), result.Output)
	if result.Details.Truncated {
		output += "\n[Output truncated]"
	}
	return output
}

// resolveToCwd resolves a path relative to cwd.
func resolveToCwd(path, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

// pathExists checks if a path exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
