// Package greptool provides the Grep tool implementation using ripgrep.
// Aligned to pi's tools/grep.ts.
package greptool

import (
	"bufio"
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

	"miniclaudecode-go/pkg/core/tools/truncate"
)

const (
	// DefaultMaxResults is the default maximum number of matches.
	DefaultMaxResults = 100

	// DefaultContextLines is the default number of context lines.
	DefaultContextLines = 0

	// MaxPatternLength is the maximum pattern length to prevent ReDoS.
	MaxPatternLength = 10240
)

// GrepInput is the input for the Grep tool.
// Aligned to pi's GrepToolInput.
type GrepInput struct {
	Pattern       string   `json:"pattern"`
	Paths         []string `json:"paths,omitempty"`
	MaxResults    int      `json:"max_results,omitempty"`
	CaseSensitive bool     `json:"case_sensitive,omitempty"`
	ContextLines  int      `json:"context,omitempty"`      // Lines before/after match
	Glob          string   `json:"glob,omitempty"`         // Glob filter for file names
	Type          string   `json:"type,omitempty"`         // File type filter (js, py, etc.)
	HeadLimit     int      `json:"head_limit,omitempty"`   // Limit output lines from head
	Offset        int      `json:"offset,omitempty"`       // Skip first N matches
}

// GrepMatch represents a single grep match.
type GrepMatch struct {
	Path        string `json:"path"`
	LineNum     int    `json:"line_num"`
	Column      int    `json:"column,omitempty"`
	Content     string `json:"content"`
	MatchStart  int    `json:"match_start,omitempty"`
	MatchEnd    int    `json:"match_end,omitempty"`
	IsTruncated bool   `json:"is_truncated,omitempty"`
}

// GrepResult holds the result of a Grep operation.
type GrepResult struct {
	Matches     []GrepMatch `json:"matches"`
	TotalCount  int         `json:"total_count,omitempty"` // Total matches (may be > len(Matches))
	Truncated   bool        `json:"truncated,omitempty"`
	FilesSearched int       `json:"files_searched,omitempty"`
}

// GrepOperations allows pluggable execution for remote/SSH.
type GrepOperations interface {
	Execute(ctx context.Context, args []string, cwd string) ([]byte, error)
}

// LocalGrepOperations implements GrepOperations using local ripgrep.
type LocalGrepOperations struct {
	RgPath string // Path to rg binary; empty means use PATH
}

func (op LocalGrepOperations) Execute(ctx context.Context, args []string, cwd string) ([]byte, error) {
	rgPath := op.RgPath
	if rgPath == "" {
		rgPath = "rg"
	}

	cmd := exec.CommandContext(ctx, rgPath, args...)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// rg returns exit code 1 when no matches found (not an error)
		if cmd.ProcessState.ExitCode() == 1 {
			return nil, nil // No matches
		}
		return nil, fmt.Errorf("rg failed: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// findRgBinary attempts to locate ripgrep on the system.
func findRgBinary() string {
	// Check PATH first
	if path, err := exec.LookPath("rg"); err == nil {
		return path
	}

	// Check common locations
	candidates := []string{
		"rg",
		"/usr/bin/rg",
		"/usr/local/bin/rg",
	}

	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			`C:\Program Files\ripgrep\rg.exe`,
			`C:\Program Files (x86)\ripgrep\rg.exe`,
			filepath.Join(os.Getenv("USERPROFILE"), ".cargo", "bin", "rg.exe"),
			filepath.Join(os.Getenv("USERPROFILE"), "scoop", "shims", "rg.exe"),
		)
	} else {
		home, _ := os.UserHomeDir()
		candidates = append(candidates,
			filepath.Join(home, ".cargo", "bin", "rg"),
			filepath.Join(home, ".local", "bin", "rg"),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	return "" // Not found
}

// HasRipgrep checks if ripgrep is available on the system.
func HasRipgrep() bool {
	return findRgBinary() != ""
}

// Execute performs the Grep operation using ripgrep.
func Execute(ctx context.Context, input GrepInput, cwd string, ops GrepOperations) (*GrepResult, error) {
	if input.Pattern == "" {
		return nil, fmt.Errorf("missing required parameter: pattern")
	}

	if len(input.Pattern) > MaxPatternLength {
		return nil, fmt.Errorf("pattern exceeds maximum length of %d bytes", MaxPatternLength)
	}

	// Default paths
	paths := input.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Default max results
	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = DefaultMaxResults
	}

	// Build rg arguments
	args := buildRgArgs(input, maxResults)

	// Execute
	output, err := ops.Execute(ctx, args, cwd)
	if err != nil {
		return nil, err
	}

	if len(output) == 0 {
		return &GrepResult{Matches: nil, TotalCount: 0}, nil
	}

	// Parse output
	return parseRgOutput(output, maxResults), nil
}

// buildRgArgs constructs the argument list for ripgrep.
func buildRgArgs(input GrepInput, maxResults int) []string {
	args := []string{
		"--line-number",      // Show line numbers
		"--column",           // Show column numbers
		"--with-filename",    // Show file paths
		"--color=never",      // No ANSI colors
		"--no-heading",       // Don't group by file
		"--max-count", fmt.Sprintf("%d", maxResults), // Limit matches
	}

	// Case sensitivity
	if !input.CaseSensitive {
		args = append(args, "--ignore-case")
	}

	// Context lines
	if input.ContextLines > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", input.ContextLines))
	}

	// Glob filter
	if input.Glob != "" {
		args = append(args, "--glob", input.Glob)
	}

	// Type filter
	if input.Type != "" {
		args = append(args, "--type", input.Type)
	}

	// Head limit (truncate output lines)
	if input.HeadLimit > 0 {
		args = append(args, "--max-lines", fmt.Sprintf("%d", input.HeadLimit))
	}

	// The pattern
	args = append(args, "--", input.Pattern)

	// Paths
	args = append(args, input.Paths...)

	return args
}

