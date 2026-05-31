// Package lstool provides the Ls tool implementation with truncation and pluggable operations.
// Aligned to pi's ls.ts.
package lstool

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"miniclaudecode-go/pkg/core/tools/truncate"
)

const (
	// DefaultLimit is the maximum number of entries to return.
	DefaultLimit = 500
)

// LsInput is the input for the Ls tool.
type LsInput struct {
	Path  string `json:"path,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// LsDetails holds metadata about a list execution.
type LsDetails struct {
	EntryLimitReached int `json:"entry_limit_reached,omitempty"`
	Truncated         bool `json:"truncated,omitempty"`
}

// LsResult holds the result of a list operation.
type LsResult struct {
	Output  string      `json:"output"`
	Details LsDetails   `json:"details"`
}

// LsOperations allows pluggable operations for remote/SSH.
type LsOperations interface {
	Exists(absolutePath string) bool
	Stat(absolutePath string) (os.FileInfo, error)
	ReadDir(absolutePath string) ([]string, error)
}

// LocalLsOperations implements LsOperations for local filesystem.
type LocalLsOperations struct{}

func (LocalLsOperations) Exists(absolutePath string) bool {
	_, err := os.Stat(absolutePath)
	return err == nil
}

func (LocalLsOperations) Stat(absolutePath string) (os.FileInfo, error) {
	return os.Stat(absolutePath)
}

func (LocalLsOperations) ReadDir(absolutePath string) ([]string, error) {
	entries, err := os.ReadDir(absolutePath)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// Execute performs the Ls operation.
func Execute(input LsInput, cwd string, ops LsOperations) (*LsResult, error) {
	if ops == nil {
		ops = LocalLsOperations{}
	}

	// Resolve path
	dirPath := input.Path
	if dirPath == "" {
		dirPath = "."
	}
	if !filepath.IsAbs(dirPath) && cwd != "" {
		dirPath = filepath.Join(cwd, dirPath)
	}
	dirPath = filepath.Clean(dirPath)

	// Check path exists
	if !ops.Exists(dirPath) {
		return nil, fmt.Errorf("path not found: %s", dirPath)
	}

	// Check is a directory
	stat, err := ops.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat path: %w", err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dirPath)
	}

	// Read directory entries
	entries, err := ops.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory: %w", err)
	}

	// Sort alphabetically, case-insensitive
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i]) < strings.ToLower(entries[j])
	})

	// Apply limit
	effectiveLimit := input.Limit
	if effectiveLimit <= 0 {
		effectiveLimit = DefaultLimit
	}

	entryLimitReached := false
	if len(entries) > effectiveLimit {
		entries = entries[:effectiveLimit]
		entryLimitReached = true
	}

	// Format entries with directory indicator
	results := make([]string, 0, len(entries))
	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry)
		suffix := ""
		if entryStat, err := ops.Stat(fullPath); err == nil && entryStat.IsDir() {
			suffix = "/"
		}
		results = append(results, entry+suffix)
	}

	// Join output
	rawOutput := strings.Join(results, "\n")

	// Apply byte truncation
	truncResult := truncate.TruncateHead(rawOutput, truncate.TruncationOptions{
		MaxBytes: truncate.DefaultMaxBytes,
	})

	output := truncResult.Content
	details := LsDetails{}

	// Build notices for truncation and entry limits
	notices := []string{}
	if entryLimitReached {
		notices = append(notices, fmt.Sprintf("%d entries limit reached. Use limit=%d for more", effectiveLimit, effectiveLimit*2))
		details.EntryLimitReached = effectiveLimit
	}
	if truncResult.Truncated {
		notices = append(notices, fmt.Sprintf("%d bytes limit reached", truncResult.MaxBytes))
		details.Truncated = true
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}

	if output == "" {
		output = "(empty directory)"
	}

	return &LsResult{
		Output:  output,
		Details: details,
	}, nil
}
