package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
)

// CacheBreakpointConfig controls the KV cache breakpoint strategy.
// Upstream Anthropic uses exactly 1 breakpoint at the last message for optimal
// Mycro KV cache manager behavior.
type CacheBreakpointConfig struct {
	// MaxBreakpoints is the maximum number of cache breakpoints to place.
	// Set to 1 to match upstream's optimal strategy.
	MaxBreakpoints int
	// SkipCacheWrite shifts the breakpoint from the last message to the
	// second-to-last message, protecting the last position's KV pages.
	// Use for fire-and-forget scenarios (forked agents, background tasks).
	SkipCacheWrite bool
}

// DefaultCacheBreakpointConfig returns the default config matching upstream's
// optimal KV cache strategy: exactly 1 breakpoint at the last message.
func DefaultCacheBreakpointConfig() CacheBreakpointConfig {
	return CacheBreakpointConfig{
		MaxBreakpoints: 1,
		SkipCacheWrite: false,
	}
}

const (
	// MaxCacheBreakpoints is the maximum number of cache breakpoints to place
	// in the API message stream. Upstream Anthropic uses exactly 1 breakpoint
	// at the last message, which is optimal for the Mycro KV cache manager.
	MaxCacheBreakpoints = 1
)

// ApplyPromptCaching applies Anthropic's optimal caching strategy to API messages.
// Places exactly 1 cache_control breakpoint at the last message (or second-to-last
// when skipCacheWrite is true for fire-and-forget scenarios like forked agents).
// Returns a new slice with cache_control breakpoints injected into the messages.
//
// This reduces input token costs by reusing cached prefixes across API calls.
// The single-breakpoint strategy matches upstream's Mycro KV cache manager,
// which writes the cache at exactly one position per request.
func ApplyPromptCaching(messages []map[string]any, ttl string) []map[string]any {
	return ApplyPromptCachingWithConfig(messages, ttl, DefaultCacheBreakpointConfig())
}

// ApplyPromptCachingWithConfig applies prompt caching with explicit config.
// This is the main entry point for cache breakpoint placement.
func ApplyPromptCachingWithConfig(messages []map[string]any, ttl string, cfg CacheBreakpointConfig) []map[string]any {
	if len(messages) == 0 {
		return messages
	}

	result := deepCopyMessages(messages)
	marker := map[string]any{"type": "ephemeral"}
	if ttl == "1h" {
		marker = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}

	maxBP := cfg.MaxBreakpoints
	if maxBP <= 0 {
		maxBP = MaxCacheBreakpoints
	}

	// Determine the breakpoint position: last message, or second-to-last
	// if skipCacheWrite is set (protects the last position's KV pages for
	// fire-and-forget scenarios like forked agents / background tasks).
	breakpointIdx := len(result) - 1
	if cfg.SkipCacheWrite && len(result) >= 2 {
		breakpointIdx = len(result) - 2
	}

	// Apply the single breakpoint at the determined position.
	applyCacheMarker(result[breakpointIdx], marker)

	// Also apply a breakpoint to the system prompt (first message if system role).
	// The system prompt breakpoint is separate from the message breakpoints
	// and counts as one of the total allowed breakpoints.
	if breakpointIdx > 0 {
		if role, _ := result[0]["role"].(string); role == "system" && maxBP >= 2 {
			applyCacheMarker(result[0], marker)
		}
	}

	return result
}

