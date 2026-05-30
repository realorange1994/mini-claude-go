package shellexec

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultTimeout is the default timeout for bash commands in seconds.
	DefaultTimeout = 600

	// MaxOutputBeforeSpill is the maximum output size before spilling to a temp file.
	MaxOutputBeforeSpill = 5 * 1024 * 1024 // 5MB

	// MaxDisplayOutput is the maximum output length shown in process logs.
	MaxDisplayOutput = 500
)

// Result represents the result of a shell execution.
type Result struct {
	stdout   string
	stderr   string
	exitCode int
	timedOut bool
	duration time.Duration
}

// Stdout returns the standard output.
func (r *Result) Stdout() string { return r.stdout }

// Stderr returns the standard error output.
func (r *Result) Stderr() string { return r.stderr }

// ExitCode returns the process exit code.
func (r *Result) ExitCode() int { return r.exitCode }

// TimedOut returns true if the command timed out.
func (r *Result) TimedOut() bool { return r.timedOut }

// Duration returns how long the command ran.
func (r *Result) Duration() time.Duration { return r.duration }

// CombinedOutput returns stdout + stderr combined.
func (r *Result) CombinedOutput() string {
	if r.stderr == "" {
		return r.stdout
	}
	return r.stdout + "\n" + r.stderr
}

// IsSuccess returns true if the command exited with code 0 and did not time out.
func (r *Result) IsSuccess() bool {
	return r.exitCode == 0 && !r.timedOut
}

// Options holds configuration for shell execution.
type Options struct {
	cwd     string
	env     []string
	timeout time.Duration
	stdin   string
}

// Option is a functional option for shell execution.
type Option func(*Options)

// WithCWD sets the working directory.
func WithCWD(cwd string) Option { return func(o *Options) { o.cwd = cwd } }

// WithEnv adds environment variables.
func WithEnv(env map[string]string) Option {
	return func(o *Options) {
		for k, v := range env {
			o.env = append(o.env, k+"="+v)
		}
	}
}

// WithTimeout sets the command timeout.
func WithTimeout(d time.Duration) Option { return func(o *Options) { o.timeout = d } }

// WithStdin sets stdin input.
func WithStdin(s string) Option { return func(o *Options) { o.stdin = s } }

// ProcessLogger is a callback for logging process execution events.
type ProcessLogger func(stage string, info map[string]string)

// Executor handles shell command execution.
type Executor struct {
	logger ProcessLogger
}

// New creates a new shell executor.
func New() *Executor { return &Executor{} }

// SetLogger sets the process logger callback.
func (e *Executor) SetLogger(logger ProcessLogger) {
	e.logger = logger
}

// log emits a log event if a logger is configured.
func (e *Executor) log(stage string, info map[string]string) {
	if e.logger != nil {
		e.logger(stage, info)
	}
}

