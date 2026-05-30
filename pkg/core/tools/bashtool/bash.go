// Package bashtool provides the Bash tool implementation with streaming, timeout, and process management.
// Aligned to pi's tools/bash.ts.
package bashtool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"miniclaudecode-go/pkg/core/tools/outputaccumulator"
)

const (
	// DefaultTimeoutSecs is the default command timeout in seconds.
	DefaultTimeoutSecs = 120

	// MaxOutputBytes is the maximum bytes of output to capture before truncating.
	MaxOutputBytes = 500 * 1024 // 500KB
)

// BashInput is the input for the Bash tool.
// Aligned to pi's BashToolInput.
type BashInput struct {
	Command string `json:"command"`
	CWD     string `json:"cwd,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // Seconds; 0 = default
	Env     map[string]string `json:"env,omitempty"`
}

// BashDetails holds metadata about a bash execution.
type BashDetails struct {
	ExitCode     int    `json:"exit_code,omitempty"`
	Signal       string `json:"signal,omitempty"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	FullOutputPath string `json:"full_output_path,omitempty"`
}

// BashResult holds the result of a Bash operation.
type BashResult struct {
	Stdout  string      `json:"stdout"`
	Stderr  string      `json:"stderr,omitempty"`
	Error   string      `json:"error,omitempty"`
	Details BashDetails `json:"details"`
}

// BashOperations allows pluggable execution for remote/SSH.
type BashOperations interface {
	Execute(ctx context.Context, cmd string, cwd string, env map[string]string) (*BashResult, error)
}

// ProcessLogger is a callback for logging process execution events.
type ProcessLogger func(stage string, info map[string]string)

var processLogger ProcessLogger

// SetProcessLogger sets the global process logger for bashtool.
func SetProcessLogger(logger ProcessLogger) {
	processLogger = logger
}

// logProcess emits a log event if a logger is configured.
func logProcess(stage string, info map[string]string) {
	if processLogger != nil {
		processLogger(stage, info)
	}
}

// truncateForDisplay truncates a string for display purposes.
func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// LocalBashOperations implements BashOperations for local execution.
type LocalBashOperations struct{}

func (LocalBashOperations) Execute(ctx context.Context, cmd string, cwd string, env map[string]string) (*BashResult, error) {
	return executeLocalCommand(ctx, cmd, cwd, env)
}

// getShell returns the shell binary and flag for the current platform.
func getShell() (shell string, arg string) {
	switch runtime.GOOS {
	case "windows":
		// Try Git Bash by absolute path only, no generic "bash" fallback (which is WSL launcher)
		candidates := []string{
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files (x86)\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
		}
		for _, p := range candidates {
			if _, err := exec.LookPath(p); err == nil {
				return p, "-c"
			}
		}
		return "cmd.exe", "/c"
	default:
		return "/bin/bash", "-c"
	}
}

// isBackgroundCommand checks if a shell command runs in the background.
// Detects Unix-style "cmd &" and Windows-style "start" / "start /B" commands.
func isBackgroundCommand(cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if len(trimmed) < 2 {
		return false
	}
	// Unix: command ending with & (but not &&)
	if trimmed[len(trimmed)-1] == '&' && trimmed[len(trimmed)-2] != '&' {
		return true
	}
	// Windows: start or start /B
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "start ") || strings.HasPrefix(lower, "start\t") {
		return true
	}
	return false
}

// Execute performs the Bash operation.
func Execute(ctx context.Context, input BashInput, ops BashOperations) (*BashResult, error) {
	if input.Command == "" {
		return nil, fmt.Errorf("missing required parameter: command")
	}

	cwd := input.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeoutSecs
	}

	// Create timeout context
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	result, err := ops.Execute(tctx, input.Command, cwd, input.Env)
	if err != nil {
		return result, err
	}

	return result, nil
}

