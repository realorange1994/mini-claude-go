package main

import (
	"fmt"
	"strings"
)

// ─── Per-Tool Invocation Style (MiMo-Code 2B) ──────────────────────────────
//
// Allows tools to be invoked in either JSON or shell mode.
// Models that prefer shell-style invocations can use them naturally.
//
// MiMo-Code source: tool/invocation-style.ts (17 lines)

// InvocationStyle represents how a tool is invoked.
type InvocationStyle string

const (
	InvocationJSON  InvocationStyle = "json"  // default JSON parameters
	InvocationShell InvocationStyle = "shell" // shell command line
)

// InvocationConfig holds invocation style configuration.
type InvocationConfig struct {
	DefaultStyle InvocationStyle       // default for all tools
	ToolStyles   map[string]InvocationStyle // per-tool overrides
}

// NewInvocationConfig creates a new invocation config.
func NewInvocationConfig() *InvocationConfig {
	return &InvocationConfig{
		DefaultStyle: InvocationJSON,
		ToolStyles:   make(map[string]InvocationStyle),
	}
}

// GetStyle returns the invocation style for a tool.
func (c *InvocationConfig) GetStyle(toolName string) InvocationStyle {
	if style, ok := c.ToolStyles[toolName]; ok {
		return style
	}
	return c.DefaultStyle
}

// SetToolStyle sets the invocation style for a specific tool.
func (c *InvocationConfig) SetToolStyle(toolName string, style InvocationStyle) {
	c.ToolStyles[toolName] = style
}

// ResolveInvocationStyle resolves the invocation style for a tool.
// Per-tool overrides take precedence over the global default.
func ResolveInvocationStyle(config *InvocationConfig, toolName string) InvocationStyle {
	if config == nil {
		return InvocationJSON
	}
	return config.GetStyle(toolName)
}

// ShellWrap converts JSON parameters to a shell command line.
func ShellWrap(toolName string, params map[string]any) string {
	var parts []string
	parts = append(parts, toolName)

	for key, value := range params {
		switch v := value.(type) {
		case string:
			if strings.Contains(v, " ") {
				parts = append(parts, fmt.Sprintf("--%s=%q", key, v))
			} else {
				parts = append(parts, fmt.Sprintf("--%s=%s", key, v))
			}
		case bool:
			if v {
				parts = append(parts, fmt.Sprintf("--%s", key))
			}
		case int, int64, float64:
			parts = append(parts, fmt.Sprintf("--%s=%v", key, v))
		default:
			parts = append(parts, fmt.Sprintf("--%s=%v", key, v))
		}
	}

	return strings.Join(parts, " ")
}

// ShouldUseShell returns true if the tool should use shell invocation.
func ShouldUseShell(config *InvocationConfig, toolName string, hasShellField bool) bool {
	if config == nil {
		return false
	}
	style := config.GetStyle(toolName)
	return style == InvocationShell && hasShellField
}
