package main

import (
	"fmt"
	"sync"
	"time"
)

// IdleCompressionTimer automatically compresses the conversation when the user
// has been idle (not typing) for the configured delay. This matches openclacky's
// idle_compression_timer.rb pattern.
//
// Integration:
//   - Call Start() after agent.Run() completes (before waiting for user input)
//   - Call Cancel() before reading user input (ReadString/ReadLine)
//
// The timer only triggers if the conversation is large enough to warrant
// compression (>= minTokens or >= minMessages), avoiding wasteful compaction
// on small conversations.
type IdleCompressionTimer struct {
	mu            sync.Mutex
	timer         *time.Timer
	compressing   bool
	delay         time.Duration
	minTokens     int
	minMessages   int
	consumedTurns int // turns already consumed before timer started (for rollback check)
}

// NewIdleCompressionTimer creates a new idle compression timer with the given
// delay (e.g., 3*time.Minute). Default thresholds: 20K tokens, 30 messages.
func NewIdleCompressionTimer(delay time.Duration) *IdleCompressionTimer {
	return &IdleCompressionTimer{
		delay:       delay,
		minTokens:   20_000,
		minMessages: 30,
	}
}

// SetThresholds configures the minimum conversation size for compression.
// If the conversation has fewer tokens AND fewer messages than these,
// the timer will not trigger compression.
func (t *IdleCompressionTimer) SetThresholds(minTokens, minMessages int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.minTokens = minTokens
	t.minMessages = minMessages
}

// Start begins monitoring for idle compression. Must be called after
// agent.Run() completes and before waiting for user input.
//
// If the conversation is too small, the timer is not started.
func (t *IdleCompressionTimer) Start(agent *AgentLoop) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Stop any existing timer
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}

	// Check if conversation is large enough
	if !t.shouldCompact(agent) {
		return
	}

	// Capture consumed turns for rollback check
	t.consumedTurns = agent.budget.Consumed()

	t.timer = time.AfterFunc(t.delay, func() {
		t.onIdle(agent)
	})
}

// Cancel stops the idle timer. Must be called before reading user input.
// If compression is in progress, it attempts to cancel it.
func (t *IdleCompressionTimer) Cancel() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}

	// If compression is happening, try to interrupt it
	if t.compressing {
		t.compressing = false
	}
}

// IsCompressing returns true if an idle-triggered compression is in progress.
func (t *IdleCompressionTimer) IsCompressing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.compressing
}

// shouldCompact checks whether the current conversation is large enough
// to warrant compression. Must be called with mu held.
func (t *IdleCompressionTimer) shouldCompact(agent *AgentLoop) bool {
	if agent == nil || agent.context == nil {
		return false
	}
	tokens := agent.context.EstimatedTokens()
	msgs := agent.context.Len()
	return tokens >= t.minTokens || msgs >= t.minMessages
}

// onIdle is called by the timer goroutine when the user has been idle long enough.
func (t *IdleCompressionTimer) onIdle(agent *AgentLoop) {
	t.mu.Lock()
	if !t.shouldCompact(agent) {
		t.mu.Unlock()
		return
	}
	if t.compressing {
		t.mu.Unlock()
		return
	}
	t.compressing = true
	t.mu.Unlock()

	preTokens := agent.context.EstimatedTokens()
	preEntries := agent.context.Len()

	fmt.Printf("[idle] User idle for %s, compressing conversation...\n", t.delay)

	agent.ForceCompact()

	postTokens := agent.context.EstimatedTokens()
	postEntries := agent.context.Len()
	saved := preTokens - postTokens

	fmt.Printf("[idle] Compressed: %d → %d entries, %d → %d tokens (saved %d)\n",
		preEntries, postEntries, preTokens, postTokens, saved)

	t.mu.Lock()
	t.compressing = false
	t.mu.Unlock()
}