// executeLocalCommand runs a shell command locally.
func executeLocalCommand(ctx context.Context, cmd, cwd string, env map[string]string) (*BashResult, error) {
	start := time.Now()

	shell, shellArg := getShell()
	execCmd := cmd

	// For background commands, handle platform differences
	if isBackgroundCommand(cmd) {
		trimmed := strings.TrimSpace(cmd)
		lower := strings.ToLower(trimmed)
		if runtime.GOOS == "windows" && (strings.HasPrefix(lower, "start ") || strings.HasPrefix(lower, "start\t")) {
			// Windows start command: use cmd.exe directly and return immediately
			shell = "cmd.exe"
			shellArg = "/c"
			execCmd = trimmed
		} else {
			// Unix-style & backgrounding: wrap in subshell
			execCmd = "( " + strings.TrimSpace(cmd) + " )"
		}
	}

	// Log command start
	logProcess("start", map[string]string{
		"shell":   shell,
		"command": truncateForDisplay(cmd, 200),
		"cwd":     cwd,
	})

	command := exec.CommandContext(ctx, shell, shellArg, execCmd)
	command.Dir = cwd

	// Merge environment
	if len(env) > 0 {
		command.Env = os.Environ()
		for k, v := range env {
			command.Env = append(command.Env, k+"="+v)
		}
	}

	// Capture output with accumulation
	stdoutAcc := outputaccumulator.NewOutputAccumulator(outputaccumulator.OutputAccumulatorOptions{
		MaxBytes: MaxOutputBytes,
	})

	var stderrBuf bytes.Buffer

	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// For background commands, return immediately after starting.
	if isBackgroundCommand(cmd) {
		pid := command.Process.Pid
		durationMs := time.Since(start).Milliseconds()
		stdoutPipe.Close()
		stderrPipe.Close()
		return &BashResult{
			Stdout: fmt.Sprintf("[Background process started, PID: %d]", pid),
			Details: BashDetails{
				DurationMs: durationMs,
				ExitCode:   0,
			},
		}, nil
	}

	// Stream stdout and stderr concurrently
	var wg sync.WaitGroup
	wg.Add(2)

		go func() {
			defer wg.Done()
			buf := make([]byte, 32*1024)
			for {
				n, err := stdoutPipe.Read(buf)
				if n > 0 {
					stdoutAcc.Append(buf[:n])
				}
				if err != nil {
					return
				}
			}
		}()

	go func() {
		defer wg.Done()
		io.Copy(&stderrBuf, stderrPipe)
	}()

	wg.Wait()

	waitErr := command.Wait()
	durationMs := time.Since(start).Milliseconds()

	result := &BashResult{
		Stderr: stderrBuf.String(),
		Details: BashDetails{
			DurationMs: durationMs,
		},
	}

	// Finish accumulation and get snapshot
	stdoutAcc.Finish()
	snapshot := stdoutAcc.Snapshot(true)
	result.Stdout = snapshot.Content
	result.Details.Truncated = snapshot.Truncation.Truncated
	if result.Details.Truncated && snapshot.FullOutputPath != "" {
		result.Details.FullOutputPath = snapshot.FullOutputPath
	}

	// Handle exit status
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("command timed out after %dms", durationMs)
			result.Details.ExitCode = -1
		} else if exitErr, ok := waitErr.(*exec.ExitError); ok {
			result.Details.ExitCode = exitErr.ExitCode()
			result.Details.Signal = exitErr.String()
		} else {
			result.Details.ExitCode = -1
			result.Error = waitErr.Error()
		}
	}

	// Log command end
	status := "success"
	if result.Details.ExitCode != 0 {
		status = "error"
	}
	if ctx.Err() == context.DeadlineExceeded {
		status = "timeout"
	}
	logProcess("end", map[string]string{
		"status":    status,
		"exitCode":  fmt.Sprintf("%d", result.Details.ExitCode),
		"duration":  (time.Duration(durationMs) * time.Millisecond).Round(time.Millisecond).String(),
		"outputLen": fmt.Sprintf("%d", len(result.Stdout)),
		"output":    truncateForDisplay(result.Stdout, 500),
	})

	return result, nil
}

// FormatBashOutput formats a bash result for display.
func FormatBashOutput(result *BashResult) string {
	var b strings.Builder

	if result.Stdout != "" {
		b.WriteString(result.Stdout)
	}

	if result.Stderr != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("[stderr]\n")
		b.WriteString(result.Stderr)
	}

	if result.Details.Truncated {
		b.WriteString(fmt.Sprintf("\n[Output truncated. Full output saved to: %s]", result.Details.FullOutputPath))
	}

	if result.Details.ExitCode != 0 {
		b.WriteString(fmt.Sprintf("\n[exit code: %d", result.Details.ExitCode))
		if result.Details.Signal != "" {
			b.WriteString(fmt.Sprintf(", signal: %s", result.Details.Signal))
		}
		b.WriteString("]")
	}

	return b.String()
}

// KillProcess kills a process by PID using platform-appropriate methods.
func KillProcess(pid int) error {
	if pid <= 0 {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		return exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
	default:
		// Kill the process group (negative PID)
		return exec.Command("kill", "-9", fmt.Sprintf("-%d", pid)).Run()
	}
}

// FindProcessByPort finds a PID listening on the given port (best effort).
func FindProcessByPort(port int) (int, error) {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("netstat", "-ano", "-pTCP").Output()
		if err != nil {
			return 0, err
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, fmt.Sprintf(":%d ", port)) && strings.Contains(line, "LISTENING") {
				fields := strings.Fields(line)
				if len(fields) >= 5 {
					var pid int
					fmt.Sscanf(fields[len(fields)-1], "%d", &pid)
					return pid, nil
				}
			}
		}
	default:
		out, err := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-t").Output()
		if err != nil {
			return 0, err
		}
		var pid int
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid)
		return pid, nil
	}

	return 0, fmt.Errorf("no process found on port %d", port)
}

// ShellName returns the name of the current shell for display purposes.
func ShellName() string {
	shell, _ := getShell()
	return filepath.Base(shell)
}