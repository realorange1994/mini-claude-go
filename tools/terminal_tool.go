package tools

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// TerminalTool provides terminal/screen/tmux session management.
type TerminalTool struct{}

func (*TerminalTool) Name() string { return "terminal" }
func (*TerminalTool) Description() string {
	if runtime.GOOS == "windows" {
		return "Terminal session management: NOT AVAILABLE on Windows (requires tmux/screen, Unix-only)."
	}
	// Pre-check: verify at least one terminal manager is available
	hasTmux := exec.Command("tmux", "-V").Run() == nil
	hasScreen := exec.Command("screen", "--version").Run() == nil
	if !hasTmux && !hasScreen {
		return "Terminal session management: NOT AVAILABLE — neither tmux nor screen found in PATH."
	}
	var avail []string
	if hasTmux {
		avail = append(avail, "tmux")
	}
	if hasScreen {
		avail = append(avail, "screen")
	}
	return fmt.Sprintf(
		"Terminal session management via tmux (preferred) or screen. Available: %s. "+
			"Operations: list, new, send (with output capture), kill, rename. "+
			"Send captures pane output automatically — use capture_mode='poll' for long commands to wait for completion. "+
			"attach/detach return manual instructions (agent process has no TTY).",
		strings.Join(avail, ", "),
	)
}

func (*TerminalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"manager": map[string]any{
				"type":        "string",
				"description": "Terminal manager: tmux (default) or screen",
				"enum":        []any{"tmux", "screen"},
			},
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation: list, new, detach, attach, send, kill, rename",
				"enum":        []any{"list", "new", "detach", "attach", "send", "kill", "rename"},
			},
			"session": map[string]any{
				"type":        "string",
				"description": "Session name (for attach, send, kill, rename)",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Command to send to session (for send operation)",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Working directory for new session",
			},
			"new_name": map[string]any{
				"type":        "string",
				"description": "New session name (for rename operation)",
			},
			"wait_ms": map[string]any{
				"type":        "integer",
				"description": "Milliseconds to wait for command completion before capturing output (default: 500). For long commands, increase or use capture_mode: \"poll\".",
			},
			"capture_mode": map[string]any{
				"type":        "string",
				"description": "Output capture mode for send: \"tail\" (default, captures last N lines) or \"poll\" (waits for sentinel marker, more reliable for async commands).",
				"enum":        []any{"tail", "poll"},
			},
			"capture_lines": map[string]any{
				"type":        "integer",
				"description": "Number of lines to capture from the end of output (default: 50, max: 200).",
			},
		},
		"required": []string{"operation"},
	}
}

