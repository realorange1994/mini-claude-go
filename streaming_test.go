package main

import (
	"testing"
)

func TestCollectHandlerText(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeText, Content: "Hello"})
	h.Handle(StreamChunk{Type: ChunkTypeText, Content: " "})
	h.Handle(StreamChunk{Type: ChunkTypeText, Content: "World"})

	if h.Text != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", h.Text)
	}
}

func TestCollectHandlerToolCall(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "tool-1", Name: "read_file"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"path": "test.txt"}`})

	if len(h.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(h.ToolCalls))
	}
	if h.ToolCalls[0].ID != "tool-1" {
		t.Errorf("expected ID 'tool-1', got %q", h.ToolCalls[0].ID)
	}
	if h.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected name 'read_file', got %q", h.ToolCalls[0].Name)
	}
	if h.ToolCalls[0].Arguments != `{"path": "test.txt"}` {
		t.Errorf("unexpected arguments: %q", h.ToolCalls[0].Arguments)
	}
}

func TestCollectHandlerMultipleToolCalls(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "tool-1", Name: "read_file"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"path": "a.txt"}`})
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "tool-2", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command": "ls"`})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `}`})

	if len(h.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(h.ToolCalls))
	}
	if h.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected first tool to be read_file, got %q", h.ToolCalls[0].Name)
	}
	if h.ToolCalls[1].Name != "exec" {
		t.Errorf("expected second tool to be exec, got %q", h.ToolCalls[1].Name)
	}
	if h.ToolCalls[1].Arguments != `{"command": "ls"}` {
		t.Errorf("expected complete arguments, got %q", h.ToolCalls[1].Arguments)
	}
}

func TestCollectHandlerThinking(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeThinking, Content: "Let me think..."})
	h.Handle(StreamChunk{Type: ChunkTypeThinking, Content: " about this."})

	if h.Thinking != "Let me think... about this." {
		t.Errorf("expected thinking content, got %q", h.Thinking)
	}
}

func TestCollectHandlerUsage(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeUsage, Usage: &Usage{InputTokens: 100, OutputTokens: 50}})

	if h.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if h.Usage.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", h.Usage.InputTokens)
	}
	if h.Usage.OutputTokens != 50 {
		t.Errorf("expected output tokens 50, got %d", h.Usage.OutputTokens)
	}
}

func TestCollectHandlerError(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeError, Content: "something went wrong"})

	if h.Err == nil {
		t.Fatal("expected error to be set")
	}
}

func TestCollectHandlerFullResponse(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeText, Content: "Hello"})

	if h.FullResponse() != "Hello" {
		t.Errorf("expected 'Hello', got %q", h.FullResponse())
	}
}

func TestCollectHandlerFullResponseFallbackToThinking(t *testing.T) {
	h := NewCollectHandler()

	// Only thinking, no text
	h.Handle(StreamChunk{Type: ChunkTypeThinking, Content: "Thinking..."})

	if h.FullResponse() != "Thinking..." {
		t.Errorf("expected thinking as fallback, got %q", h.FullResponse())
	}
}

func TestCollectHandlerAsParsedResponse(t *testing.T) {
	h := NewCollectHandler()

	h.Handle(StreamChunk{Type: ChunkTypeText, Content: "Response text"})
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "tool-1", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command": "ls"}`})

	toolCalls, textParts := h.AsParsedResponse()

	if len(textParts) != 1 {
		t.Fatalf("expected 1 text part, got %d", len(textParts))
	}
	if textParts[0] != "Response text" {
		t.Errorf("expected 'Response text', got %q", textParts[0])
	}

	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0]["id"] != "tool-1" {
		t.Errorf("expected id 'tool-1', got %v", toolCalls[0]["id"])
	}
	if toolCalls[0]["name"] != "exec" {
		t.Errorf("expected name 'exec', got %v", toolCalls[0]["name"])
	}
}

func TestCollectHandlerToolUseAsText(t *testing.T) {
	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		// New stricter logic: requires 2 of 3 structural markers (type, id, name)
		{"type + id (2 markers)", `{"type":"tool_use","id":"abc123"}`, true},
		{"type + name (2 markers)", `{"type":"tool_use","name":"read_file"}`, true},
		{"id + name (2 markers)", `{"id":"abc123","name":"read_file"}`, true},
		{"all 3 markers", `{"type":"tool_use","id":"abc","name":"exec"}`, true},
		{"only type (1 marker)", `{"type":"tool_use"}`, false},
		{"only id (1 marker)", `{"id":"abc123"}`, false},
		{"only name (1 marker)", `{"name":"read_file"}`, false},
		{"no markers", `{"path":"test.txt"}`, false},
		{"loose spacing", `{"type": "tool_use", "id": "abc"}`, true},
		{"discussing tool_use (only type)", `"The tool_use feature allows..."`, false},
		{"discussing id (only id)", `"The id field contains..."`, false},
		{"XML-like (no JSON structure)", `<tool_use><id>abc</id></tool_use>`, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewCollectHandler()
			h.Handle(StreamChunk{Type: ChunkTypeText, Content: tc.content})
			if h.toolUseAsText != tc.expected {
				t.Errorf("content=%q: expected toolUseAsText=%v, got %v", tc.content, tc.expected, h.toolUseAsText)
			}
		})
	}
}

func TestStreamBusSubscribe(t *testing.T) {
	bus := NewStreamBus()
	ch := bus.Subscribe("test")

	if ch == nil {
		t.Error("expected channel to be returned")
	}

	// Check that publishing works
	bus.Publish(StreamChunk{Type: ChunkTypeText, Content: "test"})

	select {
	case chunk := <-ch:
		if chunk.Content != "test" {
			t.Errorf("expected 'test', got %q", chunk.Content)
		}
	default:
		t.Error("expected to receive chunk")
	}
}

func TestStreamBusUnsubscribe(t *testing.T) {
	bus := NewStreamBus()
	ch := bus.Subscribe("test")
	bus.Unsubscribe("test")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestStreamBusPublishToMultiple(t *testing.T) {
	bus := NewStreamBus()
	ch1 := bus.Subscribe("sub1")
	ch2 := bus.Subscribe("sub2")

	bus.Publish(StreamChunk{Type: ChunkTypeText, Content: "hello"})

	// Both should receive
	if (<-ch1).Content != "hello" {
		t.Error("sub1 didn't receive")
	}
	if (<-ch2).Content != "hello" {
		t.Error("sub2 didn't receive")
	}
}

func TestStreamBusClose(t *testing.T) {
	bus := NewStreamBus()
	ch1 := bus.Subscribe("sub1")
	ch2 := bus.Subscribe("sub2")

	bus.Close()

	// Both channels should be closed
	_, ok1 := <-ch1
	_, ok2 := <-ch2
	if ok1 || ok2 {
		t.Error("expected both channels to be closed")
	}
}

func TestContextErr(t *testing.T) {
	testCases := []struct {
		err      string
		expected bool
	}{
		{"context canceled", true},
		{"context deadline exceeded", true},
		{"deadline exceeded", true},
		{"some other error", false},
		{"", false},
	}

	for _, tc := range testCases {
		result := contextErr(&testError{msg: tc.err})
		if result != tc.expected {
			t.Errorf("contextErr(%q) = %v, expected %v", tc.err, result, tc.expected)
		}
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }
