package main

import "fmt"

// ─── Compaction Tail-Turn Budget (MiMo-Code 7) ─────────────────────────────
//
// Selects which recent conversation turns to preserve during compaction.
// Uses budget-aware backward iteration to maximize recent context survival.
//
// MiMo-Code source: session/compaction.ts (132-171 lines)

const (
	DefaultTailTurns        = 2
	DefaultPreserveTokens   = 2000
	MaxPreserveTokens       = 8000
	PreserveTokensFraction  = 0.25 // 25% of usable context
)

// TailTurnSelection represents the result of tail-turn selection.
type TailTurnSelection struct {
	HeadMessages  []CompactionMessage
	TailMessages  []CompactionMessage
	TailStartIdx  int
	TailTokens    int
	TotalTokens   int
}

// SelectTailTurns selects which recent turns to preserve during compaction.
func SelectTailTurns(messages []CompactionMessage, usableContext int, tailTurns int) TailTurnSelection {
	if tailTurns <= 0 {
		tailTurns = DefaultTailTurns
	}

	// Calculate preserve budget
	preserveBudget := int(float64(usableContext) * PreserveTokensFraction)
	if preserveBudget < DefaultPreserveTokens {
		preserveBudget = DefaultPreserveTokens
	}
	if preserveBudget > MaxPreserveTokens {
		preserveBudget = MaxPreserveTokens
	}

	// Find user-turn boundaries
	boundaries := findTurnBoundaries(messages)
	if len(boundaries) == 0 {
		return TailTurnSelection{
			HeadMessages: messages,
			TailMessages: nil,
			TailStartIdx: len(messages),
		}
	}

	// Take last N turns
	startBoundary := len(boundaries) - tailTurns
	if startBoundary < 0 {
		startBoundary = 0
	}

	// Walk backward accumulating tokens
	tailTokens := 0
	tailStartIdx := boundaries[startBoundary]

	for i := startBoundary; i < len(boundaries); i++ {
		turnStart := boundaries[i]
		turnEnd := len(messages)
		if i+1 < len(boundaries) {
			turnEnd = boundaries[i+1]
		}

		turnTokens := estimateTurnTokens(messages[turnStart:turnEnd])
		if tailTokens+turnTokens > preserveBudget {
			break
		}
		tailTokens += turnTokens
		tailStartIdx = turnStart
	}

	return TailTurnSelection{
		HeadMessages: messages[:tailStartIdx],
		TailMessages: messages[tailStartIdx:],
		TailStartIdx: tailStartIdx,
		TailTokens:   tailTokens,
		TotalTokens:  estimateTotalTokens(messages),
	}
}

// findTurnBoundaries finds indices where user turns start.
func findTurnBoundaries(messages []CompactionMessage) []int {
	var boundaries []int
	for i, msg := range messages {
		if msg.Role == "user" {
			boundaries = append(boundaries, i)
		}
	}
	return boundaries
}

// estimateTurnTokens estimates tokens for a turn.
func estimateTurnTokens(messages []CompactionMessage) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokensBudget(msg.Content)
	}
	return total
}

// estimateTotalTokens estimates total tokens for all messages.
func estimateTotalTokens(messages []CompactionMessage) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokensBudget(msg.Content)
	}
	return total
}

// estimateTokensBudget estimates tokens from character count.
func estimateTokensBudget(text string) int {
	return (len(text) + 3) / 4
}

// FormatTailTurnSelection formats a tail turn selection for display.
func FormatTailTurnSelection(selection TailTurnSelection) string {
	var sb string
	sb += "## Tail Turn Selection\n\n"
	sb += fmt.Sprintf("- Head messages: %d\n", len(selection.HeadMessages))
	sb += fmt.Sprintf("- Tail messages: %d\n", len(selection.TailMessages))
	sb += fmt.Sprintf("- Tail tokens: %d\n", selection.TailTokens)
	sb += fmt.Sprintf("- Total tokens: %d\n", selection.TotalTokens)
	return sb
}