// truncateForDisplay truncates a string for display purposes.
func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getShell returns the appropriate shell binary and argument flag for the current OS.
func getShell() (shell string, arg string) {
	switch runtime.GOOS {
	case "windows":
		// Prefer Git Bash on Windows
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

// buildCmd constructs an exec.Cmd from a command string and options.
func buildCmd(ctx context.Context, cmd string, options *Options) *exec.Cmd {
	shell, shellArg := getShell()
	cmdObj := exec.CommandContext(ctx, shell, shellArg, cmd)

	if options.cwd != "" {
		cmdObj.Dir = options.cwd
	}
	if len(options.env) > 0 {
		cmdObj.Env = append(os.Environ(), options.env...)
	}
	if options.stdin != "" {
		cmdObj.Stdin = strings.NewReader(options.stdin)
	}

	return cmdObj
}

// isBackgroundCommand checks if a shell command ends with a background operator (&).
// It distinguishes & (background) from && (logical AND) and other operators like |, >, etc.
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

// wrapBackgroundCommand wraps a background command in a subshell so the parent
// shell exits immediately instead of waiting for the backgrounded child.
// E.g., "go run server.go &" becomes "( go run server.go & )"
func wrapBackgroundCommand(cmd string) string {
	return "( " + strings.TrimSpace(cmd) + " )"
}

// Execute runs a shell command and returns the result.
// It captures stdout and stderr into buffers, spilling to a temp file if output
// exceeds MaxOutputBeforeSpill.
// For background commands (ending with &), the shell is wrapped in a subshell
// so it exits immediately without waiting for backgrounded children.
func (e *Executor) Execute(cmd string, opts ...Option) (*Result, error) {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}

	if options.timeout == 0 {
		options.timeout = DefaultTimeout * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()

	// For background commands, wrap in subshell so parent shell exits immediately.
	execCmd := cmd
	if isBackgroundCommand(cmd) {
		execCmd = wrapBackgroundCommand(cmd)
	}

	cmdObj := buildCmd(ctx, execCmd, &options)

	// Use spillover writers for large output
	outW := newSpillWriter(MaxOutputBeforeSpill)
	defer outW.Close()
	errW := newSpillWriter(MaxOutputBeforeSpill)
	defer errW.Close()

	cmdObj.Stdout = outW
	cmdObj.Stderr = errW

	start := time.Now()
	runErr := cmdObj.Run()
	duration := time.Since(start)

	result := &Result{
		stdout:   outW.String(),
		stderr:   errW.String(),
		duration: duration,
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.timedOut = true
	}

	if exitErr, ok := runErr.(*exec.ExitError); ok {
		result.exitCode = exitErr.ExitCode()
	} else if runErr != nil && ctx.Err() == nil {
		// Non-exit error (e.g. command not found) — report as exit code 1
		result.exitCode = 1
	}

	return result, nil
}

// ExecuteStreaming runs a command with line-by-line streaming output callbacks.
// The streamCb function is called for each line of output, with isStdout indicating
// whether the line came from stdout (true) or stderr (false).
func (e *Executor) ExecuteStreaming(cmd string, streamCb func(line string, isStdout bool), opts ...Option) (*Result, error) {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}

	if options.timeout == 0 {
		options.timeout = DefaultTimeout * time.Second
	}

	// Log command start
	shell, _ := getShell()
	e.log("start", map[string]string{
		"shell":   shell,
		"command": truncateForDisplay(cmd, 200),
		"cwd":     options.cwd,
		"timeout": options.timeout.String(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), options.timeout)
	defer cancel()

	cmdObj := buildCmd(ctx, cmd, &options)

	stdoutPipe, err := cmdObj.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderrPipe, err := cmdObj.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		cancel()
		return nil, err
	}

	start := time.Now()
	if err := cmdObj.Start(); err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	var stdoutBuf, stderrBuf bytes.Buffer
	var stdoutMu, stderrMu sync.Mutex

	streamLine := func(r io.Reader, isStdout bool) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			if streamCb != nil {
				streamCb(line, isStdout)
			}
			if isStdout {
				stdoutMu.Lock()
				stdoutBuf.WriteString(line + "\n")
				stdoutMu.Unlock()
			} else {
				stderrMu.Lock()
				stderrBuf.WriteString(line + "\n")
				stderrMu.Unlock()
			}
		}
	}

	wg.Add(2)
	go streamLine(stdoutPipe, true)
	go streamLine(stderrPipe, false)
	wg.Wait()

	// Wait for the process to finish and collect its exit code
	waitErr := cmdObj.Wait()
	duration := time.Since(start)

	result := &Result{
		stdout:   stdoutBuf.String(),
		stderr:   stderrBuf.String(),
		duration: duration,
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.timedOut = true
	}

	if exitErr, ok := waitErr.(*exec.ExitError); ok {
		result.exitCode = exitErr.ExitCode()
	} else if waitErr != nil && ctx.Err() == nil {
		result.exitCode = 1
	}

	// Log command end
	status := "success"
	if result.exitCode != 0 {
		status = "error"
	}
	if result.timedOut {
		status = "timeout"
	}
	e.log("end", map[string]string{
		"status":    status,
		"exitCode":  fmt.Sprintf("%d", result.exitCode),
		"duration":  duration.Round(time.Millisecond).String(),
		"outputLen": fmt.Sprintf("%d", len(result.stdout)),
		"output":    truncateForDisplay(result.stdout, MaxDisplayOutput),
	})

	return result, nil
}

// ---------------------------------------------------------------------------
// spillWriter: in-memory buffer that overflows to a temp file when threshold
// is exceeded. This avoids unbounded memory consumption from noisy commands.
// ---------------------------------------------------------------------------

type spillWriter struct {
	buf       bytes.Buffer
	tmpFile   *os.File
	threshold int
	spilled   bool
}

func newSpillWriter(threshold int) *spillWriter {
	return &spillWriter{threshold: threshold}
}

func (sw *spillWriter) Write(p []byte) (int, error) {
	if sw.spilled {
		return sw.tmpFile.Write(p)
	}
	if sw.buf.Len()+len(p) > sw.threshold {
		// Spill to temp file
		tmp, err := os.CreateTemp("", "shellexec-*.txt")
		if err != nil {
			// Fallback: keep buffering in memory
			return sw.buf.Write(p)
		}
		sw.tmpFile = tmp
		sw.spilled = true
		// Copy existing buffer contents to file
		if sw.buf.Len() > 0 {
			if _, err := sw.tmpFile.Write(sw.buf.Bytes()); err != nil {
				return 0, err
			}
			sw.buf.Reset()
		}
		return sw.tmpFile.Write(p)
	}
	return sw.buf.Write(p)
}

func (sw *spillWriter) String() string {
	if sw.spilled && sw.tmpFile != nil {
		data, err := os.ReadFile(sw.tmpFile.Name())
		if err != nil {
			return ""
		}
		return string(data)
	}
	return sw.buf.String()
}

func (sw *spillWriter) Close() {
	if sw.tmpFile != nil {
		sw.tmpFile.Close()
		os.Remove(sw.tmpFile.Name())
	}
}
