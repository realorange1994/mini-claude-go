package main

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// =============================================================================
// Section 1: CircularBuffer
// =============================================================================

// CircularBuffer is a fixed-size buffer that evicts the oldest entries when full.
// Ported from upstream TypeScript CircularBuffer.ts.
//
// Usage:
//
//	buf := NewCircularBuffer[int](5)
//	buf.Add(1)
//	buf.Add(2)
//	items := buf.ToArray() // [1, 2]
type CircularBuffer[T any] struct {
	data     []T
	capacity int
}

// NewCircularBuffer creates a new circular buffer with the given capacity.
func NewCircularBuffer[T any](capacity int) *CircularBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &CircularBuffer[T]{
		data:     make([]T, 0, capacity),
		capacity: capacity,
	}
}

// Add appends an item to the buffer, evicting the oldest if at capacity.
func (b *CircularBuffer[T]) Add(item T) {
	if len(b.data) >= b.capacity {
		// Evict the oldest item (shift left)
		copy(b.data, b.data[1:])
		b.data[len(b.data)-1] = item
	} else {
		b.data = append(b.data, item)
	}
}

// AddAll adds multiple items to the buffer.
func (b *CircularBuffer[T]) AddAll(items []T) {
	for _, item := range items {
		b.Add(item)
	}
}

// Length returns the number of items currently in the buffer.
func (b *CircularBuffer[T]) Length() int {
	return len(b.data)
}

// ToArray returns a copy of all items in the buffer in insertion order.
func (b *CircularBuffer[T]) ToArray() []T {
	result := make([]T, len(b.data))
	copy(result, b.data)
	return result
}

// GetRecent returns the last N items from the buffer.
// If N is greater than the number of items, returns all items.
func (b *CircularBuffer[T]) GetRecent(n int) []T {
	if n >= len(b.data) {
		return b.ToArray()
	}
	start := len(b.data) - n
	result := make([]T, n)
	copy(result, b.data[start:])
	return result
}

// Clear removes all items from the buffer.
func (b *CircularBuffer[T]) Clear() {
	b.data = b.data[:0]
}

// =============================================================================
// Section 2: Retry Utils (jitteredBackoff)
// =============================================================================

// jitteredBackoff computes a jittered exponential backoff delay.
//
// Replaces fixed exponential backoff with jittered delays to prevent
// thundering-herd retry spikes when multiple sessions hit the same
// rate-limited provider concurrently.
//
// Parameters:
//   - attempt: 1-based retry attempt number
//   - baseDelay: base delay in seconds for attempt 1 (default: 5)
//   - maxDelay: maximum delay cap in seconds (default: 120)
//   - jitterRatio: fraction of computed delay to use as random jitter range (default: 0.5)
//
// Returns: delay = min(base * 2^(attempt-1), maxDelay) + uniform(0, jitterRatio * delay)
func jitteredBackoff(attempt int, opts ...JitterOpt) time.Duration {
	cfg := jitterConfig{
		baseDelay:   5 * time.Second,
		maxDelay:    120 * time.Second,
		jitterRatio: 0.5,
	}
	for _, o := range opts {
		o(&cfg)
	}

	exponent := attempt - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent >= 63 {
		return cfg.maxDelay
	}

	delay := cfg.baseDelay * (1 << uint(exponent))
	if delay > cfg.maxDelay {
		delay = cfg.maxDelay
	}

	jitter := time.Duration(rand.Float64() * cfg.jitterRatio * float64(delay))
	return delay + jitter
}

type jitterConfig struct {
	baseDelay   time.Duration
	maxDelay    time.Duration
	jitterRatio float64
}

// JitterOpt is a functional option for jitteredBackoff.
type JitterOpt func(*jitterConfig)

// WithJitterBase sets the base delay.
func WithJitterBase(d time.Duration) JitterOpt {
	return func(c *jitterConfig) { c.baseDelay = d }
}

// WithJitterMax sets the maximum delay cap.
func WithJitterMax(d time.Duration) JitterOpt {
	return func(c *jitterConfig) { c.maxDelay = d }
}

// WithJitterRatio sets the jitter ratio (0-1).
func WithJitterRatio(r float64) JitterOpt {
	return func(c *jitterConfig) { c.jitterRatio = r }
}

// =============================================================================
// Section 3: Prompt History
// =============================================================================

// PromptHistory persists user prompts to a JSONL file for session continuity.
// Each entry records the prompt text, timestamp, and session ID.
type PromptHistory struct {
	filePath string
	mu       sync.Mutex
}

// PromptEntry is a single history record.
type PromptEntry struct {
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id"`
}

// NewPromptHistory creates a history manager that writes to .claude/history.jsonl.
func NewPromptHistory(sessionID string) *PromptHistory {
	dir := ".claude"
	os.MkdirAll(dir, 0o755)
	fp := filepath.Join(dir, "history.jsonl")
	return &PromptHistory{filePath: fp}
}

// Record appends a prompt to the history file.
func (h *PromptHistory) Record(text, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := PromptEntry{
		Text:      text,
		Timestamp: time.Now().Format(time.RFC3339),
		SessionID: sessionID,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(h.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(data)
	f.Write([]byte{'\n'})
	f.Close()
}

// LoadRecent returns the most recent N prompts from history.
func (h *PromptHistory) LoadRecent(n int) []PromptEntry {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.Open(h.filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []PromptEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry PromptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if len(entries) > n {
		entries = entries[len(entries)-n:]
	}
	return entries
}

