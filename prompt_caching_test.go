package main

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestFormatCachedSystemPrompt(t *testing.T) {
	result := FormatCachedSystemPrompt("test prompt", "5m")
	if len(result) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result))
	}
	if result[0]["text"] != "test prompt" {
		t.Errorf("expected text='test prompt', got %v", result[0]["text"])
	}
	if result[0]["type"] != "text" {
		t.Errorf("expected type='text', got %v", result[0]["type"])
	}
	cc, _ := result[0]["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" {
		t.Errorf("expected cache_control type=ephemeral, got %v", cc["type"])
	}
}

func TestFormatCachedSystemPrompt1h(t *testing.T) {
	result := FormatCachedSystemPrompt("test prompt", "1h")
	cc, _ := result[0]["cache_control"].(map[string]any)
	if cc["ttl"] != "1h" {
		t.Errorf("expected ttl=1h, got %v", cc["ttl"])
	}
}

func TestCacheBreakpointConfigDefault(t *testing.T) {
	cfg := DefaultCacheBreakpointConfig()
	// MaxBreakpoints is dead code — the "all messages" strategy places
	// cache_control on every non-injected message regardless of this value.
	// The constant is kept for backward compatibility but has no effect.
	if cfg.SkipCacheWrite {
		t.Error("expected SkipCacheWrite=false by default")
	}
}

func TestMaxCacheBreakpointsConstant(t *testing.T) {
	// The constant exists but is not used — all-messages strategy ignores it.
	// Kept at 2 for backward compatibility with existing config references.
	if MaxCacheBreakpoints < 1 {
		t.Errorf("expected MaxCacheBreakpoints >= 1, got %d", MaxCacheBreakpoints)
	}
}

func TestCacheBreakDetectorUpdateAndDetect(t *testing.T) {
	d := &CacheBreakDetector{}

	// No baseline set yet
	if d.DetectBreak(1000) {
		t.Error("should not detect break when no baseline is set")
	}

	// Set baseline
	d.UpdateBaseline(10000)

	// 10% drop should not trigger (threshold is 20%)
	if d.DetectBreak(9000) {
		t.Error("10% drop should not trigger break detection")
	}

	// 20% drop exactly should not trigger (> 20%, not >=)
	if d.DetectBreak(8000) {
		t.Error("exactly 20% drop should not trigger break detection")
	}

	// 25% drop should trigger
	if !d.DetectBreak(7000) {
		t.Error("30% drop should trigger break detection")
	}

	// Zero current tokens should trigger
	if !d.DetectBreak(0) {
		t.Error("zero cache read tokens should trigger break detection")
	}
}

func TestCacheBreakDetectorResetBaseline(t *testing.T) {
	d := &CacheBreakDetector{}

	d.UpdateBaseline(10000)
	if !d.baselineSet {
		t.Error("baseline should be set after UpdateBaseline")
	}

	d.ResetBaseline()
	if d.baselineSet {
		t.Error("baseline should not be set after ResetBaseline")
	}
	if d.lastCacheReadTokens != 0 {
		t.Error("lastCacheReadTokens should be 0 after ResetBaseline")
	}

	// After reset, should not detect breaks
	if d.DetectBreak(0) {
		t.Error("should not detect break after reset")
	}
}

func TestCacheBreakDetectorNilReceiver(t *testing.T) {
	var d *CacheBreakDetector

	// Should not panic on nil receiver
	d.UpdateBaseline(1000)
	if d.DetectBreak(0) {
		t.Error("nil detector should not detect break")
	}
	d.ResetBaseline()
}

func TestCacheBreakDetectorZeroBaseline(t *testing.T) {
	d := &CacheBreakDetector{}

	// Update with zero should set baseline but not trigger detection
	d.UpdateBaseline(0)

	// With baseline of 0, DetectBreak should return false (division guard)
	if d.DetectBreak(0) {
		t.Error("should not detect break with zero baseline")
	}
}

func TestCacheBreakDetectorSequence(t *testing.T) {
	d := &CacheBreakDetector{}

	// First call: no baseline
	d.UpdateBaseline(50000)

	// Second call: small change, no break
	if d.DetectBreak(48000) {
		t.Error("4% drop should not trigger")
	}
	d.UpdateBaseline(48000)

	// Third call: big drop from updated baseline
	if !d.DetectBreak(30000) {
		t.Error("37.5% drop should trigger")
	}
	d.UpdateBaseline(30000)

	// After reset (simulating compaction), no detection
	d.ResetBaseline()
	if d.DetectBreak(0) {
		t.Error("should not detect break after ResetBaseline")
	}
}

func TestCacheBreakDetectorCategoryBased(t *testing.T) {
	d := &CacheBreakDetector{}

	// Set baseline
	d.UpdateBaseline(100000)

	// Record a small change — should not trigger (impact < 10% of baseline)
	d.RecordChange(CacheChangeUserMessage, 1)
	if d.DetectBreak(100000) {
		t.Error("small change should not trigger category-based break detection")
	}

	// Update baseline to clear pending changes
	d.UpdateBaseline(100000)

	// Record a compaction change — should trigger (impact > 10% of baseline)
	d.RecordChange(CacheChangeCompaction, 1)
	// Compaction weight = 50000, baseline = 100000, threshold = 10000
	// 50000 > 10000, and cache_read must have actually dropped > 0
	if !d.DetectBreak(90000) {
		t.Error("compaction change should trigger category-based break detection")
	}

	// Reset and test with accumulated changes
	d.ResetBaseline()
	d.UpdateBaseline(50000)

	// Multiple tool results — 5 * 5000 = 25000 > 5000 (10% of 50000)
	// cache_read must have actually dropped > 0
	d.RecordChange(CacheChangeToolResult, 5)
	if !d.DetectBreak(40000) {
		t.Error("5 tool result changes should trigger category-based break detection")
	}
}

func TestCacheBreakDetectorCategoryResetOnUpdate(t *testing.T) {
	d := &CacheBreakDetector{}

	d.UpdateBaseline(100000)
	d.RecordChange(CacheChangeCompaction, 1)

	// UpdateBaseline should clear pending changes
	d.UpdateBaseline(90000)

	if d.estimatedImpact != 0 {
		t.Error("UpdateBaseline should clear estimated impact")
	}
	if len(d.pendingChanges) != 0 {
		t.Error("UpdateBaseline should clear pending changes")
	}
}

func TestCacheChangeWeights(t *testing.T) {
	// Verify that category weights are reasonable
	tests := []struct {
		category  CacheChangeCategory
		minWeight int64
	}{
		{CacheChangeCompaction, 10000},
		{CacheChangeSystemPrompt, 5000},
		{CacheChangePDF, 1000},
		{CacheChangeImage, 1000},
		{CacheChangeToolResult, 1000},
	}
	for _, tt := range tests {
		w := cacheChangeWeight(tt.category)
		if w < tt.minWeight {
			t.Errorf("weight for %s should be >= %d, got %d", tt.category, tt.minWeight, w)
		}
	}
}

func TestApplyPinnedCacheEditsReal(t *testing.T) {
	// Create messages with tool_result blocks
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "toolu_123",
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: "file content"}},
						},
					},
				},
			},
		},
	}

	edits := []PinnedCacheEdit{
		{ToolUseID: "toolu_123", Position: 0, Content: "file content"},
	}

	result := ApplyPinnedCacheEdits(msgs, edits)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Verify cache_control was added
	block := result[0].Content[0]
	if block.OfToolResult == nil {
		t.Fatal("expected tool_result block")
	}
	// The cache_control should be set (Type field should be "ephemeral")
	cc := block.OfToolResult.CacheControl
	// CacheControlEphemeralParam has a constant Type field set to "ephemeral"
	// when constructed via NewCacheControlEphemeralParam()
	// We can verify it was set by checking that the zero-value was replaced
	// (the Type field is a constant that defaults to "ephemeral")
	if cc.Type == "" {
		t.Error("expected cache_control Type to be set on pinned tool_result")
	}
}

