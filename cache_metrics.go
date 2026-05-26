package main

import (
	"fmt"
	"strings"
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

// ReadTracker tracks files read via read_file/list_directory.
// Edit operations consult this before proceeding. Cleared on fold/compaction.
//
// Matching DeepSeek-Reasonix's tools/read-tracker.ts
type ReadTracker struct {
	mu          sync.RWMutex
	readFiles   map[string]bool // normalized file path -> true
	readDirs    map[string]bool // normalized directory path -> true
	epoch       int              // incremented on each compaction to invalidate stale reads
}

// NewReadTracker creates a new read tracker.
func NewReadTracker() *ReadTracker {
	return &ReadTracker{
		readFiles: make(map[string]bool),
		readDirs:  make(map[string]bool),
	}
}

// MarkRead marks a file as read.
func (rt *ReadTracker) MarkRead(path string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.readFiles[normalizePath(path)] = true
}

// MarkDirRead marks a directory as read (via list_directory).
func (rt *ReadTracker) MarkDirRead(path string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.readDirs[normalizePath(path)] = true
}

// WasRead returns true if the file was read in the current epoch.
func (rt *ReadTracker) WasRead(path string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.readFiles[normalizePath(path)]
}

// WasDirRead returns true if the directory was listed in the current epoch.
func (rt *ReadTracker) WasDirRead(path string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.readDirs[normalizePath(path)]
}

// Reset clears all tracked reads (called after compaction).
func (rt *ReadTracker) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.readFiles = make(map[string]bool)
	rt.readDirs = make(map[string]bool)
	rt.epoch++
}

// Epoch returns the current epoch number.
func (rt *ReadTracker) Epoch() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.epoch
}

// normalizePath normalizes a file path for comparison.
func normalizePath(path string) string {
	// Convert backslashes to forward slashes for Windows compatibility
	path = strings.ReplaceAll(path, "\\", "/")
	// Remove trailing slashes
	path = strings.TrimRight(path, "/")
	// Lowercase for case-insensitive comparison on Windows
	path = strings.ToLower(path)
	return path
}

// trimTrailingToolCalls drops unpaired assistant messages with tool_calls
// before generating a forced summary. Keeps prefix cache valid.
//
// Matching DeepSeek-Reasonix's context-manager.ts trimTrailingToolCalls
func TrimTrailingToolCalls(messages []MessageParam) bool {
	if len(messages) == 0 {
		return false
	}

	// Get the last message
	lastMsg := messages[len(messages)-1]

	// Check if it's an assistant message with tool_calls but no matching tool result
	if lastMsg.Role != RoleAssistant {
		return false
	}

	// Check for tool_calls in content blocks
	hasToolCalls := false
	for _, block := range lastMsg.Content {
		if block.ToolUse != nil {
			hasToolCalls = true
			break
		}
	}

	if !hasToolCalls {
		return false
	}

	// Check if there's a matching tool result
	hasToolResult := false
	for i := len(messages) - 2; i >= 0; i-- {
		if messages[i].Role == RoleTool {
			hasToolResult = true
			break
		}
		if messages[i].Role == RoleAssistant {
			break // Stop at previous assistant message
		}
	}

	// If there's no tool result, we need to trim this message
	if !hasToolResult {
		// Remove the last message (the unpaired tool_calls)
		// This is a destructive operation - caller should copy if needed
		messages[len(messages)-1] = MessageParam{}
		return true
	}

	return false
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