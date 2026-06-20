package main

import (
	"testing"
)

func TestTransformMessages_Anthropic(t *testing.T) {
	config := NewTransformConfig(ProviderAnthropic)

	messages := []TransformMessage{
		{Role: "user", Content: []TransformContent{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []TransformContent{
			{Type: "text", Text: ""},
			{Type: "tool_use", ID: "call-1"},
		}},
	}

	result := TransformMessages(config, messages)
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}

func TestTransformMessages_FilterEmpty(t *testing.T) {
	config := NewTransformConfig(ProviderAnthropic)

	messages := []TransformMessage{
		{Role: "user", Content: []TransformContent{{Type: "text", Text: ""}}},
		{Role: "assistant", Content: []TransformContent{{Type: "text", Text: "hello"}}},
	}

	result := TransformMessages(config, messages)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestTransformMessages_ScrubToolIDs(t *testing.T) {
	config := NewTransformConfig(ProviderAnthropic)

	messages := []TransformMessage{
		{Role: "assistant", Content: []TransformContent{
			{Type: "tool_use", ID: "call-1!@#"},
		}},
	}

	result := TransformMessages(config, messages)
	if len(result) > 0 && len(result[0].Content) > 0 {
		if result[0].Content[0].ID != "call-1___" {
			t.Errorf("expected scrubbed ID, got %s", result[0].Content[0].ID)
		}
	}
}

func TestTransformMessages_MistralID(t *testing.T) {
	config := NewTransformConfig(ProviderMistral)

	messages := []TransformMessage{
		{Role: "assistant", Content: []TransformContent{
			{Type: "tool_use", ID: "call-123456789"},
		}},
	}

	result := TransformMessages(config, messages)
	if len(result) > 0 && len(result[0].Content) > 0 {
		id := result[0].Content[0].ID
		if len(id) != 9 {
			t.Errorf("expected 9-char ID, got %d", len(id))
		}
	}
}

func TestTransformMessages_ReorderTools(t *testing.T) {
	config := NewTransformConfig(ProviderAnthropic)

	messages := []TransformMessage{
		{Role: "assistant", Content: []TransformContent{
			{Type: "tool_use", ID: "call-1"},
			{Type: "text", Text: "result"},
		}},
	}

	result := TransformMessages(config, messages)
	if len(result) > 0 && len(result[0].Content) > 1 {
		// Text should come before tool_use after reorder
		if result[0].Content[0].Type != "text" {
			t.Error("expected text before tool_use")
		}
	}
}

func TestSanitizeToolCallID_Anthropic(t *testing.T) {
	result := SanitizeToolCallID(ProviderAnthropic, "call-1!@#")
	if result != "call-1___" {
		t.Errorf("expected 'call-1___', got %q", result)
	}
}

func TestSanitizeToolCallID_Mistral(t *testing.T) {
	result := SanitizeToolCallID(ProviderMistral, "call-123456789")
	if len(result) != 9 {
		t.Errorf("expected 9-char, got %d", len(result))
	}
}

func TestSanitizeToolCallID_Default(t *testing.T) {
	result := SanitizeToolCallID(ProviderOpenAI, "call-1!@#")
	if result != "call-1!@#" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestScrubMistralID(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"abc", 9},
		{"abcdefghij", 9},
		{"a!@#b", 9},
	}

	for _, tt := range tests {
		result := scrubMistralID(tt.input)
		if len(result) != tt.expected {
			t.Errorf("scrubMistralID(%q) len = %d, want %d", tt.input, len(result), tt.expected)
		}
	}
}