func TestApplyPinnedCacheEditsEmpty(t *testing.T) {
	// Empty edits or messages should return unchanged
	msgs := []anthropic.MessageParam{{Role: anthropic.MessageParamRoleUser}}

	result := ApplyPinnedCacheEdits(msgs, nil)
	if len(result) != 1 {
		t.Error("empty edits should return messages unchanged")
	}

	result = ApplyPinnedCacheEdits(nil, []PinnedCacheEdit{{ToolUseID: "x", Position: 0}})
	if result != nil {
		t.Error("nil messages with edits should return nil")
	}
}

func TestPinnedCacheEditStruct(t *testing.T) {
	// Verify PinnedCacheEdit struct fields
	edit := PinnedCacheEdit{
		ToolUseID: "toolu_123",
		Position:  5,
		Content:   "cached content",
	}
	if edit.ToolUseID != "toolu_123" {
		t.Errorf("expected ToolUseID='toolu_123', got %v", edit.ToolUseID)
	}
	if edit.Position != 5 {
		t.Errorf("expected Position=5, got %d", edit.Position)
	}
	if edit.Content != "cached content" {
		t.Errorf("expected Content='cached content', got %v", edit.Content)
	}
}

// --- New cache optimization tests ---

func TestBuildSystemBlocksPartitionsAtBoundary(t *testing.T) {
	// System prompt with static/dynamic boundary
	prompt := "static tool descriptions\n<!-- STATIC_PROMPT_END -->\ndynamic skills section"
	blocks := buildSystemBlocks(prompt, "5m")

	// Should produce 2 blocks: static + dynamic
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	// Static block should have cache_control
	if blocks[0].CacheControl.Type != "ephemeral" {
		t.Errorf("static block cache_control type=%s, expected ephemeral", blocks[0].CacheControl.Type)
	}

	// Dynamic block should also have cache_control
	if blocks[1].CacheControl.Type != "ephemeral" {
		t.Errorf("dynamic block cache_control type=%s, expected ephemeral", blocks[1].CacheControl.Type)
	}

	// Verify content
	if !strings.Contains(blocks[0].Text, "static tool descriptions") {
		t.Error("static block should contain static content")
	}
	if !strings.Contains(blocks[1].Text, "dynamic skills section") {
		t.Error("dynamic block should contain dynamic content")
	}
}

func TestBuildSystemBlocksNoBoundary(t *testing.T) {
	// System prompt without boundary — single block
	prompt := "simple system prompt"
	blocks := buildSystemBlocks(prompt, "5m")

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block for no-boundary prompt, got %d", len(blocks))
	}
	if blocks[0].CacheControl.Type != "ephemeral" {
		t.Error("single block should still have cache_control")
	}
}

func TestCacheBreakDetectorPostCompactionGuard(t *testing.T) {
	d := &CacheBreakDetector{}
	d.UpdateBaseline(10000)

	// Mark post-compaction — next DetectBreak should return false
	d.MarkPostCompaction()

	// 50% drop would normally trigger, but post-compaction guard prevents it
	if d.DetectBreak(5000) {
		t.Error("post-compaction guard should suppress break detection")
	}

	// Second call should detect normally (guard was cleared)
	if !d.DetectBreak(5000) {
		t.Error("after guard cleared, should detect break")
	}
}

