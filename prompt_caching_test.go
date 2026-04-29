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
	// Fewer than 4 messages: all get cache markers
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
		{"role": "user", "content": "hello"},
	}

	result := ApplyPromptCaching(messages, "5m")
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// System message: string content gets converted to array with cache_control on last block
	sysContent, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("system message content should be array, got %T", result[0]["content"])
	}
	if _, ok := sysContent[len(sysContent)-1]["cache_control"]; !ok {
		t.Error("system message last content block should have cache_control")
	}

	// User message: same
	userContent, ok := result[1]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("user message content should be array, got %T", result[1]["content"])
	}
	if _, ok := userContent[len(userContent)-1]["cache_control"]; !ok {
		t.Error("user message last content block should have cache_control")
	}
}

func TestApplyPromptCachingLong(t *testing.T) {
	// 5+ messages: system + last 3 non-system get markers, middle ones don't
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

	// System (index 0) should have cache_control in content
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

	if !hasCC(result[0]) {
		t.Error("system message should have cache_control")
	}
	if hasCC(result[1]) {
		t.Error("early user message should NOT have cache_control")
	}
	if hasCC(result[2]) {
		t.Error("early assistant message should NOT have cache_control")
	}
	for i := 3; i <= 5; i++ {
		if !hasCC(result[i]) {
			t.Errorf("message at index %d should have cache_control", i)
		}
	}
}

func TestApplyPromptCachingTTL(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "system prompt"},
	}

	// Default TTL (5m)
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
		"role": "tool",
		"content": "result text",
	}
	marker := map[string]any{"type": "ephemeral"}
	applyCacheMarker(msg, marker)

	if _, ok := msg["cache_control"]; !ok {
		t.Error("tool role message should have cache_control at message level")
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