func (*TerminalTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

func (*TerminalTool) Execute(params map[string]any) ToolResult {
	if runtime.GOOS == "windows" {
		return ToolResult{
			Output:  "Error: terminal tool is not supported on Windows. It requires tmux or screen which are Unix/Linux tools.",
			IsError: true,
		}
	}

	manager, _ := params["manager"].(string)
	if manager == "" {
		manager = "tmux"
	}

	// Verify the requested manager is actually installed
	if err := checkManagerAvailable(manager); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	operation, _ := params["operation"].(string)
	if operation == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	// attach requires a real TTY, which the agent process does not have.
	// Return instructions for the user to attach manually.
	if operation == "attach" {
		session, _ := params["session"].(string)
		if session == "" {
			if manager == "tmux" {
				return ToolResult{Output: "To attach to a tmux session, run:\n  tmux attach\n  or: tmux attach -t <session-name>"}
			}
			return ToolResult{Output: "To attach to a screen session, run:\n  screen -r\n  or: screen -r <session-name>"}
		}
		if manager == "tmux" {
			return ToolResult{Output: fmt.Sprintf("To attach to tmux session %q, run:\n  tmux attach -t %s", session, session)}
		}
		return ToolResult{Output: fmt.Sprintf("To attach to screen session %q, run:\n  screen -r %s", session, session)}
	}

	// send operation: enhanced with output capture for agent-friendly results
	if operation == "send" {
		return executeSendWithCapture(manager, params)
	}

	cmd, err := buildTerminalCommand(manager, operation, params)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	// detach in a non-interactive agent context: "no current client" is expected
	// (the agent process is never attached to a tmux/screen session)
	if operation == "detach" && err != nil {
		if strings.Contains(output, "no current client") || strings.Contains(output, "No current client") {
			return ToolResult{Output: "No client attached to detach (this is normal — the agent process does not attach to sessions)."}
		}
		return ToolResult{Output: output, IsError: true}
	}

	if err != nil {
		return ToolResult{Output: output, IsError: true}
	}
	return ToolResult{Output: output}
}

// checkManagerAvailable verifies that the requested terminal manager binary exists.
func checkManagerAvailable(manager string) error {
	var cmd *exec.Cmd
	switch manager {
	case "tmux":
		cmd = exec.Command("tmux", "-V")
	case "screen":
		cmd = exec.Command("screen", "--version")
	default:
		return fmt.Errorf("unknown terminal manager: %s", manager)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s is not available in PATH (or returned non-zero exit code)", manager)
	}
	return nil
}

// executeSendWithCapture sends a command to a tmux/screen session and captures the output.
// Two modes:
//   - "tail" (default): sends command, waits briefly, captures last N lines of pane output
//   - "poll": sends command with a sentinel marker, polls until marker appears, then extracts output
func executeSendWithCapture(manager string, params map[string]any) ToolResult {
	session, _ := params["session"].(string)
	if session == "" {
		return ToolResult{Output: "Error: session is required for send", IsError: true}
	}
	command, _ := params["command"].(string)
	if command == "" {
		return ToolResult{Output: "Error: command is required for send", IsError: true}
	}

	captureMode, _ := params["capture_mode"].(string)
	if captureMode == "" {
		captureMode = "tail"
	}
	if captureMode != "tail" && captureMode != "poll" {
		return ToolResult{Output: fmt.Sprintf("Error: unknown capture_mode %q (use 'tail' or 'poll')", captureMode), IsError: true}
	}

	captureLines := 50
	if cl, ok := params["capture_lines"]; ok {
		switch v := cl.(type) {
		case float64:
			captureLines = int(v)
		case int:
			captureLines = v
		}
	}
	if captureLines < 1 {
		captureLines = 50
	}
	if captureLines > 200 {
		captureLines = 200
	}

	waitMs := 500
	if wm, ok := params["wait_ms"]; ok {
		switch v := wm.(type) {
		case float64:
			waitMs = int(v)
		case int:
			waitMs = v
		}
	}
	if waitMs < 50 {
		waitMs = 50
	}
	if waitMs > 30000 {
		waitMs = 30000
	}

	if manager == "tmux" {
		if captureMode == "poll" {
			return captureSendTmuxPoll(session, command, params)
		}
		return captureSendTmuxTail(session, command, waitMs, captureLines)
	}

	// screen: simpler capture
	if captureMode == "poll" {
		return captureSendScreenPoll(session, command, params)
	}
	return captureSendScreenTail(session, command, waitMs, captureLines)
}

// captureSendTmuxTail sends command and captures last N lines after a brief wait.
func captureSendTmuxTail(session, command string, waitMs, captureLines int) ToolResult {
	// Send the command
	sendCmd := exec.Command("tmux", "send-keys", "-t", session, command, "Enter")
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error sending command: %s", strings.TrimSpace(string(out))), IsError: true}
	}

	// Wait for command to execute
	time.Sleep(time.Duration(waitMs) * time.Millisecond)

	// Capture last N lines of the pane
	capLines := strconv.Itoa(captureLines)
	captureCmd := exec.Command("tmux", "capture-pane", "-p", "-S", "-"+capLines, "-t", session)
	captureOut, captureErr := captureCmd.CombinedOutput()
	captured := strings.TrimRight(string(captureOut), "\n")

	if captureErr != nil {
		// capture-pane may fail if session just started and pane isn't ready
		// Command was sent successfully, just can't capture yet
		return ToolResult{
			Output: fmt.Sprintf("Command sent to session %q successfully.\nOutput capture unavailable (%v).", session, strings.TrimSpace(string(captureOut))),
		}
	}

	if captured == "" {
		return ToolResult{
			Output: fmt.Sprintf("Command sent to session %q.\nNo output captured (command may produce no visible output or need more time — increase wait_ms if needed).", session),
		}
	}

	return ToolResult{Output: fmt.Sprintf("Command sent to session %q. Last %d lines of output:\n%s", session, captureLines, captured)}
}

