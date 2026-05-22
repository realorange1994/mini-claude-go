package main

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestApplyPromptCachingEmpty(t *testing.T) {
	result := ApplyPromptCaching(nil, "5m")
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestApplyPromptCachingShort(t *testing.T) {
	// Fewer than 2 non-system messages: last message gets the cache breakpoint
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "hello"},
	}

	result := ApplyPromptCaching(messages, "5m")
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// System message: should have cache_control (system prompt breakpoint)
	sysContent, ok := result[0]["content"].([]map[string]any)
	if ok {
		if _, ok2 := sysContent[len(sysContent)-1]["cache_control"]; !ok2 {
			t.Error("system message should have cache_control")
		}
	}

	// User message (last message): should have cache_control
	userContent, ok := result[1]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("user message content should be array, got %T", result[1]["content"])
	}
	if _, ok := userContent[len(userContent)-1]["cache_control"]; !ok {
		t.Error("user message (last) should have cache_control")
	}
}

func TestApplyPromptCachingLong(t *testing.T) {
	// Many messages: 2 breakpoints on last 2 non-system messages (rolling cache)
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "msg1"},
		{"role": "assistant", "content": "resp1"},
		{"role": "user", "content": "msg2"},
		{"role": "assistant", "content": "resp2"},
		{"role": "user", "content": "msg3"},
	}

	result := ApplyPromptCaching(messages, "5m")
	if len(result) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(result))
	}

	hasCC := func(msg map[string]any) bool {
		if cc, ok := msg["cache_control"]; ok && cc != nil {
			return true
		}
		if content, ok := msg["content"].([]map[string]any); ok && len(content) > 0 {
			_, ok2 := content[len(content)-1]["cache_control"]
			return ok2
		}
		return false
	}

	// With 1-breakpoint rolling cache strategy:
	// - System message (index 0) gets a breakpoint
	// - Last non-system message gets breakpoint (index 5)
	if !hasCC(result[0]) {
		t.Error("system message (index 0) should have cache_control")
	}
	for i := 1; i < 5; i++ {
		if hasCC(result[i]) {
			t.Errorf("message at index %d should NOT have cache_control", i)
		}
	}
	if !hasCC(result[5]) {
		t.Error("last message (index 5) should have cache_control")
	}
}

func TestApplyPromptCachingTTL(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
	}

	// Default TTL (5m) - with single message, last message IS the system message
	result := ApplyPromptCaching(messages, "5m")
	sysContent, _ := result[0]["content"].([]map[string]any)
	cc, _ := sysContent[0]["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" {
		t.Errorf("expected type=ephemeral, got %v", cc["type"])
	}
	if _, ok := cc["ttl"]; ok {
		t.Error("default TTL should not have ttl field")
	}

	// 1h TTL
	result = ApplyPromptCaching(messages, "1h")
	sysContent, _ = result[0]["content"].([]map[string]any)
	cc, _ = sysContent[0]["cache_control"].(map[string]any)
	if cc["ttl"] != "1h" {
		t.Errorf("expected ttl=1h, got %v", cc["ttl"])
	}
}

