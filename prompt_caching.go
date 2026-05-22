package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// CacheBreakpointConfig controls the KV cache breakpoint strategy.
// Uses 2 checkpoints (matching OpenClacky's rolling cache design) so that
// Turn N's last message becomes Turn N+1's second-to-last → cache READ hit.
type CacheBreakpointConfig struct {
	// MaxBreakpoints is the maximum number of cache breakpoints to place.
	// Set to 2 for the rolling cache strategy.
	MaxBreakpoints int
	// SkipCacheWrite shifts the breakpoint from the last message to the
	// second-to-last message, protecting the last position's KV pages.
	// Use for fire-and-forget scenarios (forked agents, background tasks).
	SkipCacheWrite bool
}

// DefaultCacheBreakpointConfig returns the default config with 1 breakpoint
// for the rolling cache strategy: Turn N's last message (still marked) becomes
// Turn N+1's last message → cache READ hit on the prefix.
func DefaultCacheBreakpointConfig() CacheBreakpointConfig {
	return CacheBreakpointConfig{
		MaxBreakpoints: 1,
		SkipCacheWrite: false,
	}
}

const (
	// MaxCacheBreakpoints is the maximum number of cache breakpoints to place
	// in the API message stream. 2 breakpoints enable the rolling cache strategy.
	MaxCacheBreakpoints = 2
)

// ApplyPromptCaching applies Anthropic's optimal caching strategy to API messages.
// Places 2 cache_control breakpoints using a rolling strategy (matching OpenClacky):
//   - Turn N: marks messages[-2] and messages[-1]; server caches prefix up to [-1]
//   - Turn N+1: messages[-2] is Turn N's last message (still marked) → cache READ hit
//
// Auto-injected content (marked with SystemInjectedPrefix) is skipped for breakpoint
// placement, preventing variable attachment/summary content from becoming cache
// breakpoints that change every turn.
func ApplyPromptCaching(messages []map[string]any, ttl string) []map[string]any {
	return ApplyPromptCachingWithConfig(messages, ttl, DefaultCacheBreakpointConfig())
}

// isSystemInjected checks if a message's content starts with the SystemInjectedPrefix
// marker, indicating it was auto-injected (session memory, file recovery, etc.)
// and should be skipped for cache breakpoint placement.
func isSystemInjected(msg map[string]any) bool {
	content, exists := msg["content"]
	if !exists {
		return false
	}
	// String content
	if s, ok := content.(string); ok {
		return strings.HasPrefix(s, SystemInjectedPrefix)
	}
	// Array content — check first text block
	if arr, ok := content.([]any); ok && len(arr) > 0 {
		if m, ok := arr[0].(map[string]any); ok {
			if text, ok := m["text"].(string); ok {
				return strings.HasPrefix(text, SystemInjectedPrefix)
			}
		}
	}
	return false
}

// stripSystemInjected removes the SystemInjectedPrefix from a message's content.
// The prefix is only used internally for breakpoint placement decisions and should
// not be sent to the API.
func stripSystemInjected(msg map[string]any) {
	content, exists := msg["content"]
	if !exists {
		return
	}
	// String content
	if s, ok := content.(string); ok {
		msg["content"] = strings.TrimPrefix(s, SystemInjectedPrefix)
		return
	}
	// Array content — strip from first text block
	if arr, ok := content.([]any); ok && len(arr) > 0 {
		if m, ok := arr[0].(map[string]any); ok {
			if text, ok := m["text"].(string); ok {
				m["text"] = strings.TrimPrefix(text, SystemInjectedPrefix)
			}
		}
	}
}

