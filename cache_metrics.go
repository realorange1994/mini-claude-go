package main

import (
	"fmt"
	"sync"
)

// CacheMetrics tracks prompt cache hit/miss tokens per API call.
// Matching DeepSeek-Reasonix's telemetry/stats.ts Usage tracking.
type CacheMetrics struct {
	mu                   sync.RWMutex
	promptTokens         int     // total prompt tokens
	cacheHitTokens       int     // tokens charged at cache-hit rate
	cacheMissTokens      int     // tokens charged at cache-miss rate
	totalCompletionTokens int   // total completion tokens
	turnCount            int     // number of turns

	// Cumulative stats (persisted across session)
	cumulativeCacheHitTokens   int64
	cumulativeCacheMissTokens  int64
	cumulativeCompletionTokens int64
}

// NewCacheMetrics creates a new cache metrics tracker.
func NewCacheMetrics() *CacheMetrics {
	return &CacheMetrics{}
}

// Record records usage from an API response.
func (m *CacheMetrics) Record(promptTokens, cacheHitTokens, cacheMissTokens, completionTokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.promptTokens = promptTokens
	m.cacheHitTokens = cacheHitTokens
	m.cacheMissTokens = cacheMissTokens
	m.totalCompletionTokens = completionTokens
	m.turnCount++

	// Update cumulative stats
	m.cumulativeCacheHitTokens += int64(cacheHitTokens)
	m.cumulativeCacheMissTokens += int64(cacheMissTokens)
	m.cumulativeCompletionTokens += int64(completionTokens)
}

// CacheHitRatio returns the ratio of cache-hit tokens to total cache-eligible tokens.
// Returns 0 if no cache-eligible tokens.
func (m *CacheMetrics) CacheHitRatio() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := m.cacheHitTokens + m.cacheMissTokens
	if total == 0 {
		return 0
	}
	return float64(m.cacheHitTokens) / float64(total)
}

// CumulativeCacheHitRatio returns cumulative cache hit ratio across all turns.
func (m *CacheMetrics) CumulativeCacheHitRatio() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := m.cumulativeCacheHitTokens + m.cumulativeCacheMissTokens
	if total == 0 {
		return 0
	}
	return float64(m.cumulativeCacheHitTokens) / float64(total)
}

// CacheSavingsUSD estimates USD savings from cache hits.
// Uses DeepSeek pricing: cache hit ~0.0028 USD/M, cache miss ~0.14 USD/M
func (m *CacheMetrics) CacheSavingsUSD() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	const cacheHitRate = 0.0028 / 1_000_000   // USD per cache-hit token
	const cacheMissRate = 0.14 / 1_000_000   // USD per cache-miss token

	hitCost := float64(m.cumulativeCacheHitTokens) * cacheHitRate
	missCost := float64(m.cumulativeCacheMissTokens) * cacheMissRate
	savings := missCost - hitCost

	return savings
}

// Stats returns current turn's cache statistics.
func (m *CacheMetrics) Stats() (promptTokens, cacheHit, cacheMiss, completionTokens int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.promptTokens, m.cacheHitTokens, m.cacheMissTokens, m.totalCompletionTokens
}

// CumulativeStats returns cumulative statistics across all turns.
func (m *CacheMetrics) CumulativeStats() (cacheHit, cacheMiss, completionTokens int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cumulativeCacheHitTokens, m.cumulativeCacheMissTokens, m.cumulativeCompletionTokens
}

// TurnCount returns the number of turns processed.
func (m *CacheMetrics) TurnCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.turnCount
}

// String returns a human-readable cache stats string.
func (m *CacheMetrics) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ratio := 0.0
	total := m.cacheHitTokens + m.cacheMissTokens
	if total > 0 {
		ratio = float64(m.cacheHitTokens) / float64(total)
	}

	return fmt.Sprintf("cache: %.1f%% hit (%d/%d tokens), completion: %d tokens",
		ratio*100, m.cacheHitTokens, total, m.totalCompletionTokens)
}


// MessageParam and Role constants (simplified for this file)
type MessageParam struct {
	Role    string
	Content []ContentBlock
}

type ContentBlock struct {
	Text     string
	ToolUse  *ToolUseBlock
	ToolResult *ToolResultBlock
}

type ToolUseBlock struct {
	ID     string
	Name   string
	Input  any
}

type ToolResultBlock struct {
	ToolUseID string
}

const (
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleUser      = "user"
)