package builtin

import (
	"bufio"
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

// Tool names
const (
	ToolRead  = "Read"
	ToolBash  = "Bash"
	ToolEdit  = "Edit"
	ToolWrite = "Write"
	ToolGrep  = "Grep"
	ToolFind  = "Find"
	ToolGlob  = "Glob"
	ToolLs    = "Ls"
)

// ReadResult is the result of a Read operation
type ReadResult struct {
	content   string
	truncated bool
	path      string
}

func (r *ReadResult) Success() string { return r.content }
func (r *ReadResult) Truncated() bool { return r.truncated }
func (r *ReadResult) Path() string    { return r.path }

// Read reads a file and returns its content
func Read(path string, lineRange *[2]int) (*ReadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)

	if lineRange != nil {
		lines := strings.Split(content, "\n")
		start := lineRange[0]
		end := lineRange[1]
		if start < 0 {
			start = 0
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start > end {
			start = end
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return &ReadResult{content: content, path: path}, nil
}

// Write writes content to a file.
// Uses 0666 so that umask determines final permissions (typically 0644).
func Write(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0666)
}

// EditSpec specifies an edit operation on a file
type EditSpec struct {
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	OldString  string `json:"old_string,omitempty"`
	NewString  string `json:"new_string,omitempty"`
	InsertLine int    `json:"insert_line,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
}

// Edit applies an edit to a file (mirrors pi's ReadTool's edit operation)
// editType: "replace", "insert", "delete"
func Edit(edit EditSpec) (string, error) {
	switch edit.Type {
	case "replace":
		return editReplace(edit)
	case "insert":
		return editInsert(edit)
	case "delete":
		return editDelete(edit)
	default:
		return "", fmt.Errorf("unknown edit type: %s", edit.Type)
	}
}

func editReplace(e EditSpec) (string, error) {
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)

	if e.OldString != "" {
		if !strings.Contains(content, e.OldString) {
			return "", fmt.Errorf("old_string not found in file")
		}
		content = strings.Replace(content, e.OldString, e.NewString, 1)
	} else if e.StartLine > 0 && e.EndLine > 0 {
		lines := strings.Split(content, "\n")
		if e.StartLine > len(lines) || e.EndLine > len(lines) {
			return "", fmt.Errorf("line range out of bounds")
		}
		newLines := append(lines[:e.StartLine-1], append(strings.Split(e.NewString, "\n"), lines[e.EndLine:]...)...)
		content = strings.Join(newLines, "\n")
	}

	if err := os.WriteFile(e.Path, []byte(content), 0666); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return "Applied edit", nil
}

func editInsert(e EditSpec) (string, error) {
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)

	lines := strings.Split(content, "\n")
	if e.InsertLine < 1 {
		e.InsertLine = 1
	}
	if e.InsertLine > len(lines) {
		e.InsertLine = len(lines)
	}

	newLines := append(lines[:e.InsertLine], append(strings.Split(e.NewString, "\n"), lines[e.InsertLine:]...)...)
	content = strings.Join(newLines, "\n")

	if err := os.WriteFile(e.Path, []byte(content), 0666); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return "Inserted content", nil
}

func editDelete(e EditSpec) (string, error) {
	return editReplace(EditSpec{
		Type:      "replace",
		Path:      e.Path,
		OldString: e.OldString,
		NewString: "",
	})
}

// getShell returns the appropriate shell binary and argument flag for the current OS.
// Prefers Git Bash on Windows since it properly supports background operators (&).
func getShell() (shell string, arg string) {
	switch runtime.GOOS {
	case "windows":
		gitBashPaths := []string{
			"bash",                                             // in PATH (Git installed)
			`C:\Program Files\Git\bin\bash.exe`,               // standard 64-bit install
			`C:\Program Files (x86)\Git\bin\bash.exe`,         // 32-bit install
			`C:\Program Files\Git\usr\bin\bash.exe`,           // alternate path
		}
		for _, p := range gitBashPaths {
			if _, err := exec.LookPath(p); err == nil {
				return p, "-c"
			}
		}
		// Fallback to cmd.exe if no bash found
		return "cmd.exe", "/c"
	default:
		return "/bin/bash", "-c"
	}
}

// isBackgroundCommand checks if a shell command ends with a background operator (&).
func isBackgroundCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if len(trimmed) < 2 {
		return false
	}
	if trimmed[len(trimmed)-1] != '&' {
		return false
	}
	// Make sure it's not && (logical AND)
	if len(trimmed) >= 2 && trimmed[len(trimmed)-2] == '&' {
		return false
	}
	return true
}

// Bash executes a shell command.
// NOTE: shell -c invocation is intentional — this is a coding agent where
// the LLM generates shell commands that require shell interpretation (pipes,
// redirections, env vars, etc.). The permission system gates which commands
// are allowed to run; the Bash tool itself is not a boundary.
func Bash(cmd string, cwd string, timeout int) (string, error) {
	if cmd == "" {
		return "", fmt.Errorf("Bash: empty command")
	}

	// Apply timeout via context. Default to 60s if not specified.
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	shell, shellArg := getShell()
	execCmd := cmd

	// For background commands (ending with &), wrap in subshell so the parent
	// shell exits immediately without waiting for the backgrounded child process.
	// This prevents cmd.Wait() from blocking indefinitely on background processes.
	if isBackgroundCommand(cmd) {
		execCmd = "( " + strings.TrimSpace(cmd) + " )"
	}

	command := exec.CommandContext(ctx, shell, shellArg, execCmd)
	if cwd != "" {
		command.Dir = cwd
	}

	out, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command timed out after %ds: %w", timeout, ctx.Err())
	}
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// GrepOptions contains options for Grep
type GrepOptions struct {
	MaxResults    int
	CaseSensitive bool
}

// GrepMatch represents a single grep match
type GrepMatch struct {
	path    string
	lineNum int
	content string
}

// Path returns the file path of the match
func (m GrepMatch) Path() string { return m.path }

// LineNum returns the line number of the match
func (m GrepMatch) LineNum() int { return m.lineNum }

// Content returns the matching line content
func (m GrepMatch) Content() string { return m.content }

// Grep searches for a pattern in files.
func Grep(pattern string, paths []string, opts GrepOptions) ([]GrepMatch, error) {
	var matches []GrepMatch

	// Limit pattern length to prevent ReDoS (10KB max)
	if len(pattern) > 10240 {
		return nil, fmt.Errorf("pattern exceeds maximum length of 10240 bytes")
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		// Try as literal
		regex = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 1MB initial, 10MB max
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if regex.MatchString(scanner.Text()) {
				matches = append(matches, GrepMatch{
					path:    path,
					lineNum: lineNum,
					content: scanner.Text(),
				})
				if opts.MaxResults > 0 && len(matches) >= opts.MaxResults {
					file.Close()
					return matches, nil
				}
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, fmt.Errorf("scanning %s: %w", path, err)
		}
		file.Close()
	}

	return matches, nil
}

// Find finds files matching a pattern
func Find(dir, pattern string, maxDepth int) ([]string, error) {
	var results []string
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
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

		if !info.IsDir() && regex.MatchString(info.Name()) {
			results = append(results, path)
		}
		return nil
	})

	return results, err
}

// Glob finds files using glob patterns
func Glob(dir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(dir, pattern)
	return filepath.Glob(fullPattern)
}

// FileInfo represents information about a file
type FileInfo struct {
	Name  string
	IsDir bool
	Size  int64
	Mode  string
}

// Ls lists files in a directory.
// Cleans the path to prevent traversal attacks.
func Ls(dir string) ([]FileInfo, error) {
	dir = filepath.Clean(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Mode:  info.Mode().String(),
		})
	}
	return files, nil
}
