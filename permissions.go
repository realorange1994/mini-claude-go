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
	config        *Config
	classifier    *AutoModeClassifier
	transcriptSrc TranscriptSource
	denialCount   int // consecutive denial count for auto mode
}

// TranscriptSource provides compact transcript data for the classifier.
type TranscriptSource interface {
	BuildCompactTranscript(maxMessages int) string
}

func NewPermissionGate(cfg *Config) *PermissionGate {
	return &PermissionGate{config: cfg}
}

// WithClassifier sets the auto mode classifier.
func (g *PermissionGate) WithClassifier(c *AutoModeClassifier) *PermissionGate {
	g.classifier = c
	return g
}

// WithTranscriptSource sets the transcript source for the classifier.
func (g *PermissionGate) WithTranscriptSource(src TranscriptSource) *PermissionGate {
	g.transcriptSrc = src
	return g
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

		if g.shouldAvoidPrompts() {
			if isDangerous {
				return &tools.ToolResult{Output: fmt.Sprintf("Permission denied: '%s' requires user approval (interactive prompts disabled for sub-agent).", tool.Name()), IsError: true}
			}
			return nil // non-dangerous tool, allow
		}

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
	case ModeBypass:
		// Allow all tools directly without classifier evaluation.
		// Tool's own security check (Layer 1) still runs unconditionally.
		return nil
	case ModeAuto:
		return g.checkAutoMode(tool, params)
	}

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

// shouldAvoidPrompts reports whether interactive permission prompts should be
// skipped (e.g. sub-agents that have no terminal user to ask).
func (g *PermissionGate) shouldAvoidPrompts() bool {
	return g.config.ShouldAvoidPermissionPrompts
}

func (g *PermissionGate) askUser(toolName string, params map[string]any) bool {
	return g.askUserWithWarning(toolName, params, "")
}

// checkAutoMode implements the auto mode permission check using the classifier.
// Safe tools are auto-allowed. Other tools are evaluated by the LLM classifier.
// After 3 consecutive denials, falls back to interactive prompt.
func (g *PermissionGate) checkAutoMode(tool tools.Tool, params map[string]any) *tools.ToolResult {
	// Fast path: whitelisted tools are always allowed
	if IsAutoAllowlisted(tool.Name(), params) {
		g.denialCount = 0
		return nil
	}

	// If classifier is not available, fall back to legacy behavior: allow all
	if g.classifier == nil || !g.classifier.IsEnabled() {
		// No classifier configured: auto mode allows all tools (old behavior)
		return nil
	}

	// Build transcript for classifier context
	transcript := ""
	if g.transcriptSrc != nil {
		transcript = g.transcriptSrc.BuildCompactTranscript(20)
	}

	// Call classifier
	result := g.classifier.Classify(tool.Name(), params, transcript)

	if !result.Allow {
		g.denialCount++
		// After 3 consecutive denials, fall back to interactive prompt
		// (unless interactive prompts are disabled for sub-agents)
		if g.denialCount >= 3 && !g.shouldAvoidPrompts() {
			fmt.Fprintf(os.Stderr, "  [auto-classifier] %d consecutive denials, falling back to manual approval\n", g.denialCount)
			if g.askUser(tool.Name(), params) {
				g.denialCount = 0
				return nil
			}
			return &tools.ToolResult{Output: "Permission denied: user rejected.", IsError: true}
		}
		return &tools.ToolResult{
			Output:  fmt.Sprintf("Permission denied: %s", result.Reason),
			IsError: true,
		}
	}

	// Allowed: reset denial count
	g.denialCount = 0
	return nil
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
