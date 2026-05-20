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
		// Do NOT delete tool_use_id — the API requires it for tool_result pairing.
		// cache_reference is an additional field for cache tracking, not a replacement.
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

	// Array content -> add cache_control to last block
		if arr, ok := content.([]any); ok && len(arr) > 0 {
			last := arr[len(arr)-1]
			if m, ok := last.(map[string]any); ok {
				m["cache_control"] = marker
				// Do NOT delete tool_use_id from tool_result blocks —
				// the API requires it for tool_result/tool_use pairing (error 2013).
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
		params.System[0].CacheControl = anthropic.NewCacheControlEphemeralParam()
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

// CacheChangeCategory represents a specific category of change that can break
// the KV cache. Matches upstream's promptCacheBreakDetection.ts categories.
type CacheChangeCategory string

const (
	CacheChangeToolResult     CacheChangeCategory = "tool_result"
	CacheChangeThinking       CacheChangeCategory = "thinking"
	CacheChangeImage          CacheChangeCategory = "image"
	CacheChangePDF            CacheChangeCategory = "pdf"
	CacheChangeAttachment     CacheChangeCategory = "attachment"
	CacheChangeSystemPrompt   CacheChangeCategory = "system_prompt"
	CacheChangeCompaction     CacheChangeCategory = "compaction"
	CacheChangeEdit           CacheChangeCategory = "edit"
	CacheChangeUserMessage    CacheChangeCategory = "user_message"
	CacheChangeToolUse        CacheChangeCategory = "tool_use"
	CacheChangeNormalization  CacheChangeCategory = "normalization"
	CacheChangeOther          CacheChangeCategory = "other"
)

// cacheChangeWeight returns the expected token impact weight for a change category.
// Matches upstream's per-category weights in promptCacheBreakDetection.ts.
func cacheChangeWeight(cat CacheChangeCategory) int64 {
	switch cat {
	case CacheChangeCompaction:
		return 50000 // compaction restructures the entire context
	case CacheChangeSystemPrompt:
		return 20000 // system prompt changes invalidate the prefix
	case CacheChangeToolResult:
		return 5000 // tool results vary in size
	case CacheChangeThinking:
		return 3000 // thinking blocks are moderate
	case CacheChangeEdit:
		return 3000 // file edits are moderate
	case CacheChangeAttachment:
		return 4000 // attachments can be large
	case CacheChangePDF:
		return 8000 // PDFs are typically large
	case CacheChangeImage:
		return 6000 // images are token-expensive
	case CacheChangeUserMessage:
		return 2000 // user messages are typically short
	case CacheChangeToolUse:
		return 1000 // tool use blocks are small
	case CacheChangeNormalization:
		return 500 // normalization changes are minor
	default:
		return 2000 // default weight
	}
}

// CacheBreakDetector tracks cache read tokens between API calls to detect
// when the KV cache has been broken. Uses category-based tracking matching
// upstream's promptCacheBreakDetection.ts (12+ change categories with weights).
type CacheBreakDetector struct {
	mu                  sync.Mutex
	lastCacheReadTokens int64 // tokens read from cache in previous call
	baselineSet         bool
	pendingChanges      map[CacheChangeCategory]int // changes recorded since last API call
	estimatedImpact     int64                       // estimated token impact of pending changes
}

// RecordChange records a change in a specific category. This should be called
// whenever the message array is modified between API calls (e.g., adding a tool
// result, editing a file, normalizing messages). Matches upstream's tracking
// of specific change categories instead of a simple threshold heuristic.
func (d *CacheBreakDetector) RecordChange(category CacheChangeCategory, count int) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pendingChanges == nil {
		d.pendingChanges = make(map[CacheChangeCategory]int)
	}
	d.pendingChanges[category] += count
	d.estimatedImpact += cacheChangeWeight(category) * int64(count)
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
	// Clear pending changes after baseline update
	d.pendingChanges = nil
	d.estimatedImpact = 0
}

// DetectBreak checks if there was a significant cache break between calls.
// Uses two methods:
//  1. Category-based: if pending changes exceed a weight threshold, predict a break
//  2. Token-based: if cache_read dropped by more than 20% from baseline
//
// Method 1 matches upstream's approach of tracking specific change categories.
// Method 2 is kept as a fallback for cases where changes aren't explicitly recorded.
func (d *CacheBreakDetector) DetectBreak(currentCacheReadTokens int64) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	// Method 1: Category-based prediction
	// If we've recorded changes with significant estimated impact,
	// predict a break even before seeing the API response.
	// The threshold is based on the baseline — if estimated impact > 10% of baseline,
	// a break is likely.
	if d.baselineSet && d.lastCacheReadTokens > 0 && d.estimatedImpact > 0 {
		categoryThreshold := d.lastCacheReadTokens / 10 // 10% of baseline
		if d.estimatedImpact > categoryThreshold {
			return true
		}
	}

	// Method 2: Token-based fallback
	// A break is detected when cache_read dropped by more than 20% from baseline.
	if !d.baselineSet || d.lastCacheReadTokens == 0 {
		return false
	}
	drop := d.lastCacheReadTokens - currentCacheReadTokens
	threshold := int64(float64(d.lastCacheReadTokens) * 0.20)
	return drop > threshold
}

// ResetBaseline clears the baseline, e.g., after compaction invalidates all
// cached prefixes. Also records the compaction change.
func (d *CacheBreakDetector) ResetBaseline() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.baselineSet = false
	d.lastCacheReadTokens = 0
	d.pendingChanges = nil
	d.estimatedImpact = 0
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
// Matches upstream's applyPinnedCacheEdits in cachedMicrocompact.ts:
//  1. Find the message at the pinned position
//  2. Search for tool_result blocks within that message
//  3. Add cache_control: ephemeral to preserve the KV cache prefix
func ApplyPinnedCacheEdits(messages []anthropic.MessageParam, edits []PinnedCacheEdit) []anthropic.MessageParam {
	if len(edits) == 0 || len(messages) == 0 {
		return messages
	}

	// Track which messages need modification to avoid unnecessary serialization
	modified := false

	for _, edit := range edits {
		if edit.Position < 0 || edit.Position >= len(messages) {
			continue
		}

		msg := messages[edit.Position]

		// Only process user messages (tool results are in user role)
		if msg.Role != anthropic.MessageParamRoleUser {
			continue
		}

		// Search for tool_result blocks matching the tool_use_id
		for i := range msg.Content {
			block := &msg.Content[i]
			if block.OfToolResult != nil && block.OfToolResult.ToolUseID == edit.ToolUseID {
				// Add cache_control to preserve this tool_result in KV cache
				block.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
				modified = true
			}
		}
	}

	if !modified {
		return messages
	}

	return messages
}