// parseRgOutput parses ripgrep's output into GrepMatch structs.
// Format: path:line:column:content
func parseRgOutput(output []byte, maxResults int) *GrepResult {
	lines := bytes.Split(output, []byte{'\n'})

	var matches []GrepMatch
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		match := parseRgLine(string(line))
		if match != nil {
			matches = append(matches, *match)
			if len(matches) >= maxResults {
				break
			}
		}
	}

	return &GrepResult{
		Matches:    matches,
		TotalCount: len(matches),
		Truncated:  len(matches) >= maxResults,
	}
}

// parseRgLine parses a single line of rg output.
// Format: path:line:column:content or path:line:content (older rg versions)
func parseRgLine(line string) *GrepMatch {
	// Find the last colon that separates content from metadata
	// Format: path:line:column:content

	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 3 {
		return nil
	}

	path := parts[0]
	lineNum := parseInt(parts[1], 0)
	column := 0
	content := ""

	if len(parts) == 4 {
		column = parseInt(parts[2], 0)
		content = parts[3]
	} else if len(parts) == 3 {
		content = parts[2]
	}

	// Truncate long lines
	truncated := false
	if len(content) > truncate.GrepMaxLineLength {
		content = content[:truncate.GrepMaxLineLength] + "... [truncated]"
		truncated = true
	}

	return &GrepMatch{
		Path:        path,
		LineNum:     lineNum,
		Column:      column,
		Content:     content,
		IsTruncated: truncated,
	}
}

// parseInt parses a string to int, returning default on failure.
func parseInt(s string, def int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

// ExecuteWithFallback performs grep using ripgrep if available, otherwise falls back to Go regex.
func ExecuteWithFallback(ctx context.Context, input GrepInput, cwd string) (*GrepResult, error) {
	if HasRipgrep() {
		ops := LocalGrepOperations{}
		return Execute(ctx, input, cwd, ops)
	}

	// Fallback to Go-based grep
	return executeGoGrep(ctx, input, cwd)
}

// executeGoGrep is a pure-Go fallback when ripgrep is not available.
func executeGoGrep(ctx context.Context, input GrepInput, cwd string) (*GrepResult, error) {
	// Default paths
	paths := input.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// Default max results
	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = DefaultMaxResults
	}

	// Compile pattern
	var regex *regexp.Regexp
	var err error

	pattern := input.Pattern
	if !input.CaseSensitive {
		pattern = "(?i)" + pattern
	}

	regex, err = regexp.Compile(pattern)
	if err != nil {
		// Try as literal
		regex = regexp.MustCompile(regexp.QuoteMeta(input.Pattern))
	}

	var matches []GrepMatch

	for _, path := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		absPath := path
		if !filepath.IsAbs(path) {
			absPath = filepath.Join(cwd, path)
		}

		fileMatches, err := grepFile(ctx, absPath, regex, maxResults-len(matches))
		if err != nil {
			continue // Skip files we can't read
		}

		matches = append(matches, fileMatches...)
		if len(matches) >= maxResults {
			break
		}
	}

	return &GrepResult{
		Matches:    matches,
		TotalCount: len(matches),
		Truncated:  len(matches) >= maxResults,
	}, nil
}

// grepFile searches a single file for matches.
func grepFile(ctx context.Context, path string, regex *regexp.Regexp, limit int) ([]GrepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []GrepMatch
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 1MB initial, 10MB max

	lineNum := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return matches, ctx.Err()
		default:
		}

		lineNum++
		line := scanner.Text()

		if regex.MatchString(line) {
			// Truncate long lines
			content := line
			truncated := false
			if len(content) > truncate.GrepMaxLineLength {
				content = content[:truncate.GrepMaxLineLength] + "... [truncated]"
				truncated = true
			}

			matches = append(matches, GrepMatch{
				Path:        path,
				LineNum:     lineNum,
				Content:     content,
				IsTruncated: truncated,
			})

			if len(matches) >= limit {
				break
			}
		}
	}

	return matches, scanner.Err()
}

// FormatGrepOutput formats grep results as a human-readable string.
func FormatGrepOutput(result *GrepResult) string {
	if len(result.Matches) == 0 {
		return "No matches found"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d matches", result.TotalCount))
	if result.Truncated {
		b.WriteString(" (truncated)")
	}
	b.WriteString(":\n")

	for _, m := range result.Matches {
		b.WriteString(fmt.Sprintf("%s:%d: %s\n", m.Path, m.LineNum, m.Content))
	}

	return b.String()
}

// GrepTimeout is the default timeout for grep operations.
const GrepTimeout = 60 * time.Second