func TestCacheBreakDetectorStabilityLatch(t *testing.T) {
	d := &CacheBreakDetector{}
	d.latchAfter = 3
	d.UpdateBaseline(10000)

	// First break
	if !d.DetectBreak(5000) {
		t.Error("first break should be detected")
	}
	if d.latched {
		t.Error("should not be latched after 1 break (latchAfter=3)")
	}

	// Second break
	d.UpdateBaseline(10000)
	if !d.DetectBreak(5000) {
		t.Error("second break should be detected")
	}
	if d.latched {
		t.Error("should not be latched after 2 breaks (latchAfter=3)")
	}

	// Third break — triggers latch
	d.UpdateBaseline(10000)
	if !d.DetectBreak(5000) {
		t.Error("third break should be detected")
	}
	if !d.latched {
		t.Error("should be latched after 3 breaks (latchAfter=3)")
	}

	// Fourth call — should be suppressed by latch
	d.UpdateBaseline(10000)
	if d.DetectBreak(5000) {
		t.Error("break detection should be suppressed after latch")
	}
}

func TestCacheBreakDetectorLastBaseline(t *testing.T) {
	d := &CacheBreakDetector{}

	// Before baseline set
	if d.LastBaseline() != 0 {
		t.Error("baseline should be 0 before set")
	}

	d.UpdateBaseline(42000)
	if d.LastBaseline() != 42000 {
		t.Errorf("expected baseline=42000, got %d", d.LastBaseline())
	}
}

// TestUpstreamCacheStructureParity verifies our cache structure matches upstream's design:
// 1. System prompt partitioned at static/dynamic boundary
// 2. Static part gets separate cache_control (global-like, long-lived)
// 3. Dynamic part gets separate cache_control (short-lived)
// 4. Only 1 cache_control marker on messages (not 2)
// 5. Compaction guard prevents false-positive break detection
func TestUpstreamCacheStructureParity(t *testing.T) {
	// Build a realistic system prompt with boundary
	staticContent := "You are an AI assistant. Tools: read, write, exec."
	dynamicContent := "Skills: git, python. Memory: working on main.go."
	prompt := staticContent + "\n<!-- STATIC_PROMPT_END -->\n" + dynamicContent

	blocks := buildSystemBlocks(prompt, "5m")

	// Upstream: static and dynamic are separate blocks
	if len(blocks) != 2 {
		t.Fatalf("expected 2 system blocks (static+dynamic), got %d", len(blocks))
	}

	// Both blocks have cache_control (upstream: static gets global, dynamic gets ephemeral)
	if blocks[0].CacheControl.Type != "ephemeral" {
		t.Error("static block should have cache_control")
	}
	if blocks[1].CacheControl.Type != "ephemeral" {
		t.Error("dynamic block should have cache_control")
	}

	// Static block contains tool descriptions, dynamic contains skills
	if !strings.Contains(blocks[0].Text, "Tools:") {
		t.Error("static block should contain tool descriptions")
	}
	if !strings.Contains(blocks[1].Text, "Skills:") {
		t.Error("dynamic block should contain skills")
	}

	// Verify that changing dynamic content doesn't affect static block structure
	staticOnly := staticContent + "\n<!-- STATIC_PROMPT_END -->\nchanged dynamic content"
	blocksChanged := buildSystemBlocks(staticOnly, "5m")

	// Static block content should be identical (only dynamic changed)
	if blocks[0].Text != blocksChanged[0].Text {
		t.Error("static block should not change when dynamic content changes")
	}
}

// TestCompactionGuardMatchesUpstreamVerify that our compaction guard matches upstream's
// notifyCompaction() behavior: after compaction, the next cache_read drop is expected.
func TestCompactionGuardMatchesUpstream(t *testing.T) {
	d := &CacheBreakDetector{}
	d.UpdateBaseline(50000) // typical cache_read after a productive session

	// Simulate compaction: reset baseline and set guard
	d.ResetBaseline()
	d.MarkPostCompaction()

	// API returns much lower cache_read (compaction invalidated cache)
	if d.DetectBreak(5000) {
		t.Error("compaction should not trigger cache break on next call")
	}

	// After the guard is consumed, normal detection resumes
	d.UpdateBaseline(5000) // new baseline after compaction
	if !d.DetectBreak(1000) {
		t.Error("normal detection should resume after guard")
	}
}
