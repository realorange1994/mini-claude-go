package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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

// DefaultCacheBreakpointConfig returns the default config with 2 breakpoints
// for the rolling cache strategy: Turn N marks messages[-2] and messages[-1];
// Turn N+1, messages[-1] from Turn N becomes messages[-2] (still marked)
// → cache READ hit on the prefix.
func DefaultCacheBreakpointConfig() CacheBreakpointConfig {
	return CacheBreakpointConfig{
		MaxBreakpoints: 2,
		SkipCacheWrite: false,
	}
}

const (
	// MaxCacheBreakpoints is the maximum number of cache breakpoints to place
	// in the API message stream. 2 breakpoints enable the rolling cache strategy.
	MaxCacheBreakpoints = 2
)

// cacheMessageParams applies prompt caching using SDK types directly,
// avoiding JSON round-trip that can cause SDK unmarshaler to drop tool_result blocks.
func cacheMessageParams(params *anthropic.MessageNewParams, ttl string) {
	if params.Messages == nil || len(params.Messages) == 0 {
		return
	}

	var cacheCtrl anthropic.CacheControlEphemeralParam
	switch ttl {
	case "1h":
		cacheCtrl = anthropic.CacheControlEphemeralParam{Type: "ephemeral", TTL: "1h"}
	default:
		cacheCtrl = anthropic.CacheControlEphemeralParam{Type: "ephemeral", TTL: "5m"}
	}

	// Find the breakpoint index (last non-system-injected message)
	breakpointIdx := len(params.Messages) - 1
	for i := len(params.Messages) - 1; i >= 0; i-- {
		msg := &params.Messages[i]
		if (msg.Role == anthropic.MessageParamRoleUser || msg.Role == anthropic.MessageParamRoleAssistant) &&
			!isSystemInjectedSDK(msg) {
			breakpointIdx = i
			break
		}
	}

	if breakpointIdx >= 0 && breakpointIdx < len(params.Messages) {
		msg := &params.Messages[breakpointIdx]
		if len(msg.Content) == 0 {
			return
		}
		lastIdx := len(msg.Content) - 1

		if msg.Content[lastIdx].OfToolResult != nil {
			msg.Content[lastIdx].OfToolResult.CacheControl = cacheCtrl
		} else if msg.Content[lastIdx].OfText != nil {
			msg.Content[lastIdx].OfText.CacheControl = cacheCtrl
		}
	}
}

func isSystemInjectedSDK(msg *anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfText != nil && strings.Contains(block.OfText.Text, "<!-- system-injected -->") {
			return true
		}
	}
	return false
}

