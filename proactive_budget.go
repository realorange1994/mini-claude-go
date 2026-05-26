package main

import (
	"fmt"
	"strings"
)

// ProactiveBudgetManager tracks context window usage and injects
// behavioral hints when approaching limits. This matches openclacky's
// context_window.rb proactive budget tracking pattern.
//
// Instead of waiting for a 400 "context too long" error, the manager
// monitors usage and injects hints that encourage the LLM to be more
// concise, reducing the chance of hitting hard limits.
type ProactiveBudgetManager struct {
	contextWindow int // total context window size for current model
}

// NewProactiveBudgetManager creates a budget manager for the given context window size.
func NewProactiveBudgetManager(contextWindow int) *ProactiveBudgetManager {
	return &ProactiveBudgetManager{contextWindow: contextWindow}
}

// BudgetHint returns a behavioral hint string based on current usage.
// Returns empty string if no hint needed.
//
// Thresholds (matching openclacky's context_window.rb):
//   > 50% usage: "Consider being concise in your tool call arguments"
//   > 75% usage: "Be concise — context window is getting full. Prefer shorter tool calls and smaller file edits"
//   > 90% usage: "URGENT: Context window nearly exhausted. Use minimal tool calls. Do NOT read entire files."
func (b *ProactiveBudgetManager) BudgetHint(currentTokens int) string {
	if b.contextWindow <= 0 {
		return ""
	}
	usagePercent := float64(currentTokens) / float64(b.contextWindow) * 100

	switch {
	case usagePercent > 90:
		return "URGENT: The context window is nearly full (>90%%). Use minimal, targeted tool calls. Do NOT read entire files. Prefer grep/search to find specific lines. Keep tool call arguments short."
	case usagePercent > 75:
		return "Note: The context window is getting full (>75%%). Be concise in your tool call arguments and prefer smaller, targeted edits over large writes."
	case usagePercent > 50:
		return "Consider being concise in your tool call arguments to keep the context window manageable."
	default:
		return ""
	}
}

// ShouldProactiveCompact returns true if proactive compaction should be triggered.
// This fires at 75% usage — compacting early preserves more useful context than
// waiting for a hard 400 error at 100%.
func (b *ProactiveBudgetManager) ShouldProactiveCompact(currentTokens int) bool {
	if b.contextWindow <= 0 {
		return false
	}
	usagePercent := float64(currentTokens) / float64(b.contextWindow)
	return usagePercent > 0.75
}

// ToolErrorSelfCorrector generates a self-correction hint based on the
// tool error. This matches openclacky's error recovery pattern: instead
// of just returning the error, inject a hint that guides the LLM toward
// a better approach.
//
// Patterns:
//   - File not found → suggest searching for the file
//   - Grep no results → suggest broadening the pattern
//   - Edit failed (old_string not found) → suggest reading the file first
//   - Permission denied → suggest alternative approach
func ToolErrorSelfCorrectionHint(toolName string, errMsg string, input map[string]any) string {
	msg := strings.ToLower(errMsg)

	// File not found errors
	if strings.Contains(msg, "no such file") || strings.Contains(msg, "file not found") ||
		strings.Contains(msg, "does not exist") {
		path, _ := input["file_path"].(string)
		if path != "" {
			return fmt.Sprintf(
				"The file '%s' does not exist. Try using search_files or glob to find the correct path, or check if you need to create it first with write_file.",
				path)
		}
		return "The file does not exist. Try using search_files or glob to find the correct path."
	}

	// Grep/search no results
	if (toolName == "grep" || toolName == "search_files") &&
		(strings.Contains(msg, "no matches") || strings.Contains(msg, "0 results") || strings.Contains(msg, "nothing found")) {
		pattern, _ := input["pattern"].(string)
		if pattern != "" {
			return fmt.Sprintf(
				"No results for pattern '%s'. Try broadening the search: use a simpler pattern, remove special characters, or search in a different directory.",
				pattern)
		}
		return "No search results. Try broadening the pattern or searching in a different directory."
	}

	// Edit failed (old_string not found)
	if toolName == "edit_file" && (strings.Contains(msg, "not found") || strings.Contains(msg, "does not match")) {
		path, _ := input["file_path"].(string)
		if path != "" {
			return fmt.Sprintf(
				"The old_string was not found in '%s'. Read the file first to see its current content, then use the exact text that exists in the file.",
				path)
		}
		return "The old_string was not found. Read the file first to see its current content."
	}

	// Permission denied
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "access denied") {
		return "Permission denied. Try using a different approach or check if you need elevated permissions."
	}

	return ""
}