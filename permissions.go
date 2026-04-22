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
	config Config
}

// NewPermissionGate creates a new gate.
func NewPermissionGate(cfg Config) PermissionGate {
	return PermissionGate{config: cfg}
}

// Check runs the permission gauntlet. Returns a ToolResult if denied, nil if allowed.
func (g *PermissionGate) Check(tool tools.Tool, params map[string]any) *tools.ToolResult {
	// Layer 1: tool-level self-check
	denial := tool.CheckPermissions(params)
	if denial != "" {
		return &tools.ToolResult{
			Output:  "Permission denied: " + denial,
			IsError: true,
		}
	}

	// Layer 1.5: denied patterns check
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
		// Ask for permission on dangerous operations
		dangerousTools := map[string]bool{
			"exec": true, "write_file": true, "edit_file": true,
			"multi_edit": true, "fileops": true,
		}
		if dangerousTools[tool.Name()] {
			// For exec, check if it's a safe command first
			if tool.Name() == "exec" {
				cmd, _ := params["command"].(string)
				if g.isSafeCommand(cmd) {
					return nil // Safe command, allow without asking
				}
			}
			// Ask user for permission
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
			return true
		}
	}
	return false
}

func (g *PermissionGate) askUser(toolName string, params map[string]any) bool {
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
	prompt += "? [y/N] "

	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}
