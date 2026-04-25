package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ExecTool executes shell commands with security guards.
type ExecTool struct{}

func (*ExecTool) Name() string { return "exec" }
func (*ExecTool) Description() string {
	return "Execute a shell command. On Windows, use PowerShell syntax (`;` to separate commands, not `&&`). Use for running scripts, installing packages, git operations, and any shell task. Commands run in the current working directory."
}

func (*ExecTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute.",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Working directory for the command (default: current directory).",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 120, max 600).",
			},
		},
		"required": []string{"command"},
	}
}

var denyRegexps = compileDenyPatterns()

func compileDenyPatterns() []*regexp.Regexp {
	patterns := []string{
		`\brm\s+-[rf]{1,2}\b`,                       // rm -r, rm -rf
		`\bdel\s+/[fq]\b`,                            // del /f, del /q
		`\brmdir\s+/s\b`,                             // rmdir /s
		`(?:^|[;&|]\s*)format\b`,                     // format (as standalone command only)
		`\b(mkfs|diskpart)\b`,                        // disk formatting
		`\bdd\s+.*\bof=`,                             // dd with output
		`>\s*/dev/sd`,                                // write to disk device
		`\b(shutdown|reboot|poweroff)\b`,             // power operations
		`:\(\)\s*\{.*\};\s*:`,                        // fork bomb
		`\w+\(\)\s*\{[^}]*\|\s*[^}]*&\s*\}\s*;\s*`,   // fork bomb variation
		`&\S*&\S*&`,                                  // chained background processes
	}
	result := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		result[i] = regexp.MustCompile("(?i)" + p)
	}
	return result
}

func (*ExecTool) CheckPermissions(params map[string]any) string {
	cmd, _ := params["command"].(string)
	cmd = strings.TrimSpace(cmd)
	lower := strings.ToLower(cmd)

	for _, re := range denyRegexps {
		if re.MatchString(lower) {
			return "Dangerous command pattern detected: " + re.String()
		}
	}

	// Warn about commands accessing internal/private URLs
	if containsInternalURL(cmd) {
		return "Internal/private URL detected"
	}

	return ""
}

func (*ExecTool) Execute(params map[string]any) ToolResult {
	return execToolExecute(context.Background(), params)
}

// ExecuteContext runs the command with context support for cancellation.
func (*ExecTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	return execToolExecute(ctx, params)
}

func execToolExecute(ctx context.Context, params map[string]any) ToolResult {
	command, _ := params["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return ToolResult{Output: "Error: empty command", IsError: true}
	}

	timeout := 120
	if t, ok := params["timeout"]; ok {
		switch v := t.(type) {
		case float64:
			timeout = int(v)
		case int:
			timeout = v
		}
	}
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	var shell, flag string
	if runtime.GOOS == "windows" {
		// Prefer PowerShell on Windows, then bash (Git Bash), then cmd
		if _, err := exec.LookPath("powershell"); err == nil {
			shell, flag = "powershell", "-Command"
		} else if _, err := exec.LookPath("bash"); err == nil {
			shell, flag = "bash", "-c"
		} else {
			shell, flag = "cmd", "/C"
		}
	} else {
		shell, flag = "bash", "-c"
	}

	// Determine working directory
	wd, _ := params["working_dir"].(string)
	if wd == "" {
		wd, _ = os.Getwd()
	} else {
		wd = expandPath(wd)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, flag, command)
	cmd.Dir = wd
	cmd.Stdin = nil // Isolate from REPL stdin to prevent interactive prompts

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	if err := cmd.Start(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	// Read outputs concurrently
	type readResult struct {
		data string
		isStderr bool
	}
	outputCh := make(chan readResult, 2)

	go func() {
		data := readLimited(stdout, 50000)
		outputCh <- readResult{data, false}
	}()
	go func() {
		data := readLimited(stderr, 25000)
		outputCh <- readResult{"STDERR:\n" + data, true}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-errCh
		return ToolResult{Output: fmt.Sprintf("Error: command timed out after %ds", timeout), IsError: true}
	case err := <-errCh:
		var stdoutOut, stderrOut string
		for i := 0; i < 2; i++ {
			r := <-outputCh
			if r.isStderr {
				stderrOut = r.data
			} else {
				stdoutOut = r.data
			}
		}

		var result strings.Builder
		if stdoutOut != "" {
			result.WriteString(stdoutOut)
		}
		if stderrOut != "" && stderrOut != "STDERR:\n" {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(stderrOut)
		}

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
		}
		result.WriteString(fmt.Sprintf("\nExit code: %d", exitCode))

		if result.Len() == 0 {
			result.WriteString("(no output)")
		}

		// Truncate if too large
		output := result.String()
		const maxOutput = 50000
		if len(output) > maxOutput {
			half := maxOutput / 2
			truncated := len(output) - maxOutput
			output = output[:half] + fmt.Sprintf("\n\n... (%d chars truncated) ...\n\n", truncated) + output[len(output)-half:]
		}

		return ToolResult{Output: output, IsError: err != nil && !isExitError(err)}
	}
}

func isExitError(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}

func readLimited(r interface{ Read([]byte) (int, error) }, limit int) string {
	buf := make([]byte, limit)
	off := 0
	for {
		n, err := r.Read(buf[off:])
		off += n
		if err != nil {
			break
		}
		if off >= limit {
			break
		}
	}
	return string(buf[:off])
}

// containsInternalURL checks for internal/private URLs.
func containsInternalURL(cmd string) bool {
	lower := strings.ToLower(cmd)
	patterns := []string{
		`https?://(localhost|127\.0\.0\.1|0\.0\.0\.0|192\.168\.\d+\.\d+|10\.\d+\.\d+\.\d+|172\.(1[6-9]|2\d|3[01])\.\d+\.\d+)[:/]`,
		`https?://[0-9]+(?:\.[0-9]+){3}:\d+`,
	}
	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p, lower); matched {
			return true
		}
	}
	return false
}

