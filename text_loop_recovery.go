package main

import (
	"strings"
	"sync"
)

// ─── Text Loop Recovery (MiMo-Code 2) ──────────────────────────────────────
//
// Detects when the LLM outputs identical text across consecutive turns.
// Injects escalating recovery prompts to break the loop.
//
// MiMo-Code source: session/prompt/text-loop-recovery.ts (40 lines)

const (
	TextLoopBufferSize = 5
	TextLoopThreshold  = 3
	TextLoopMaxRecovery = 2
)

// TextLoopDetector detects text-level repetition loops.
type TextLoopDetector struct {
	mu           sync.Mutex
	buffer       []string
	recoveryCount int
}

// NewTextLoopDetector creates a new text loop detector.
func NewTextLoopDetector() *TextLoopDetector {
	return &TextLoopDetector{
		buffer: make([]string, 0, TextLoopBufferSize),
	}
}

// CheckRecord records text output and checks for loops.
// Returns true if a text loop is detected.
func (d *TextLoopDetector) CheckRecord(text string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	normalized := normalizeForLoopDetection(text)
	if normalized == "" {
		return false
	}

	d.buffer = append(d.buffer, normalized)
	if len(d.buffer) > TextLoopBufferSize {
		d.buffer = d.buffer[1:]
	}

	// Count consecutive identical outputs
	if len(d.buffer) < TextLoopThreshold {
		return false
	}

	last := d.buffer[len(d.buffer)-1]
	count := 0
	for i := len(d.buffer) - 1; i >= 0; i-- {
		if d.buffer[i] == last {
			count++
		} else {
			break
		}
	}

	return count >= TextLoopThreshold
}

// GetRecoveryPrompt returns an escalating recovery prompt.
func (d *TextLoopDetector) GetRecoveryPrompt() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.recoveryCount++

	if d.recoveryCount == 1 {
		return RecoveryPromptMild
	}
	if d.recoveryCount <= TextLoopMaxRecovery {
		return RecoveryPromptStrong
	}
	return ""
}

// Reset resets the detector state.
func (d *TextLoopDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buffer = make([]string, 0, TextLoopBufferSize)
	d.recoveryCount = 0
}

// normalizeForLoopDetection normalizes text for comparison.
func normalizeForLoopDetection(text string) string {
	// Lowercase
	text = strings.ToLower(text)
	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")
	// Trim
	text = strings.TrimSpace(text)
	return text
}

// Recovery prompts
const (
	RecoveryPromptMild = "STOP. You are repeating yourself. Try a completely different approach or ask the user for clarification."
	RecoveryPromptStrong = "STOP. You are stuck in a loop. Abandon your current approach entirely. Ask the user what they want you to do differently."
)