// captureSendTmuxPoll sends command with a sentinel and polls for completion.
func captureSendTmuxPoll(session, command string, params map[string]any) ToolResult {
	const sentinel = "__TACOS_END__"

	// We'll send the command, then the sentinel with exit code
	sendCmd := exec.Command("tmux", "send-keys", "-t", session, command, "Enter")
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error sending command: %s", strings.TrimSpace(string(out))), IsError: true}
	}

	// Send the sentinel marker with exit code
	markerLine := fmt.Sprintf("echo; echo %s $?; echo %s_END", sentinel, sentinel)
	markerCmd := exec.Command("tmux", "send-keys", "-t", session, markerLine, "Enter")
	markerOut, markerErr := markerCmd.CombinedOutput()
	if markerErr != nil {
		return ToolResult{Output: fmt.Sprintf("Error sending marker: %s", strings.TrimSpace(string(markerOut))), IsError: true}
	}

	captureLines := 50
	if cl, ok := params["capture_lines"]; ok {
		switch v := cl.(type) {
		case float64:
			captureLines = int(v)
		case int:
			captureLines = v
		}
	}
	if captureLines < 1 {
		captureLines = 50
	}
	if captureLines > 200 {
		captureLines = 200
	}

	// Poll for the sentinel to appear
	const maxWait = 60 // seconds
	const pollInterval = 200 * time.Millisecond
	deadline := time.Now().Add(maxWait * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		// Capture enough lines to find the sentinel
		fullCapture := strconv.Itoa(captureLines + 10)
		captureCmd := exec.Command("tmux", "capture-pane", "-p", "-S", "-"+fullCapture, "-t", session)
		captureOut, err := captureCmd.CombinedOutput()
		if err != nil {
			continue
		}

		captured := string(captureOut)
		if strings.Contains(captured, sentinel) {
			// Parse exit code from sentinel line
			exitCode := -1
			for _, line := range strings.Split(captured, "\n") {
				if strings.HasPrefix(line, sentinel+" ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						if code, e := strconv.Atoi(parts[1]); e == nil {
							exitCode = code
						}
					}
					break
				}
			}

			// Extract output before sentinel
			beforeSentinel := extractBeforeSentinel(captured, sentinel)

			var sb strings.Builder
			fmt.Fprintf(&sb, "Command sent to session %q.\n", session)
			if exitCode >= 0 {
				fmt.Fprintf(&sb, "Exit code: %d\n", exitCode)
			}
			if beforeSentinel != "" {
				fmt.Fprintf(&sb, "\nOutput:\n%s", beforeSentinel)
			} else {
				fmt.Fprintf(&sb, "No output produced.")
			}
			return ToolResult{Output: sb.String()}
		}
	}

	// Sentinel didn't appear within timeout - command is still running
	return ToolResult{
		Output: fmt.Sprintf("Command sent to session %q.\nCommand is still running (sentinel marker did not appear within %ds). Use operation:send with capture_mode:tail to check output, or increase wait_ms.", session, maxWait),
	}
}

// captureSendScreenTail: screen version of tail capture.
func captureSendScreenTail(session, command string, waitMs, captureLines int) ToolResult {
	sendCmd := exec.Command("screen", "-S", session, "-X", "stuff", command+"\n")
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error sending command: %s", strings.TrimSpace(string(out))), IsError: true}
	}

	time.Sleep(time.Duration(waitMs) * time.Millisecond)

	// screen doesn't have capture-pane; capture output is not reliably available
	return ToolResult{
		Output: fmt.Sprintf("Command sent to screen session %q.\nNote: screen does not support output capture like tmux. Use tmux for captured output feedback.", session),
	}
}

