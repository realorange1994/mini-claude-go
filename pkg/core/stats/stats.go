// Package stats provides session statistics tracking.
// Aligned to pi's stats.ts.
package stats

import (
	"sync"
	"sync/atomic"
	"time"
)

// TokenUsage records token usage for a single API call.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
}

// SessionStats tracks statistics for an agent session.
type SessionStats struct {
	mu sync.Mutex

	// Token usage
	totalInputTokens  int64
	totalOutputTokens int64
	totalCacheRead    int64
	totalCacheWrite   int64

	// Turn tracking
	totalTurns       int64
	totalToolCalls   int64

	// Duration tracking
	sessionStart time.Time
	lastTurnTime time.Time

	// Model tracking
	model string

	// Cost tracking (in USD)
	totalCost float64
}

// NewSessionStats creates a new session stats tracker.
func NewSessionStats(model string) *SessionStats {
	return &SessionStats{
		sessionStart: time.Now(),
		lastTurnTime: time.Now(),
		model:        model,
	}
}

// RecordTokenUsage adds token usage from an API call.
func (s *SessionStats) RecordTokenUsage(usage TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.AddInt64(&s.totalInputTokens, int64(usage.InputTokens))
	atomic.AddInt64(&s.totalOutputTokens, int64(usage.OutputTokens))
	atomic.AddInt64(&s.totalCacheRead, int64(usage.CacheRead))
	atomic.AddInt64(&s.totalCacheWrite, int64(usage.CacheWrite))
}

// RecordTurn increments the turn counter.
func (s *SessionStats) RecordTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalTurns++
	s.lastTurnTime = time.Now()
}

// RecordToolCall increments the tool call counter.
func (s *SessionStats) RecordToolCall() {
	atomic.AddInt64(&s.totalToolCalls, 1)
}

// SetModel updates the current model.
func (s *SessionStats) SetModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = model
}

// GetStats returns a snapshot of the current session statistics.
func (s *SessionStats) GetStats() StatsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return StatsSnapshot{
		TotalInputTokens:  atomic.LoadInt64(&s.totalInputTokens),
		TotalOutputTokens: atomic.LoadInt64(&s.totalOutputTokens),
		TotalCacheRead:    atomic.LoadInt64(&s.totalCacheRead),
		TotalCacheWrite:   atomic.LoadInt64(&s.totalCacheWrite),
		TotalTurns:        atomic.LoadInt64(&s.totalTurns),
		TotalToolCalls:    atomic.LoadInt64(&s.totalToolCalls),
		SessionDuration:   time.Since(s.sessionStart),
		LastTurnTime:      s.lastTurnTime,
		Model:             s.model,
	}
}

// StatsSnapshot is a read-only snapshot of session stats.
type StatsSnapshot struct {
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCacheRead    int64
	TotalCacheWrite   int64
	TotalTurns        int64
	TotalToolCalls    int64
	SessionDuration   time.Duration
	LastTurnTime      time.Time
	Model             string
}
