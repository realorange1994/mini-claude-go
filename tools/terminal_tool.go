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
	return fmt.Sprintf("Terminal session management via tmux or screen. Available: %s. Supports list, new, send, kill, and rename operations. (attach/detach require a real TTY — agent returns instructions)", strings.Join(avail, ", "))
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
