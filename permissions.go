package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"miniclaudecode-go/tools"
)

// PermissionDenied is returned when a tool call is blocked.
type PermissionDenied struct {
	Reason string
}

func (e PermissionDenied) Error() string { return e.Reason }

// PermissionGate implements the two-layer permission check.
type PermissionGate struct {
	config *Config
}

// NewPermissionGate creates a new gate.
func NewPermissionGate(cfg *Config) *PermissionGate {
	return &PermissionGate{config: cfg}
}

// Check runs the permission gauntlet. Returns a ToolResult if denied, nil if allowed.
func (g *PermissionGate) Check(tool tools.Tool, params map[string]any) *tools.ToolResult {
	// Layer 1: tool-level self-check (returns warning, not hard denial)
	warning := tool.CheckPermissions(params)

	// UNCONDITIONAL: Block if tool's own security check fails, regardless of mode.
	if warning != "" {
		return &tools.ToolResult{
			Output:  fmt.Sprintf("Permission denied: %s", warning),
			IsError: true,
		}
	}

	// Layer 1.5: denied patterns check (hard denial)
	if len(g.config.DeniedPatterns) > 0 {
		var target string
		switch tool.Name() {
		case "exec":
			target, _ = params["command"].(string)
		case "write_file", "edit_file", "multi_edit", "fileops":
			target, _ = params["path"].(string)
		}
		if target != "" {
			lower := strings.ToLower(target)
			for _, pattern := range g.config.DeniedPatterns {
				if strings.Contains(lower, strings.ToLower(pattern)) {
					return &tools.ToolResult{
						Output:  fmt.Sprintf("Permission denied: matches denied pattern %q", pattern),
						IsError: true,
					}
				}
			}
		}
	}

	// Layer 2: mode-based check
	switch g.config.PermissionMode {
	case ModePlan:
		writeTools := map[string]bool{"exec": true, "write_file": true, "edit_file": true, "multi_edit": true, "fileops": true}
		if writeTools[tool.Name()] {
			return &tools.ToolResult{
				Output:  fmt.Sprintf("Permission denied: '%s' is blocked in plan (read-only) mode.", tool.Name()),
				IsError: true,
			}
		}

	case ModeAsk:
		dangerousTools := map[string]bool{
			"exec": true, "write_file": true, "edit_file": true,
			"multi_edit": true, "fileops": true,
		}
		isDangerous := dangerousTools[tool.Name()]

		if warning != "" {
			// Tool returned a warning -- always ask user regardless of tool type
			if !g.askUserWithWarning(tool.Name(), params, warning) {
				return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
			}
			return nil // user approved
		}

		// No warning but still dangerous -- ask normally
		if isDangerous {
			if tool.Name() == "exec" {
				cmd, _ := params["command"].(string)
				if g.isSafeCommand(cmd) {
					return nil // Safe command, allow without asking
				}
			}
			if !g.askUser(tool.Name(), params) {
				return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
			}
		}
	}

	// ModeAuto or passed: allow
	return nil
}

func (g *PermissionGate) isSafeCommand(command string) bool {
	cmd := strings.TrimSpace(strings.ToLower(command))
	for _, safe := range g.config.AllowedCommands {
		if strings.HasPrefix(cmd, safe) {
			remainder := cmd[len(safe):]
			if remainder == "" {
				return true
			}
			// Only allow if remainder is a simple argument (no shell metacharacters)
			if remainder[0] == ' ' && !containsShellMetacharacters(remainder[1:]) {
				return true
			}
		}
	}
	return false
}

// containsShellMetacharacters checks if a string contains characters that
// could be used for command injection (shell operators, redirections, etc.).
func containsShellMetacharacters(s string) bool {
	return strings.ContainsAny(s, "&|;`$(){}[]<>!#~\n\r")
}

func (g *PermissionGate) askUser(toolName string, params map[string]any) bool {
	return g.askUserWithWarning(toolName, params, "")
}

func (g *PermissionGate) askUserWithWarning(toolName string, params map[string]any, warning string) bool {
	var detail string
	switch toolName {
	case "exec":
		detail, _ = params["command"].(string)
	case "write_file", "edit_file":
		detail, _ = params["path"].(string)
	}

	prompt := fmt.Sprintf("\n[Permission] Allow '%s'", toolName)
	if detail != "" {
		prompt += ": " + detail
	}
	if warning != "" {
		prompt += "\n  [WARN] " + warning
	}
	prompt += "? [y/N] "

	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}