// getCacheTTL determines the cache TTL based on session activity.
// When the session is active (recent API calls), locks the TTL to "1h" to prevent
// mid-session KV cache eviction. When idle for >5 minutes, allows the cache to
// expire naturally by using a shorter TTL.
//
// Upstream: session-stable TTL locking (claude.ts) — the cache TTL is extended
// on each API call so the KV cache stays warm during active sessions. Without
// this, a 5-minute idle between turns can evict the entire cached prefix.
func (a *AgentLoop) getCacheTTL() string {
	now := time.Now()

	// If we have a TTL lock that hasn't expired, keep using "1h"
	ttlUnix := atomic.LoadInt64(&a.ttlLockedUntilUnix)
	if ttlUnix > 0 && time.Unix(ttlUnix, 0).After(now) {
		return "1h"
	}

	// If the session has been idle for >5 minutes, let the cache expire
	// naturally (shorter TTL). This prevents wasting server-side cache
	// resources on inactive sessions.
	// Note: lastApiCompletionTime is read directly here; it was already being
	// accessed without locks in other parts of the codebase.
	if !a.lastApiCompletionTime.IsZero() && now.Sub(a.lastApiCompletionTime) > 5*time.Minute {
		return "5m"
	}

	// Session is active or recently active — lock TTL to 1h for 10 minutes
	// from now. This ensures the KV cache survives the gap between turns.
	atomic.StoreInt64(&a.ttlLockedUntilUnix, now.Add(10*time.Minute).Unix())
	return "1h"
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
					ccParam := anthropic.CacheControlEphemeralParam{Type: "ephemeral"}
					if ttlVal, hasTTL := cm["ttl"]; hasTTL {
						ccParam.TTL = anthropic.CacheControlEphemeralTTL(ttlVal.(string))
					}
					// Preserve scope field via SetExtraFields — the SDK's
					// CacheControlEphemeralParam doesn't have a Scope field,
					// but paramObj.SetExtraFields injects it into the JSON
					// output. This is critical for the static system prompt
					// block which uses scope:"global" for cross-user cache
					// sharing on the Anthropic API.
					if scopeVal, hasScope := cm["scope"]; hasScope {
						ccParam.SetExtraFields(map[string]any{"scope": scopeVal})
					}
					tb.CacheControl = ccParam
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
	CacheChangeToolResult    CacheChangeCategory = "tool_result"
	CacheChangeThinking      CacheChangeCategory = "thinking"
	CacheChangeImage         CacheChangeCategory = "image"
	CacheChangePDF           CacheChangeCategory = "pdf"
	CacheChangeAttachment    CacheChangeCategory = "attachment"
	CacheChangeSystemPrompt  CacheChangeCategory = "system_prompt"
	CacheChangeCompaction    CacheChangeCategory = "compaction"
	CacheChangeEdit          CacheChangeCategory = "edit"
	CacheChangeUserMessage   CacheChangeCategory = "user_message"
	CacheChangeToolUse       CacheChangeCategory = "tool_use"
	CacheChangeNormalization CacheChangeCategory = "normalization"
	CacheChangeOther         CacheChangeCategory = "other"
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

	if !d.baselineSet || d.lastCacheReadTokens == 0 {
		return false
	}

	// Always compute actual drop — both methods require it
	drop := d.lastCacheReadTokens - currentCacheReadTokens

	// Method 1: Category-based prediction
	// If estimated impact > 10% of baseline AND actual drop > 0, detect break.
	// The drop > 0 requirement prevents false positives when changes are tracked
	// but the API's cache_read didn't actually change (e.g., cache miss was avoided
	// due to other factors).
	if d.estimatedImpact > 0 {
		categoryThreshold := d.lastCacheReadTokens / 10 // 10% of baseline
		if d.estimatedImpact > categoryThreshold && drop > 0 {
			d.recordBreak()
			return true
		}
	}

	// Method 2: Token-based fallback
	// A break is detected when cache_read dropped by more than 20% from baseline.
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

	d.mu.Lock()
	pendingCopy := make(map[CacheChangeCategory]int, len(d.pendingChanges))
	for k, v := range d.pendingChanges {
		pendingCopy[k] = v
	}
	impact := d.estimatedImpact
	d.mu.Unlock()

	// Infer dimension from pendingChanges, not from the details string
	dimension := inferDimensionFromChanges(pendingCopy, details)

	brk := CacheBreak{
		Timestamp:    time.Now(),
		Dimension:    dimension,
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
	fmt.Fprintf(f, "Estimated Impact: %d tokens\n", impact)
	fmt.Fprintf(f, "\nDetails:\n%s\n", brk.Details)
	if len(pendingCopy) > 0 {
		fmt.Fprintf(f, "\nPending Changes:\n")
		for cat, count := range pendingCopy {
			fmt.Fprintf(f, "  %s: %d (weight: %d)\n", cat, count, cacheChangeWeight(cat))
		}
	} else {
		fmt.Fprintf(f, "\nPending Changes: none recorded (break was not predicted by category tracking)\n")
	}

	return fpath
}

// inferDimensionFromChanges determines the likely dimension from pending changes
// and the details string. Prioritizes category-based analysis over text matching.
func inferDimensionFromChanges(changes map[CacheChangeCategory]int, details string) string {
	// Check changes by impact weight (heaviest first)
	type catImpact struct {
		cat    CacheChangeCategory
		weight int64
	}
	var impacts []catImpact
	for cat, count := range changes {
		impacts = append(impacts, catImpact{cat, cacheChangeWeight(cat) * int64(count)})
	}
	sort.Slice(impacts, func(i, j int) bool { return impacts[i].weight > impacts[j].weight })

	if len(impacts) > 0 {
		// Map heaviest change category to dimension
		switch impacts[0].cat {
		case CacheChangeCompaction:
			return "compaction"
		case CacheChangeSystemPrompt:
			return "system"
		case CacheChangeToolResult:
			return "tool_result"
		case CacheChangeToolUse:
			return "tool_use"
		case CacheChangeThinking:
			return "thinking"
		case CacheChangeEdit:
			return "edit"
		case CacheChangeNormalization:
			return "normalization"
		case CacheChangeAttachment:
			return "attachment"
		case CacheChangeImage:
			return "image"
		case CacheChangePDF:
			return "pdf"
		case CacheChangeUserMessage:
			return "user_message"
		default:
			return "other"
		}
	}

	// Fallback: text matching from details string
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
	case strings.Contains(lower, "normaliz"):
		return "normalization"
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
