package main

import (
	"sync"
)

// ─── Never-Ask Mode (MiMo-Code 3) ──────────────────────────────────────────
//
// When in headless/CI mode, the question tool skips blocking for human input.
// Instead, it returns a structured prompt for the model to self-resolve.
//
// MiMo-Code source: tool/question.ts (29-45 lines)

// NeverAskConfig holds never-ask mode configuration.
type NeverAskConfig struct {
	mu      sync.Mutex
	enabled bool
}

// NewNeverAskConfig creates a new never-ask config.
func NewNeverAskConfig(enabled bool) *NeverAskConfig {
	return &NeverAskConfig{
		enabled: enabled,
	}
}

// IsEnabled returns true if never-ask mode is enabled.
func (c *NeverAskConfig) IsEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enabled
}

// SetEnabled enables or disables never-ask mode.
func (c *NeverAskConfig) SetEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = enabled
}

// BuildNeverAskPrompt builds a prompt for self-resolution when no user is available.
func BuildNeverAskPrompt(question string, options []string) string {
	var sb string
	sb += "<system-reminder>\n"
	sb += "No user is available to answer this question. You must decide yourself.\n\n"
	sb += "**Question**: " + question + "\n\n"

	if len(options) > 0 {
		sb += "**Options**:\n"
		for i, opt := range options {
			sb += "  " + string(rune('A'+i)) + ". " + opt + "\n"
		}
		sb += "\n"
	}

	sb += "Pick the best option for unattended execution and explicitly state your choice in your response.\n"
	sb += "</system-reminder>"

	return sb
}

// ShouldNeverAsk returns true if the question should be auto-resolved.
func ShouldNeverAsk(config *NeverAskConfig) bool {
	if config == nil {
		return false
	}
	return config.IsEnabled()
}
