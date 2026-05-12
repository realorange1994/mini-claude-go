package main

import (
	"testing"
)

func TestApplyPromptCachingEmpty(t *testing.T) {
	result := ApplyPromptCaching(nil, "5m")
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestApplyPromptCachingShort(t *testing.T) {
	// Fewer than 4 messages: last message gets the single cache breakpoint
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "hello"},
	}

	result := ApplyPromptCaching(messages, "5m")
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// System message: no cache_control (only 1 breakpoint, placed at last message)
	if cc, ok := result[0]["cache_control"]; ok && cc != nil {
		t.Errorf("system message should NOT have cache_control with single-breakpoint strategy, got %v", cc)
	}
	sysContent, ok := result[0]["content"].([]map[string]any)
	if ok {
		if _, ok2 := sysContent[len(sysContent)-1]["cache_control"]; ok2 {
			t.Error("system message should NOT have cache_control with single-breakpoint strategy")
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
	// Many messages: only the last message gets the cache breakpoint
	// (single-breakpoint strategy matching upstream)
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

	// With single-breakpoint strategy, only the last message (index 5) should have cache_control
	for i := 0; i < 5; i++ {
		if hasCC(result[i]) {
			t.Errorf("message at index %d should NOT have cache_control with single-breakpoint strategy", i)
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
	// For cached tool_result blocks, tool_use_id should be replaced with cache_reference
	if _, ok := msg["tool_use_id"]; ok {
		t.Error("tool role message should NOT have tool_use_id after cache marking (replaced by cache_reference)")
	}
	if msg["cache_reference"] != "toolu_12345" {
		t.Errorf("expected cache_reference='toolu_12345', got %v", msg["cache_reference"])
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
	// No tool_use_id to convert, cache_reference should not be set
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
	if MaxCacheBreakpoints != 1 {
		t.Errorf("expected MaxCacheBreakpoints=1, got %d", MaxCacheBreakpoints)
	}
}

func TestApplyPromptCachingWithConfigSkipCacheWrite(t *testing.T) {
	// skipCacheWrite shifts breakpoint from last to second-to-last
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

	// With skipCacheWrite, breakpoint is at length-2 (index 4), not length-1 (index 5)
	for i := 0; i < 4; i++ {
		if hasCC(result[i]) {
			t.Errorf("message at index %d should NOT have cache_control", i)
		}
	}
	if !hasCC(result[4]) {
		t.Error("second-to-last message (index 4) should have cache_control with skipCacheWrite=true")
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
	// tool_result block should have cache_reference instead of tool_use_id
	if lastBlock["cache_reference"] != "toolu_abc123" {
		t.Errorf("expected cache_reference='toolu_abc123', got %v", lastBlock["cache_reference"])
	}
	if _, ok := lastBlock["tool_use_id"]; ok {
		t.Error("tool_result block should NOT have tool_use_id after cache marking (replaced by cache_reference)")
	}
}