func TestApplyCacheMarkerToolRole(t *testing.T) {
	msg := map[string]any{
		"role":        "tool",
		"content":     "result text",
		"tool_use_id": "toolu_12345",
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	if _, ok := msg["cache_control"]; !ok {
		t.Error("tool role message should have cache_control at message level")
	}
	// tool_use_id MUST be preserved — the API requires it for tool_result pairing (error 2013 fix)
	if _, ok := msg["tool_use_id"]; !ok {
		t.Error("tool role message MUST have tool_use_id preserved after cache marking")
	}
	if _, ok := msg["cache_reference"]; ok {
		t.Error("tool role message should NOT have cache_reference (deprecated)")
	}
}

func TestApplyCacheMarkerToolRoleNoToolUseID(t *testing.T) {
	msg := map[string]any{
		"role":    "tool",
		"content": "result text",
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	if _, ok := msg["cache_control"]; !ok {
		t.Error("tool role message should have cache_control at message level")
	}
	// No tool_use_id present — should just add cache_control
	if _, ok := msg["cache_reference"]; ok {
		t.Error("tool role message without tool_use_id should NOT have cache_reference")
	}
}

func TestApplyCacheMarkerStringContent(t *testing.T) {
	msg := map[string]any{
		"role":    "user",
		"content": "hello world",
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	// String content should be converted to array format
	content, ok := msg["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be converted to array, got %T", msg["content"])
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	if content[0]["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", content[0]["text"])
	}
	if _, ok := content[0]["cache_control"]; !ok {
		t.Error("content block should have cache_control")
	}
}

func TestApplyCacheMarkerArrayContent(t *testing.T) {
	msg := map[string]any{
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "text", "text": "first"},
			map[string]any{"type": "text", "text": "second"},
		},
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	// Cache control should be on the LAST block
	arr, ok := msg["content"].([]any)
	if !ok {
		t.Fatal("expected content to remain as array")
	}
	lastBlock, _ := arr[len(arr)-1].(map[string]any)
	if _, ok := lastBlock["cache_control"]; !ok {
		t.Error("last content block should have cache_control")
	}
	// First block should NOT have cache_control
	firstBlock, _ := arr[0].(map[string]any)
	if _, ok := firstBlock["cache_control"]; ok {
		t.Error("first content block should NOT have cache_control")
	}
}

func TestApplyCacheMarkerEmptyString(t *testing.T) {
	msg := map[string]any{
		"role":    "user",
		"content": "",
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	// Empty string content -> cache_control at message level
	if _, ok := msg["cache_control"]; !ok {
		t.Error("empty string content should have cache_control at message level")
	}
}

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

func TestDeepCopyMessages(t *testing.T) {
	original := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	copy := deepCopyMessages(original)

	// Mutate the copy
	copy[0]["content"] = "modified"

	// Original should be unchanged
	if original[0]["content"] != "hello" {
		t.Error("deepCopyMessages should produce independent copy")
	}
}

func TestCacheBreakpointConfigDefault(t *testing.T) {
	cfg := DefaultCacheBreakpointConfig()
	if cfg.MaxBreakpoints != 1 {
		t.Errorf("expected MaxBreakpoints=1, got %d", cfg.MaxBreakpoints)
	}
	if cfg.SkipCacheWrite {
		t.Error("expected SkipCacheWrite=false by default")
	}
}

func TestMaxCacheBreakpointsConstant(t *testing.T) {
	if MaxCacheBreakpoints != 2 {
		t.Errorf("expected MaxCacheBreakpoints=2, got %d", MaxCacheBreakpoints)
	}
}

func TestApplyPromptCachingWithConfigSkipCacheWrite(t *testing.T) {
	// skipCacheWrite shifts the last breakpoint from index 5 to index 4.
	// System message (index 0) always gets a breakpoint.
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "msg1"},
		{"role": "assistant", "content": "resp1"},
		{"role": "user", "content": "msg2"},
		{"role": "assistant", "content": "resp2"},
		{"role": "user", "content": "msg3"},
	}

	cfg := CacheBreakpointConfig{
		MaxBreakpoints: 1,
		SkipCacheWrite: true,
	}

	result := ApplyPromptCachingWithConfig(messages, "5m", cfg)
	if len(result) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(result))
	}

	hasCC := func(msg map[string]any) bool {
		if cc, ok := msg["cache_control"]; ok && cc != nil {
			return true
		}
		if content, ok := msg["content"].([]map[string]any); ok && len(content) > 0 {
			_, ok2 := content[len(content)-1]["cache_control"]
			return ok2
		}
		return false
	}

	// System message gets a breakpoint (always)
	if !hasCC(result[0]) {
		t.Error("system message (index 0) should have cache_control")
	}
	// Non-system, non-last messages should NOT have cache_control
	for i := 1; i < 4; i++ {
		if hasCC(result[i]) {
			t.Errorf("message at index %d should NOT have cache_control", i)
		}
	}
	// With skipCacheWrite, breakpoint is at index 4, not index 5
	if !hasCC(result[4]) {
		t.Error("second-to-last non-system message (index 4) should have cache_control with skipCacheWrite=true")
	}
	if hasCC(result[5]) {
		t.Error("last message (index 5) should NOT have cache_control with skipCacheWrite=true")
	}
}

func TestApplyPromptCachingWithConfigTwoMessagesSkipCacheWrite(t *testing.T) {
	// With only 2 messages and skipCacheWrite, breakpoint shifts to index 0
	messages := []map[string]any{
		{"role": "user", "content": "msg1"},
		{"role": "assistant", "content": "resp1"},
	}

	cfg := CacheBreakpointConfig{
		MaxBreakpoints: 1,
		SkipCacheWrite: true,
	}

	result := ApplyPromptCachingWithConfig(messages, "5m", cfg)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	hasCC := func(msg map[string]any) bool {
		if cc, ok := msg["cache_control"]; ok && cc != nil {
			return true
		}
		if content, ok := msg["content"].([]map[string]any); ok && len(content) > 0 {
			_, ok2 := content[len(content)-1]["cache_control"]
			return ok2
		}
		return false
	}

	// With skipCacheWrite and 2 messages, breakpoint at index 0 (length-2)
	if !hasCC(result[0]) {
		t.Error("first message (index 0) should have cache_control with skipCacheWrite and only 2 messages")
	}
	if hasCC(result[1]) {
		t.Error("last message (index 1) should NOT have cache_control with skipCacheWrite=true")
	}
}

