// Package findtool provides the Find tool implementation using fd or Go fallback.
// Aligned to pi's tools/find.ts.
package findtool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultMaxFindResults is the default maximum number of results.
	DefaultMaxFindResults = 100

	// DefaultMaxDepth is the default maximum directory depth.
	DefaultMaxDepth = 10
)

// FindInput is the input for the Find tool.
// Aligned to pi's FindToolInput.
type FindInput struct {
	Dir      string `json:"dir,omitempty"`      // Directory to search in
	Pattern  string `json:"pattern"`            // Glob or regex pattern for file names
	MaxDepth int    `json:"max_depth,omitempty"` // Maximum directory depth
	Type     string `json:"type,omitempty"`     // "f" (files only), "d" (dirs only), "" (both)
	MaxAge   string `json:"max_age,omitempty"`  // Maximum file age (e.g. "1d", "1w")
}

// FindResult holds the result of a Find operation.
type FindResult struct {
	Paths       []string `json:"paths"`
	TotalCount  int      `json:"total_count,omitempty"`
	Truncated   bool     `json:"truncated,omitempty"`
	FilesSearched int    `json:"files_searched,omitempty"`
}

// FindOperations allows pluggable execution for remote/SSH.
type FindOperations interface {
	Execute(ctx context.Context, args []string, cwd string) ([]byte, error)
}

// LocalFindOperations implements FindOperations using local fd or Go fallback.
type LocalFindOperations struct {
	FdPath string // Path to fd binary; empty means use PATH
}

func (op LocalFindOperations) Execute(ctx context.Context, args []string, cwd string) ([]byte, error) {
	fdPath := op.FdPath
	if fdPath == "" {
		fdPath = "fd"
	}

	cmd := exec.CommandContext(ctx, fdPath, args...)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if cmd.ProcessState.ExitCode() == 1 {
			return nil, nil // No results
		}
		return nil, fmt.Errorf("fd failed: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// findFdBinary attempts to locate fd on the system.
func findFdBinary() string {
	if path, err := exec.LookPath("fd"); err == nil {
		return path
	}

	candidates := []string{"fd", "/usr/bin/fd", "/usr/local/bin/fd"}

	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			`C:\Program Files\fd\fd.exe`,
			filepath.Join(os.Getenv("USERPROFILE"), ".cargo", "bin", "fd.exe"),
			filepath.Join(os.Getenv("USERPROFILE"), "scoop", "shims", "fd.exe"),
		)
	} else {
		home, _ := os.UserHomeDir()
		candidates = append(candidates,
			filepath.Join(home, ".cargo", "bin", "fd"),
			filepath.Join(home, ".local", "bin", "fd"),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return ""
}

// HasFd checks if fd is available on the system.
func HasFd() bool {
	return findFdBinary() != ""
}

// Execute performs the Find operation using fd if available.
func Execute(ctx context.Context, input FindInput, cwd string, ops FindOperations) (*FindResult, error) {
	if input.Pattern == "" {
		return nil, fmt.Errorf("missing required parameter: pattern")
	}

	dir := input.Dir
	if dir == "" {
		dir = cwd
	}
	if dir == "" {
		dir = "."
	}

	maxDepth := input.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}

	// Try fd first
	if HasFd() {
		return executeWithFd(ctx, input, dir, ops)
	}

	// Fallback to Go implementation
	return executeWithGo(ctx, input, dir, maxDepth)
}

// executeWithFd uses fd for file finding.
func executeWithFd(ctx context.Context, input FindInput, dir string, ops FindOperations) (*FindResult, error) {
	args := []string{
		"--color=never",
		"--max-depth", fmt.Sprintf("%d", input.MaxDepth),
	}

	// Type filter
	switch input.Type {
	case "f":
		args = append(args, "--type", "f")
	case "d":
		args = append(args, "--type", "d")
	}

	// Max age
	if input.MaxAge != "" {
		args = append(args, "--changed-within", input.MaxAge)
	}

	// Result limit
	args = append(args, "--max-results", fmt.Sprintf("%d", DefaultMaxFindResults+1)) // +1 to detect truncation

	// Pattern
	args = append(args, input.Pattern)

	// Directory
	args = append(args, dir)

	output, err := ops.Execute(ctx, args, "")
	if err != nil {
		return nil, err
	}

	if len(output) == 0 {
		return &FindResult{Paths: nil, TotalCount: 0}, nil
	}

	// Parse output
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	truncated := len(lines) > DefaultMaxFindResults
	if truncated {
		lines = lines[:DefaultMaxFindResults]
	}

	return &FindResult{
		Paths:      lines,
		TotalCount: len(lines),
		Truncated:  truncated,
	}, nil
}

// executeWithGo is a pure-Go fallback using filepath.Walk.
func executeWithGo(ctx context.Context, input FindInput, dir string, maxDepth int) (*FindResult, error) {
	// Compile pattern as regex (try regex first, fall back to glob)
	var regex *regexp.Regexp
	var err error

	regex, err = regexp.Compile(input.Pattern)
	if err != nil {
		// Try as glob pattern - convert to regex
		globRegex := globToRegex(input.Pattern)
		regex, _ = regexp.Compile(globRegex)
	}

	var paths []string

	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check depth
		if maxDepth > 0 {
			rel, _ := filepath.Rel(dir, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if depth > maxDepth {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Type filter
		switch input.Type {
		case "f":
			if info.IsDir() {
				return nil
			}
		case "d":
			if !info.IsDir() {
				return nil
			}
		}

		// Pattern match
		name := info.Name()
		if regex != nil && regex.MatchString(name) {
			paths = append(paths, path)
		}

		// Limit results
		if len(paths) >= DefaultMaxFindResults+1 {
			return fmt.Errorf("max results reached")
		}

		return nil
	})

	// Handle max results error
	truncated := len(paths) > DefaultMaxFindResults
	if truncated {
		paths = paths[:DefaultMaxFindResults]
	}

	if err != nil && err.Error() != "max results reached" {
		return nil, err
	}

	return &FindResult{
		Paths:      paths,
		TotalCount: len(paths),
		Truncated:  truncated,
	}, nil
}

// globToRegex converts a simple glob pattern to a regex pattern.
func globToRegex(glob string) string {
	var b strings.Builder
	for _, c := range glob {
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '^', '$', '+', '(', ')', '[', ']', '{', '}', '|', '\\':
			b.WriteRune('\\')
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// FormatFindOutput formats find results as a human-readable string.
func FormatFindOutput(result *FindResult) string {
	if len(result.Paths) == 0 {
		return "No files found"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d files", result.TotalCount))
	if result.Truncated {
		b.WriteString(" (truncated)")
	}
	b.WriteString(":\n")

	for _, p := range result.Paths {
		b.WriteString(p)
		b.WriteByte('\n')
	}

	return b.String()
}

// FindTimeout is the default timeout for find operations.
const FindTimeout = 60 * time.Second