// hoistToolResultCache detects tool_result blocks where cache_control was
// placed on the inner text block and hoists it to the tool_result level.
// When a tool_result has content: [{text: "foo", cache_control: ...}],
// the shape is [{text, cache_control}] instead of "foo". This shape flip
// destroys cache_read hit rate because the cached prefix changes every turn.
// After hoisting, the block becomes: {type: "tool_result", content: "foo", cache_control: ...}.
// Inspired by openclacky's cache_control hoisting in message_format/anthropic.rb.
func hoistToolResultCache(msg map[string]any) {
	content, exists := msg["content"]
	if !exists {
		return
	}
	arr, ok := content.([]any)
	if !ok || len(arr) == 0 {
		return
	}

	for i, elem := range arr {
		block, ok := elem.(map[string]any)
		if !ok {
			continue
		}
		// Only handle tool_result blocks
		if block["type"] != "tool_result" {
			continue
		}

		// Check if content is a single-element array with cache_control
		inner, hasInner := block["content"]
		if !hasInner {
			continue
		}
		innerArr, ok := inner.([]any)
		if !ok || len(innerArr) != 1 {
			continue
		}
		innerBlock, ok := innerArr[0].(map[string]any)
		if !ok {
			continue
		}
		cacheCtrl, hasCache := innerBlock["cache_control"]
		if !hasCache {
			continue
		}

		// Hoist: extract cache_control to tool_result level
		// Flatten content to just the text string
		if text, ok := innerBlock["text"].(string); ok {
			block["content"] = text
			block["cache_control"] = cacheCtrl
			delete(innerBlock, "cache_control")
			arr[i] = block
		}
	}
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

	// Strip system-injected prefixes from all messages (they're internal markers,
	// not for the API). Do this before placing breakpoints so the API never sees them.
	for i := range result {
		stripSystemInjected(result[i])
	}

	// Cache-control hoisting: for tool_result blocks that have cache_control
	// on their inner text block, hoist the marker to the tool_result level
	// itself and flatten the content to a string. This prevents the content
	// shape from flipping between "string" and [{text, cache_control}] across
	// turns, which destroys cache_read hit rate because the cached prefix changes.
	// Inspired by openclacky's cache_control hoisting in message_format/anthropic.rb.
	for i := range result {
		hoistToolResultCache(result[i])
	}

	// Collect candidate indices for breakpoint placement, skipping system-injected
	// messages. Injected content (session memory, file recovery) changes between
	// turns, so placing breakpoints there would cause cache misses.
	candidates := make([]int, 0, len(result))
	for i := range result {
		if !isSystemInjected(result[i]) {
			candidates = append(candidates, i)
		}
	}

	if len(candidates) == 0 {
		// All messages are system-injected; fall back to last message
		candidates = []int{len(result) - 1}
	}

	// Determine starting position for skipCacheWrite mode.
	startOffset := 0
	if cfg.SkipCacheWrite && len(candidates) >= 2 {
		startOffset = 1 // skip the last candidate, use second-to-last as first breakpoint
	}

	// Place breakpoints on the last N non-injected candidates (up to maxBP).
	// Rolling cache: Turn N's last message (marked) becomes Turn N+1's second-to-last
	// → cache READ hit on the prefix.
	breakpointsPlaced := 0
	for i := len(candidates) - 1 - startOffset; i >= 0 && breakpointsPlaced < maxBP; i-- {
		applyCacheMarker(result[candidates[i]], marker)
		breakpointsPlaced++
	}

	// Also apply a breakpoint to the system prompt (first message if system role).
	// The system prompt breakpoint is separate from the message breakpoints.
	if role, _ := result[0]["role"].(string); role == "system" {
		applyCacheMarker(result[0], marker)
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

	// Array content -> add cache_control to last block.
	// Cache-control hoisting: if the last block is a tool_result,
	// place cache_control on the tool_result block itself (not any
	// nested text block). This prevents the content shape from
	// flipping between "string" and [{text, cache_control}] depending
	// on whether this message is the current cache breakpoint. Shape
	// mutation destroys cache_read hit rate because the cached prefix
	// changes every turn. Inspired by openclacky's cache_control hoisting.
	if arr, ok := content.([]any); ok && len(arr) > 0 {
		last := arr[len(arr)-1]
		if m, ok := last.(map[string]any); ok {
			// Hoist: if this is a tool_result, ensure cache_control is on
			// the tool_result block itself. For non-tool_result blocks,
			// place directly on the block.
			m["cache_control"] = marker
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
	marker := map[string]any{"type": "ephemeral", "scope": "global"}
	if ttl == "1h" {
		marker = map[string]any{"type": "ephemeral", "ttl": "1h", "scope": "global"}
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
	globalMarker := map[string]any{"type": "ephemeral", "scope": "global"}
	if ttl == "1h" {
		globalMarker = map[string]any{"type": "ephemeral", "ttl": "1h", "scope": "global"}
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

// buildSystemBlocks converts a system prompt into []anthropic.TextBlockParam
// with cache_control markers. Uses the static/dynamic boundary for partitioned
// caching: the static part gets its own cache_control marker so dynamic changes
// (skills, memory, todo) don't invalidate static tool descriptions.
func buildSystemBlocks(prompt string, ttl string) []anthropic.TextBlockParam {
	blocks := FormatBoundaryCachedSystemPrompt(prompt, ttl)
	result := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		text, _ := block["text"].(string)
		tb := anthropic.TextBlockParam{Text: text}
		if cc, ok := block["cache_control"]; ok {
			if cm, ok := cc.(map[string]any); ok {
				if cm["type"] == "ephemeral" {
					if ttlVal, hasTTL := cm["ttl"]; hasTTL {
						tb.CacheControl = anthropic.CacheControlEphemeralParam{
							Type: "ephemeral",
							TTL:  anthropic.CacheControlEphemeralTTL(ttlVal.(string)),
						}
					} else {
						tb.CacheControl = anthropic.NewCacheControlEphemeralParam()
					}
				}
			}
		}
		result = append(result, tb)
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
	postCompactionReset bool                        // skip next DetectBreak — compaction just ran
	breakCount          int                         // total breaks detected this session
	latchAfter          int                         // after N breaks, stop detecting (default: 3)
	latched             bool                        // detection disabled after latch triggered
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
//
// Session stability: if postCompactionReset is set, skips detection (compaction
// legitimately reduces cache tokens). After breakCount >= latchAfter, detection
// is disabled to prevent cascading false positives from mid-session changes.
func (d *CacheBreakDetector) DetectBreak(currentCacheReadTokens int64) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	// Latch: after too many breaks, stop detecting to prevent cascading false positives
	if d.latched {
		return false
	}

	// Compaction guard: skip detection on the first call after compaction
	if d.postCompactionReset {
		d.postCompactionReset = false
		return false
	}

	// Method 1: Category-based prediction
	// If we've recorded changes with significant estimated impact,
	// predict a break even before seeing the API response.
	// The threshold is based on the baseline — if estimated impact > 10% of baseline,
	// a break is likely.
	if d.baselineSet && d.lastCacheReadTokens > 0 && d.estimatedImpact > 0 {
		categoryThreshold := d.lastCacheReadTokens / 10 // 10% of baseline
		if d.estimatedImpact > categoryThreshold {
			d.recordBreak()
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
	if drop > threshold {
		d.recordBreak()
		return true
	}
	return false
}

// recordBreak increments the break counter and enables the latch if threshold reached.
// Must be called with d.mu held.
func (d *CacheBreakDetector) recordBreak() {
	if d.latchAfter <= 0 {
		d.latchAfter = 3
	}
	d.breakCount++
	if d.breakCount >= d.latchAfter {
		d.latched = true
	}
}

// CacheBreak captures a detected cache break event for diagnostics.
// Upstream: promptCacheBreakDetection.ts writes diff files on cache breaks.
type CacheBreak struct {
	Timestamp    time.Time
	Dimension    string  // "model", "system", "tools", "betas", "compaction", "eviction", "unknown"
	BeforeTokens int64   // baseline cache_read_tokens
	AfterTokens  int64   // current cache_read_tokens
	DropPercent  float64 // percentage drop
	Details      string  // free-form context
}

// WriteDiagnosticFile writes a cache break diagnostic file to the temp directory
// if the break is significant (>10% drop and >5000 absolute drop).
// Upstream: writes diff files to temp on cache breaks with dimension analysis.
func (d *CacheBreakDetector) WriteDiagnosticFile(before, after int64, details string) string {
	drop := before - after
	if before <= 0 {
		return ""
	}
	pct := float64(drop) / float64(before) * 100

	// Only write diagnostics for significant breaks
	if pct < 10 || drop < 5000 {
		return ""
	}

	brk := CacheBreak{
		Timestamp:    time.Now(),
		Dimension:    d.detectDimension(details),
		BeforeTokens: before,
		AfterTokens:  after,
		DropPercent:  pct,
		Details:      details,
	}

	dir := os.TempDir()
	filename := fmt.Sprintf("cache_break_%s.txt", time.Now().Format("20060102_150405"))
	fpath := filepath.Join(dir, filename)

	f, err := os.Create(fpath)
	if err != nil {
		return ""
	}
	defer f.Close()

	fmt.Fprintf(f, "Cache Break Diagnostic\n")
	fmt.Fprintf(f, "======================\n\n")
	fmt.Fprintf(f, "Timestamp: %s\n", brk.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(f, "Dimension: %s\n", brk.Dimension)
	fmt.Fprintf(f, "Before:   %d tokens\n", brk.BeforeTokens)
	fmt.Fprintf(f, "After:    %d tokens\n", brk.AfterTokens)
	fmt.Fprintf(f, "Drop:     %d tokens (%.1f%%)\n", brk.BeforeTokens-brk.AfterTokens, brk.DropPercent)
	fmt.Fprintf(f, "\nDetails:\n%s\n", brk.Details)

	return fpath
}

// detectDimension infers the likely cause of a cache break from the details string.
// Matches upstream's dimension tracking in promptCacheBreakDetection.ts.
func (d *CacheBreakDetector) detectDimension(details string) string {
	lower := strings.ToLower(details)
	switch {
	case strings.Contains(lower, "model"):
		return "model"
	case strings.Contains(lower, "system prompt") || strings.Contains(lower, "system_prompt"):
		return "system"
	case strings.Contains(lower, "tool"):
		return "tools"
	case strings.Contains(lower, "beta"):
		return "betas"
	case strings.Contains(lower, "compact"):
		return "compaction"
	case strings.Contains(lower, "evict") || strings.Contains(lower, "ttl"):
		return "eviction"
	default:
		return "unknown"
	}
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

// MarkPostCompaction sets the post-compaction guard so the next DetectBreak
// call returns false (compaction legitimately reduces cache tokens).
func (d *CacheBreakDetector) MarkPostCompaction() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.postCompactionReset = true
}

// LastBaseline returns the last recorded cache_read_tokens baseline.
// Read-only accessor, safe for logging.
func (d *CacheBreakDetector) LastBaseline() int64 {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastCacheReadTokens
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