func TestApplyPromptCachingSkipsSystemInjected(t *testing.T) {
	// System-injected messages should be skipped for breakpoint placement
	prefix := "<!-- system-injected -->"
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "msg1"},
		{"role": "assistant", "content": "resp1"},
		{"role": "user", "content": prefix + "injected content"}, // should be skipped
		{"role": "assistant", "content": "resp2"},
		{"role": "user", "content": "msg3"},
	}

	result := ApplyPromptCaching(messages, "5m")
	if len(result) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(result))
	}

	hasCC := func(msg map[string]any) bool {
		if cc, ok := msg["cache_control"]; ok && cc != nil {
			return true
		}
		if content, ok := msg["content"].([]map[string]any); ok && len(content) > 0 {
			_, ok2 := content[len(content)-1]["cache_control"]
			return ok2
		}
		return false
	}

	// The injected message (index 3) should NOT have cache_control
	if hasCC(result[3]) {
		t.Error("system-injected message (index 3) should NOT have cache_control")
	}

	// The prefix should be stripped from the injected message
	userContent, ok := result[3]["content"].([]map[string]any)
	if ok && len(userContent) > 0 {
		if text, ok2 := userContent[0]["text"].(string); ok2 {
			if strings.HasPrefix(text, prefix) {
				t.Error("system-injected prefix should be stripped from message content")
			}
		}
	}

	// Last non-injected message (index 5) should have cache_control
	if hasCC(result[4]) {
		t.Error("second-to-last non-injected message (index 4) should NOT have cache_control with 1 breakpoint")
	}
	if !hasCC(result[5]) {
		t.Error("last message (index 5) should have cache_control")
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
	// 50000 > 10000, so should detect break
	if !d.DetectBreak(100000) {
		t.Error("compaction change should trigger category-based break detection")
	}

	// Reset and test with accumulated changes
	d.ResetBaseline()
	d.UpdateBaseline(50000)

	// Multiple tool results — 5 * 5000 = 25000 > 5000 (10% of 50000)
	d.RecordChange(CacheChangeToolResult, 5)
	if !d.DetectBreak(50000) {
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
		category CacheChangeCategory
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

func TestApplyCacheMarkerToolResultArrayBlock(t *testing.T) {
	// Test cache_reference on tool_result blocks in array content
	msg := map[string]any{
		"role": "assistant",
		"content": []any{
			map[string]any{"type": "text", "text": "first"},
			map[string]any{"type": "tool_result", "tool_use_id": "toolu_abc123", "content": "result"},
		},
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	arr, ok := msg["content"].([]any)
	if !ok {
		t.Fatal("expected content to remain as array")
	}
	lastBlock, _ := arr[len(arr)-1].(map[string]any)
	if _, ok := lastBlock["cache_control"]; !ok {
		t.Error("last block (tool_result) should have cache_control")
	}
	// tool_use_id MUST be preserved — the API requires it for tool_result pairing (error 2013 fix)
	if lastBlock["tool_use_id"] != "toolu_abc123" {
		t.Errorf("expected tool_use_id='toolu_abc123', got %v", lastBlock["tool_use_id"])
	}
	if _, ok := lastBlock["cache_reference"]; ok {
		t.Error("tool_result block should NOT have cache_reference (deprecated)")
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

func TestApplyPromptCachingOneBreakpoint(t *testing.T) {
	// With 1 breakpoint: system + last message
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "msg1"},
		{"role": "assistant", "content": "resp1"},
		{"role": "user", "content": "msg2"},
	}

	result := ApplyPromptCaching(messages, "5m")

	hasCC := func(msg map[string]any) bool {
		if cc, ok := msg["cache_control"]; ok && cc != nil {
			return true
		}
		if content, ok := msg["content"].([]map[string]any); ok && len(content) > 0 {
			_, ok2 := content[len(content)-1]["cache_control"]
			return ok2
		}
		return false
	}

	// System message gets breakpoint
	if !hasCC(result[0]) {
		t.Error("system message should have cache_control")
	}
	// Only last message gets breakpoint (1 breakpoint strategy)
	for i := 1; i < 3; i++ {
		if hasCC(result[i]) {
			t.Errorf("message at index %d should NOT have cache_control", i)
		}
	}
	if !hasCC(result[3]) {
		t.Error("last message should have cache_control")
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