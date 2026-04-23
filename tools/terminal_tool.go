package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// TerminalTool provides terminal session management (tmux/screen).
type TerminalTool struct{}

func (*TerminalTool) Name() string    { return "terminal" }
func (*TerminalTool) Description() string {
	return "Terminal session management. Supports list, new, detach, attach, send, kill, rename for tmux/screen. On Windows, returns an error (Unix-only)."
}

func (*TerminalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation: list, new, detach, attach, send, kill, rename",
				"enum":        []string{"list", "new", "detach", "attach", "send", "kill", "rename"},
			},
			"session": map[string]any{
				"type":        "string",
				"description": "Session name (for attach, send, kill, rename).",
			},
			"session_type": map[string]any{
				"type":        "string",
				"description": "Session type: tmux (default) or screen.",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Command to send to session (for send).",
			},
			"new_name": map[string]any{
				"type":        "string",
				"description": "New session name (for rename).",
			},
		},
		"required": []string{"operation"},
	}
}

func (*TerminalTool) CheckPermissions(params map[string]any) string { return "" }

func (*TerminalTool) Execute(params map[string]any) ToolResult {
	return terminalExecute(context.Background(), params)
}

func (*TerminalTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	return terminalExecute(ctx, params)
}

func terminalExecute(ctx context.Context, params map[string]any) ToolResult {
	operation, _ := params["operation"].(string)
	if operation == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	if runtime.GOOS == "windows" {
		return ToolResult{Output: "Error: terminal tool is not supported on Windows", IsError: true}
	}

	sessionType := "tmux"
	if st, _ := params["session_type"].(string); st != "" {
		sessionType = st
	}

	var result ToolResult
	switch operation {
	case "list":
		result = terminalList(sessionType)
	case "new":
		result = terminalNew(params, sessionType)
	case "detach":
		result = terminalDetach(params, sessionType)
	case "attach":
		result = terminalAttach(params, sessionType)
	case "send":
		result = terminalSend(params, sessionType)
	case "kill":
		result = terminalKill(params, sessionType)
	case "rename":
		result = terminalRename(params, sessionType)
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation: %s", operation), IsError: true}
	}
	return result
}

func terminalList(sessionType string) ToolResult {
	var cmd *exec.Cmd
	if sessionType == "screen" {
		cmd = exec.Command("screen", "-ls")
	} else {
		cmd = exec.Command("tmux", "list-sessions")
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: output, IsError: true}
	}
	return ToolResult{Output: output}
}

func terminalNew(params map[string]any, sessionType string) ToolResult {
	session, _ := params["session"].(string)
	var cmd *exec.Cmd
	if sessionType == "screen" {
		if session != "" {
			cmd = exec.Command("screen", "-dmS", session)
		} else {
			cmd = exec.Command("screen", "-dm")
		}
	} else {
		if session != "" {
			cmd = exec.Command("tmux", "new-session", "-d", "-s", session)
		} else {
			cmd = exec.Command("tmux", "new-session", "-d")
		}
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, output), IsError: true}
	}
	return ToolResult{Output: "Session created: " + output}
}

func terminalDetach(params map[string]any, sessionType string) ToolResult {
	session, _ := params["session"].(string)
	var cmd *exec.Cmd
	if sessionType == "screen" {
		return ToolResult{Output: "Error: screen does not support detach via CLI (use Ctrl-a d)", IsError: false}
	}
	if session == "" {
		cmd = exec.Command("tmux", "detach")
	} else {
		cmd = exec.Command("tmux", "detach", "-t", session)
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, output), IsError: true}
	}
	return ToolResult{Output: "Detached" + output}
}

func terminalAttach(params map[string]any, sessionType string) ToolResult {
	session, _ := params["session"].(string)
	var cmd *exec.Cmd
	if sessionType == "screen" {
		if session != "" {
			cmd = exec.Command("screen", "-r", session)
		} else {
			cmd = exec.Command("screen", "-r")
		}
	} else {
		if session != "" {
			cmd = exec.Command("tmux", "attach-session", "-t", session)
		} else {
			cmd = exec.Command("tmux", "attach-session")
		}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// attach is interactive, will likely fail in non-interactive mode
	if err := cmd.Run(); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, strings.TrimSpace(stderr.String())), IsError: true}
	}
	return ToolResult{Output: strings.TrimSpace(stdout.String())}
}

func terminalSend(params map[string]any, sessionType string) ToolResult {
	session, _ := params["session"].(string)
	command, _ := params["command"].(string)
	if command == "" {
		return ToolResult{Output: "Error: command is required for send", IsError: true}
	}
	if session == "" {
		return ToolResult{Output: "Error: session is required for send", IsError: true}
	}

	var cmd *exec.Cmd
	if sessionType == "screen" {
		cmd = exec.Command("screen", "-S", session, "-X", "stuff", command+"\n")
	} else {
		cmd = exec.Command("tmux", "send-keys", "-t", session, command, "Enter")
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, output), IsError: true}
	}
	return ToolResult{Output: "Command sent: " + command}
}

func terminalKill(params map[string]any, sessionType string) ToolResult {
	session, _ := params["session"].(string)
	if session == "" {
		return ToolResult{Output: "Error: session is required for kill", IsError: true}
	}

	var cmd *exec.Cmd
	if sessionType == "screen" {
		cmd = exec.Command("screen", "-S", session, "-X", "quit")
	} else {
		cmd = exec.Command("tmux", "kill-session", "-t", session)
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, output), IsError: true}
	}
	return ToolResult{Output: "Session killed: " + session}
}

func terminalRename(params map[string]any, sessionType string) ToolResult {
	session, _ := params["session"].(string)
	newName, _ := params["new_name"].(string)
	if newName == "" {
		return ToolResult{Output: "Error: new_name is required for rename", IsError: true}
	}
	if session == "" {
		return ToolResult{Output: "Error: session is required for rename", IsError: true}
	}

	var cmd *exec.Cmd
	if sessionType == "screen" {
		cmd = exec.Command("screen", "-S", session, "-X", "sessionname", newName)
	} else {
		cmd = exec.Command("tmux", "rename-session", "-t", session, newName)
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v\n%s", err, output), IsError: true}
	}
	return ToolResult{Output: fmt.Sprintf("Session renamed from %s to %s", session, newName)}
}