// captureSendScreenPoll: screen version of poll capture.
func captureSendScreenPoll(session, command string, _ map[string]any) ToolResult {
	// screen doesn't support capture-pane, so poll mode is not reliable
	sendCmd := exec.Command("screen", "-S", session, "-X", "stuff", command+"\n")
	out, err := sendCmd.CombinedOutput()
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error sending command: %s", strings.TrimSpace(string(out))), IsError: true}
	}
	return ToolResult{
		Output: fmt.Sprintf("Command sent to screen session %q.\nNote: screen does not support output capture. Use tmux for captured output feedback.", session),
	}
}

// extractBeforeSentinel extracts lines from captured output before the sentinel marker,
// and trims the echoed command line and trailing blank lines.
func extractBeforeSentinel(captured, sentinel string) string {
	lines := strings.Split(captured, "\n")
	var result []string
	for _, line := range lines {
		if strings.HasPrefix(line, sentinel) {
			break
		}
		result = append(result, line)
	}
	// Trim trailing blank lines
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	// Remove first line if it looks like the echoed command (starts with the command text)
	if len(result) > 0 {
		// The echoed command line in screen/pane often appears as the command itself
		// We skip lines that are empty or match the start of the command
		firstNonEmpty := 0
		for i, l := range result {
			if strings.TrimSpace(l) != "" {
				firstNonEmpty = i
				break
			}
		}
		// If first non-empty line starts with the command, skip it (it's the echoed input)
		cmdStart := strings.TrimSpace(result[firstNonEmpty])
		if strings.HasPrefix(cmdStart, "echo; echo "+sentinel) {
			// Only the marker line, no real command echo
			result = result[:0]
		}
	}
	return strings.Join(result, "\n")
}

func buildTerminalCommand(manager, operation string, params map[string]any) (*exec.Cmd, error) {
	session, _ := params["session"].(string)
	var cmd *exec.Cmd

	switch operation {
	case "list":
		if manager == "tmux" {
			cmd = exec.Command("tmux", "list-sessions")
		} else {
			cmd = exec.Command("screen", "-ls")
		}

	case "new":
		sessionName := session
		if sessionName == "" {
			sessionName = "main"
		}
		if manager == "tmux" {
			// -d: detached mode (no TTY required)
			cmd = exec.Command("tmux", "new-session", "-d", "-s", sessionName)
		} else {
			// -dm: detached mode, -S: session name
			cmd = exec.Command("screen", "-dmS", sessionName)
		}
		if cwd, ok := params["cwd"].(string); ok && cwd != "" {
			if manager == "tmux" {
				cmd.Args = append(cmd.Args, "-c", cwd)
			}
			// screen: cwd must be set via Dir field, not -c flag
			cmd.Dir = cwd
		}

	case "detach":
		if manager == "tmux" {
			cmd = exec.Command("tmux", "detach-client")
		} else {
			cmd = exec.Command("screen", "-d")
		}

	case "send":
		// send is handled by executeSendWithCapture, not buildTerminalCommand
		return nil, fmt.Errorf("send operation is handled separately with output capture")

	case "kill":
		if session == "" {
			return nil, fmt.Errorf("session name is required for kill")
		}
		if manager == "tmux" {
			cmd = exec.Command("tmux", "kill-session", "-t", session)
		} else {
			cmd = exec.Command("screen", "-S", session, "-X", "quit")
		}

	case "rename":
		if session == "" {
			return nil, fmt.Errorf("session name is required for rename")
		}
		newName, _ := params["new_name"].(string)
		if newName == "" {
			return nil, fmt.Errorf("new_name is required for rename")
		}
		if manager == "tmux" {
			cmd = exec.Command("tmux", "rename-session", "-t", session, newName)
		} else {
			cmd = exec.Command("screen", "-S", session, "-X", "sessionname", newName)
		}

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	return cmd, nil
}