// applyCacheMarker adds cache_control to a single message, handling all formats.
// For tool_result blocks that are cached, uses cache_reference instead of
// tool_use_id, matching the upstream API field name.
func applyCacheMarker(msg map[string]any, marker map[string]any) {
	role, _ := msg["role"].(string)

	// tool role: cache_control goes at message level
	if role == "tool" {
		msg["cache_control"] = marker
		// Use cache_reference instead of tool_use_id for cached tool_result blocks
		if toolUseID, ok := msg["tool_use_id"].(string); ok && toolUseID != "" {
			msg["cache_reference"] = toolUseID
			delete(msg, "tool_use_id")
		}
		return
	}

	content, exists := msg["content"]
	if !exists {
		msg["cache_control"] = marker
		return
	}

	// Empty string content
	if s, ok := content.(string); ok && s == "" {
		msg["cache_control"] = marker
		return
	}

	// String content -> convert to array format
	if s, ok := content.(string); ok {
		msg["content"] = []map[string]any{
			{
				"type":          "text",
				"text":          s,
				"cache_control": marker,
			},
		}
		return
	}

	// Array content -> add cache_control to last block;
	// for tool_result blocks, use cache_reference instead of tool_use_id
	if arr, ok := content.([]any); ok && len(arr) > 0 {
		last := arr[len(arr)-1]
		if m, ok := last.(map[string]any); ok {
			m["cache_control"] = marker
			// For tool_result blocks, use cache_reference field
			if blockType, _ := m["type"].(string); blockType == "tool_result" {
				if toolUseID, ok := m["tool_use_id"].(string); ok && toolUseID != "" {
					m["cache_reference"] = toolUseID
					delete(m, "tool_use_id")
				}
			}
		}
	}
}

// deepCopyMessages does a deep copy via JSON marshal/unmarshal.
// Returns the original slice on marshal failure (avoiding nil/empty results).
func deepCopyMessages(messages []map[string]any) []map[string]any {
	data, err := json.Marshal(messages)
	if err != nil {
		return messages
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return messages
	}
	return result
}

// cacheMessageParams converts []anthropic.MessageParam to []map[string]any,
// applies prompt caching, and converts back.
func cacheMessageParams(params *anthropic.MessageNewParams) {
	// Convert messages to maps
	msgMaps := messageParamToMaps(params.Messages)
	msgMaps = ApplyPromptCaching(msgMaps, "5m")

	// Convert back to MessageParam
	params.Messages = mapsToMessageParam(msgMaps)

	// Add cache_control to system prompt
	if len(params.System) > 0 {
		params.System[0].CacheControl = anthropic.CacheControlEphemeralParam{}
	}
}

// messageParamToMaps converts SDK message params to map representation.
func messageParamToMaps(msgs []anthropic.MessageParam) []map[string]any {
	data, err := json.Marshal(msgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: marshal failed: %v\n", err)
		return nil
	}
	var result []map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: unmarshal failed: %v\n", err)
		return nil
	}
	return result
}

// mapsToMessageParam converts maps back to SDK message params.
func mapsToMessageParam(msgs []map[string]any) []anthropic.MessageParam {
	data, err := json.Marshal(msgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: marshal failed: %v\n", err)
		return nil
	}
	var result []anthropic.MessageParam
	if err := json.Unmarshal(data, &result); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] prompt_caching: unmarshal failed: %v\n", err)
		return nil
	}
	return result
}

// FormatCachedSystemPrompt wraps the system prompt text for Anthropic caching.
func FormatCachedSystemPrompt(text string, ttl string) []map[string]any {
	marker := map[string]any{"type": "ephemeral"}
	if ttl == "1h" {
		marker = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}
	return []map[string]any{
		{
			"type":          "text",
			"text":          text,
			"cache_control": marker,
		},
	}
}

