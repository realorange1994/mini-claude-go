package tools

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// TerminalTool provides terminal/screen/tmux session management.
type TerminalTool struct{}

func (*TerminalTool) Name() string    { return "terminal" }
func (*TerminalTool) Description() string {
	return "Terminal session management via tmux or screen. Supports list, new, detach, attach, send, kill, and rename operations. Unix/Linux only."
}

func (*TerminalTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"manager": map[string]any{
				"type":        "string",
				"description": "Terminal manager: tmux (default) or screen",
				"enum":        []string{"tmux", "screen"},
			},
			"operation": map[string]any{
				"type":        "string",
				"description": "Operation: list, new, detach, attach, send, kill, rename",
				"enum":        []string{"list", "new", "detach", "attach", "send", "kill", "rename"},
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
		},
		"required": []string{"operation"},
	}
}

func (*TerminalTool) CheckPermissions(params map[string]any) string {
	return ""
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

	operation, _ := params["operation"].(string)
	if operation == "" {
		return ToolResult{Output: "Error: operation is required", IsError: true}
	}

	cmd, err := buildTerminalCommand(manager, operation, params)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return ToolResult{Output: output, IsError: true}
	}
	return ToolResult{Output: output}
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
			cmd = exec.Command("tmux", "new-session", "-s", sessionName)
		} else {
			cmd = exec.Command("screen", "-S", sessionName)
		}
		if cwd, ok := params["cwd"].(string); ok && cwd != "" {
			if manager == "tmux" {
				cmd.Args = append(cmd.Args, "-c", cwd)
			} else {
				cmd.Args = append(cmd.Args, "-c", cwd)
			}
		}

	case "attach":
		if session == "" {
			return nil, fmt.Errorf("session name is required for attach")
		}
		if manager == "tmux" {
			cmd = exec.Command("tmux", "attach-session", "-t", session)
		} else {
			cmd = exec.Command("screen", "-r", session)
		}

	case "detach":
		if manager == "tmux" {
			cmd = exec.Command("tmux", "detach-client")
		} else {
			cmd = exec.Command("screen", "-d")
		}

	case "send":
		if session == "" {
			return nil, fmt.Errorf("session name is required for send")
		}
		command, _ := params["command"].(string)
		if command == "" {
			return nil, fmt.Errorf("command is required for send")
		}
		if manager == "tmux" {
			cmd = exec.Command("tmux", "send-keys", "-t", session, command, "Enter")
		} else {
			cmd = exec.Command("screen", "-S", session, "-X", "stuff", command+"\n")
		}

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
