package main

import (
	"fmt"
	"strings"
)

// ─── Two-Level Pruning (MiMo-Code P7) ──────────────────────────────────────
//
// Two-level context pruning system:
//   Level 1 (soft-trim): Keep head 1536 + tail 1536 of large tool outputs (>4096)
//   Level 2 (hard-clear): Mark old tool outputs as compacted, strip media/reasoning
//
// MiMo-Code source: session/prune.ts (481 lines)

const (
	// PruneMinimum minimum tokens before pruning activates
	PruneMinimum = 20000
	// PruneProtect tokens of recent content to protect from pruning
	PruneProtect = 40000
	// SoftTrimThreshold min chars for soft-trim to activate
	SoftTrimThreshold = 4096
	// SoftTrimKeepHead chars to keep from head
	SoftTrimKeepHead = 1536
	// SoftTrimKeepTail chars to keep from tail
	SoftTrimKeepTail = 1536
	// PruneProtectTurns number of recent turns to protect
	PruneProtectTurns = 3
)

// PruneProtectedTools are tools whose output is never pruned
var PruneProtectedTools = []string{"skill", "search_skills", "read_skill"}

// PruneLevel determines the pruning level based on pressure
func PruneLevel(pressure int) int {
	if pressure >= 2 {
		return 2
	}
	if pressure >= 1 {
		return 1
	}
	return 0
}

// ToolOutput represents a tool output in the conversation
type ToolOutput struct {
	ToolName  string
	Output    string
	TurnIndex int
	Compacted bool
}

// PruneResult holds the result of a pruning operation
type PruneResult struct {
	SoftTrimmed int // number of outputs soft-trimmed
	HardCleared int // number of outputs hard-cleared
	Stripped    int // number of non-essential items stripped
	SavedTokens int // estimated tokens saved
}

// PruneOldToolOutputs performs two-level pruning on old tool outputs.
// Level 1: soft-trim large outputs (keep head + tail)
// Level 2: hard-clear old outputs (mark as compacted)
func PruneOldToolOutputs(outputs []ToolOutput, pressure int) PruneResult {
	level := PruneLevel(pressure)
	if level == 0 {
		return PruneResult{}
	}

	var result PruneResult
	totalTokens := 0

	// Scan from newest to oldest, protect recent turns
	for i := len(outputs) - 1; i >= 0; i-- {
		out := &outputs[i]

		// Skip protected tools
		if isProtectedTool(out.ToolName) {
			continue
		}

		// Skip already compacted
		if out.Compacted {
			break
		}

		// Skip recent turns (protect last N turns)
		if out.TurnIndex >= len(outputs)-PruneProtectTurns {
			continue
		}

		estimate := estimateTokensPrune(out.Output)
		totalTokens += estimate

		// Only prune if we've exceeded the protect threshold
		if totalTokens <= PruneProtect {
			continue
		}

		if level == 1 {
			// Soft-trim: keep head + tail
			if len(out.Output) > SoftTrimThreshold {
				trimmed := softTrimOutput(out.Output)
				result.SoftTrimmed++
				result.SavedTokens += estimate - estimateTokensPrune(trimmed)
				out.Output = trimmed
			}
		} else {
			// Hard-clear: mark as compacted
			out.Compacted = true
			result.HardCleared++
			result.SavedTokens += estimate
		}
	}

	return result
}

// softTrimOutput trims a large output keeping head and tail.
func softTrimOutput(output string) string {
	if len(output) <= SoftTrimThreshold {
		return output
	}

	head := output[:SoftTrimKeepHead]
	tail := output[len(output)-SoftTrimKeepTail:]

	return head + fmt.Sprintf("\n\n[... trimmed — kept first and last 1.5K of %d chars ...]\n\n", len(output)) + tail
}

// StripNonEssential strips media and reasoning from old messages.
// Protects the last N turns.
func StripNonEssential(messages []string, turnRoles []string) (int, int) {
	// Find boundary (protect last 3 turns)
	turnCount := 0
	boundary := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if turnRoles[i] == "user" {
			turnCount++
		}
		if turnCount > PruneProtectTurns {
			boundary = i
			break
		}
	}

	stripped := 0
	reasoningCleared := 0

	for i := 0; i < boundary; i++ {
		msg := messages[i]
		// Strip media references (simplified — check for image patterns)
		if strings.Contains(msg, "![") && strings.Contains(msg, "](") {
			stripped++
		}
		// Strip reasoning blocks (simplified — check for thinking patterns)
		if strings.Contains(msg, "<thinking>") && strings.Contains(msg, "</thinking>") {
			reasoningCleared++
		}
	}

	return stripped, reasoningCleared
}

// isProtectedTool checks if a tool is protected from pruning.
func isProtectedTool(name string) bool {
	for _, p := range PruneProtectedTools {
		if p == name {
			return true
		}
	}
	return false
}

// estimateTokensPrune estimates tokens from character count.
func estimateTokensPrune(text string) int {
	return (len(text) + 3) / 4
}