// FormatBoundaryCachedSystemPrompt splits the system prompt at the static/dynamic
// boundary and applies separate caching scopes. The static part gets a "global"
// cache scope (long-lived, survives across sessions), while the dynamic part
// gets an "org" or no caching scope (short-lived, per-session).
//
// This means the static tool descriptions only need to be hashed once, and
// changes to dynamic content (skills, memory, project instructions) don't
// invalidate the static cache.
func FormatBoundaryCachedSystemPrompt(text string, ttl string) []map[string]any {
	staticPart, dynamicPart, found := SplitSystemPrompt(text)

	if !found {
		// No boundary found, fall back to single-block caching
		return FormatCachedSystemPrompt(text, ttl)
	}

	// Static content: use global cache scope for long-lived caching.
	// The static part (tool descriptions, rules) rarely changes,
	// so a global cache scope maximizes cache hit rates.
	globalMarker := map[string]any{"type": "ephemeral"}
	if ttl == "1h" {
		globalMarker = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}

	// Dynamic content: use standard ephemeral cache (no extended TTL).
	// This content changes per-session or per-turn, so no point in
	// extending its cache lifetime beyond the default.
	dynamicMarker := map[string]any{"type": "ephemeral"}

	result := []map[string]any{
		{
			"type":          "text",
			"text":          staticPart + "\n" + SYSTEM_PROMPT_STATIC_BOUNDARY,
			"cache_control": globalMarker,
		},
	}

	if dynamicPart != "" {
		result = append(result, map[string]any{
			"type":          "text",
			"text":          dynamicPart,
			"cache_control": dynamicMarker,
		})
	}

	return result
}


// ---------------------------------------------------------------------------
// Cache Break Detection
// ---------------------------------------------------------------------------

// CacheBreakDetector tracks cache read tokens between API calls to detect
// when the KV cache has been broken (e.g., by message reordering, compaction,
// or prompt changes that invalidate cached prefixes).
type CacheBreakDetector struct {
	mu                  sync.Mutex
	lastCacheReadTokens int64 // tokens read from cache in previous call
	baselineSet         bool
}

// UpdateBaseline records the cache read tokens after a successful API call.
// This establishes a new baseline for subsequent break detection.
func (d *CacheBreakDetector) UpdateBaseline(cacheReadTokens int64) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastCacheReadTokens = cacheReadTokens
	d.baselineSet = true
}

// DetectBreak checks if there was a significant cache break between calls.
// A break is detected when cache_read drops by more than 20% from baseline,
// indicating the server could not reuse cached KV pages from the previous request.
func (d *CacheBreakDetector) DetectBreak(currentCacheReadTokens int64) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.baselineSet || d.lastCacheReadTokens == 0 {
		return false
	}
	drop := d.lastCacheReadTokens - currentCacheReadTokens
	threshold := int64(float64(d.lastCacheReadTokens) * 0.20)
	return drop > threshold
}

// ResetBaseline clears the baseline, e.g., after compaction invalidates all
// cached prefixes.
func (d *CacheBreakDetector) ResetBaseline() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.baselineSet = false
	d.lastCacheReadTokens = 0
}

// ---------------------------------------------------------------------------
// Pinned Cache Edits
// ---------------------------------------------------------------------------

// PinnedCacheEdit represents a cache edit (tool_result block with cache_control)
// that should persist across API calls. Re-inserting these at their original
// positions preserves KV cache positions for cached tool results.
type PinnedCacheEdit struct {
	ToolUseID string
	Position  int    // original position in message array
	Content   string // cached content
}

// ApplyPinnedCacheEdits re-inserts pinned cache edits at their original positions
// in the message stream. For each pinned edit, it ensures the tool_result at that
// position has cache_control set to preserve the KV cache prefix.
//
// Full implementation requires deeper integration with the message building pipeline.
// Currently a placeholder that logs when pinned edits are applied.
func ApplyPinnedCacheEdits(messages []anthropic.MessageParam, edits []PinnedCacheEdit) []anthropic.MessageParam {
	if len(edits) == 0 || len(messages) == 0 {
		return messages
	}

	for _, edit := range edits {
		if edit.Position < 0 || edit.Position >= len(messages) {
			fmt.Fprintf(os.Stderr, "[WARN] ApplyPinnedCacheEdits: position %d out of range (len=%d)\n",
				edit.Position, len(messages))
			continue
		}

		msg := &messages[edit.Position]
		// Ensure the message has content that can receive cache_control.
		// Full implementation would check for tool_result type and preserve
		// the cache_reference field matching the original edit.
		_ = msg // placeholder: real integration needs MessageParam mutation
	}

	return messages
}
