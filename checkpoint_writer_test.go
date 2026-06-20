package main

import (
	"testing"
	"time"
)

func TestCheckpointWriter_Submit(t *testing.T) {
	dir := t.TempDir()
	w := NewCheckpointWriter(dir)

	req := CheckpointRequest{
		SessionID: "test-session",
		Messages: []CheckpointMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		ProjectDir: dir,
		Timestamp:  time.Now(),
	}

	w.Submit(req)
	time.Sleep(100 * time.Millisecond)

	if w.GetLastCheckpointID() == "" {
		t.Error("expected checkpoint to be written")
	}
}

func TestCheckpointWriter_IsRunning(t *testing.T) {
	dir := t.TempDir()
	w := NewCheckpointWriter(dir)

	if w.IsRunning() {
		t.Error("expected writer to not be running initially")
	}
}

func TestComputeBoundary_SmallConversation(t *testing.T) {
	messages := []CheckpointMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	tail := computeBoundary(messages, 1000)
	if len(tail) != 2 {
		t.Errorf("expected 2 messages, got %d", len(tail))
	}
}

func TestComputeBoundary_LargeConversation(t *testing.T) {
	// Create messages that exceed budget
	var messages []CheckpointMessage
	for i := 0; i < 100; i++ {
		messages = append(messages, CheckpointMessage{
			Role:    "user",
			Content: "This is a test message with enough content to estimate tokens properly.",
		})
	}

	tail := computeBoundary(messages, 500)
	if len(tail) >= 100 {
		t.Error("expected tail to be smaller than full conversation")
	}
	if len(tail) == 0 {
		t.Error("expected non-empty tail")
	}
}

func TestBuildCheckpointContent(t *testing.T) {
	tail := []CheckpointMessage{
		{Role: "user", Content: "test message"},
	}

	content := buildCheckpointContent("cp-test", tail, "/tmp")
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestEstimateTokensCW(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},
		{"hello", 2},
	}

	for _, tt := range tests {
		result := estimateTokensCW(tt.input)
		if result != tt.expected {
			t.Errorf("estimateTokensCW(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestCountTokens(t *testing.T) {
	messages := []CheckpointMessage{
		{Role: "user", Content: "hello"},      // 5 chars -> 2 tokens
		{Role: "assistant", Content: "hi"},    // 2 chars -> 1 token
		{Role: "user", Content: "test", Tokens: 10}, // explicit tokens
	}

	total := countTokens(messages)
	if total != 13 { // 2 + 1 + 10
		t.Errorf("expected 13 tokens, got %d", total)
	}
}

func TestTruncateStringCW_Small(t *testing.T) {
	result := truncateStringCW("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateStringCW_Large(t *testing.T) {
	result := truncateStringCW("hello world this is a test", 10)
	if result != "hello worl..." {
		t.Errorf("expected 'hello worl...', got %q", result)
	}
}
