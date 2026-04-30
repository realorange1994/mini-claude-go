package main

import (
	"testing"
	"time"
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

// ---------------------------------------------------------------------------
// FinishReason tests
// ---------------------------------------------------------------------------

func TestCollectHandlerFinishReasonDefault(t *testing.T) {
	h := NewCollectHandler()
	if h.FinishReason() != "" {
		t.Errorf("expected empty finish reason by default, got %q", h.FinishReason())
	}
}

func TestCollectHandlerFinishReasonSetAndGet(t *testing.T) {
	h := NewCollectHandler()
	h.SetFinishReason("end_turn")
	if h.FinishReason() != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", h.FinishReason())
	}
}

func TestCollectHandlerFinishReasonOverwrite(t *testing.T) {
	h := NewCollectHandler()
	h.SetFinishReason("tool_use")
	h.SetFinishReason("max_tokens")
	if h.FinishReason() != "max_tokens" {
		t.Errorf("expected 'max_tokens' after overwrite, got %q", h.FinishReason())
	}
}

// ---------------------------------------------------------------------------
// HasPartialToolCall tests
// ---------------------------------------------------------------------------

func TestCollectHandlerHasPartialToolCallEmpty(t *testing.T) {
	h := NewCollectHandler()
	if h.HasPartialToolCall() {
		t.Error("expected false when no tool calls")
	}
}

func TestCollectHandlerHasPartialToolCallWithArgs(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command":"ls"}`})
	if h.HasPartialToolCall() {
		t.Error("expected false when last tool has args")
	}
}

func TestCollectHandlerHasPartialToolCallNoArgs(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	if !h.HasPartialToolCall() {
		t.Error("expected true when last tool has no args")
	}
}

func TestCollectHandlerHasPartialToolCallMultipleLastEmpty(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "read_file"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"path":"a.txt"}`})
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t2", Name: "exec"})
	// t2 has no args
	if !h.HasPartialToolCall() {
		t.Error("expected true when last of multiple tools has no args")
	}
}

// ---------------------------------------------------------------------------
// ClearPartialToolCall tests
// ---------------------------------------------------------------------------

func TestCollectHandlerClearPartialToolCallBasic(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	if !h.HasPartialToolCall() {
		t.Fatal("expected partial before clear")
	}
	h.ClearPartialToolCall()
	if h.HasPartialToolCall() {
		t.Error("expected no partial after clear")
	}
	if len(h.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls after clear, got %d", len(h.ToolCalls))
	}
}

func TestCollectHandlerClearPartialToolCallWhenNone(t *testing.T) {
	h := NewCollectHandler()
	h.ClearPartialToolCall() // should not panic
	if len(h.ToolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(h.ToolCalls))
	}
}

func TestCollectHandlerClearPartialToolCallPreservesEarlier(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "read_file"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"path":"a.txt"}`})
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t2", Name: "exec"})
	// Only t2 is partial
	h.ClearPartialToolCall()
	if len(h.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call after clearing partial, got %d", len(h.ToolCalls))
	}
	if h.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected read_file preserved, got %q", h.ToolCalls[0].Name)
	}
}

// ---------------------------------------------------------------------------
// HasTruncatedToolArgs tests
// ---------------------------------------------------------------------------

func TestCollectHandlerHasTruncatedToolArgsValidJSON(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command":"ls"}`})
	if h.HasTruncatedToolArgs() {
		t.Error("expected false when tool args are valid JSON")
	}
}

func TestCollectHandlerHasTruncatedToolArgsInvalidJSON(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command":"l`})
	if !h.HasTruncatedToolArgs() {
		t.Error("expected true when tool args are invalid JSON")
	}
}

func TestCollectHandlerHasTruncatedToolArgsEmptyArgsIgnored(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	// Tool call with no args at all -- not truncated, just incomplete
	if h.HasTruncatedToolArgs() {
		t.Error("expected false when tool has no args (not truncated)")
	}
}

func TestCollectHandlerHasTruncatedToolArgsMultipleOneTruncated(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "read_file"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"path":"a.txt"}`})
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t2", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command":"l`})
	if !h.HasTruncatedToolArgs() {
		t.Error("expected true when one of multiple tools has truncated args")
	}
}

// ---------------------------------------------------------------------------
// ClearText tests
// ---------------------------------------------------------------------------

func TestCollectHandlerClearText(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeText, Content: "Hello World"})
	if h.Text != "Hello World" {
		t.Fatalf("expected text before clear, got %q", h.Text)
	}
	h.ClearText()
	if h.Text != "" {
		t.Errorf("expected empty text after clear, got %q", h.Text)
	}
}

// ---------------------------------------------------------------------------
// StreamResult tests
// ---------------------------------------------------------------------------

func TestStreamResultCompletedTrue(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeText, Content: "Done"})
	h.SetFinishReason("end_turn")

	result := StreamResultFrom(h, true)
	if !result.Completed {
		t.Error("expected completed=true")
	}
	if result.Text != "Done" {
		t.Errorf("expected text 'Done', got %q", result.Text)
	}
	if result.FinishReason != "end_turn" {
		t.Errorf("expected finish_reason 'end_turn', got %q", result.FinishReason)
	}
}

func TestStreamResultCompletedFalseHasPartial(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeToolCall, ID: "t1", Name: "exec"})
	h.Handle(StreamChunk{Type: ChunkTypeToolArgument, Content: `{"command":"l`})

	result := StreamResultFrom(h, false)
	if result.Completed {
		t.Error("expected completed=false")
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in result, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "exec" {
		t.Errorf("expected tool name 'exec', got %q", result.ToolCalls[0].Name)
	}
}

func TestStreamResultThinkingFallback(t *testing.T) {
	h := NewCollectHandler()
	h.Handle(StreamChunk{Type: ChunkTypeThinking, Content: "Thinking..."})

	result := StreamResultFrom(h, true)
	if result.Text != "Thinking..." {
		t.Errorf("expected thinking as text fallback, got %q", result.Text)
	}
}

// ---------------------------------------------------------------------------
// DeltasState tracking tests
// ---------------------------------------------------------------------------

func TestDeltasStateTrackTextOnly(t *testing.T) {
	sa := NewStreamAdapter(nil, nil)
	sa.trackDeltaState(ChunkTypeText, "")
	if sa.deltasState != DeltasStateTextOnly {
		t.Errorf("expected TextOnly after text delta, got %q", sa.deltasState)
	}
}

func TestDeltasStateTrackToolInFlight(t *testing.T) {
	sa := NewStreamAdapter(nil, nil)
	sa.trackDeltaState(ChunkTypeToolCall, "t1")
	if sa.deltasState != DeltasStateToolInFlight {
		t.Errorf("expected ToolInFlight after tool call, got %q", sa.deltasState)
	}
}

func TestDeltasStateStaysToolInFlight(t *testing.T) {
	sa := NewStreamAdapter(nil, nil)
	sa.trackDeltaState(ChunkTypeToolCall, "t1")
	sa.trackDeltaState(ChunkTypeToolArgument, "")
	if sa.deltasState != DeltasStateToolInFlight {
		t.Errorf("expected ToolInFlight to persist through args, got %q", sa.deltasState)
	}
}

func TestDeltasStateTextThenTool(t *testing.T) {
	sa := NewStreamAdapter(nil, nil)
	sa.trackDeltaState(ChunkTypeText, "")
	sa.trackDeltaState(ChunkTypeToolCall, "t1")
	if sa.deltasState != DeltasStateTextOnly {
		t.Errorf("expected TextOnly to persist after tool following text, got %q", sa.deltasState)
	}
}

func TestDeltasStateInitialStateNone(t *testing.T) {
	sa := NewStreamAdapter(nil, nil)
	if sa.DeltasState() != DeltasStateNone {
		t.Errorf("expected initial state to be None, got %q", sa.DeltasState())
	}
}

// ---------------------------------------------------------------------------
// StreamProgress tests
// ---------------------------------------------------------------------------

func TestStreamProgressTTFB(t *testing.T) {
	p := &StreamProgress{}
	p.StartTime = time.Now()

	// Before first byte, TTFB should be 0
	if p.TTFB() != 0 {
		t.Error("expected TTFB=0 before first byte")
	}

	time.Sleep(2 * time.Millisecond)
	p.RecordFirstByte()

	// After first byte, TTFB should be non-zero
	ttfb := p.TTFB()
	if ttfb == 0 {
		t.Errorf("expected TTFB>0 after first byte, got %v", ttfb)
	}
}

func TestStreamProgressTTFBOnlyOnce(t *testing.T) {
	p := &StreamProgress{}
	p.StartTime = time.Now()

	p.RecordFirstByte()
	first := p.TTFB()

	// Small delay then record again -- should not change
	time.Sleep(10 * time.Millisecond)
	p.RecordFirstByte()
	second := p.TTFB()

	if first != second {
		t.Errorf("TTFB should not change on subsequent RecordFirstByte calls: first=%v, second=%v", first, second)
	}
}

func TestStreamProgressThroughput(t *testing.T) {
	p := &StreamProgress{}
	p.StartTime = time.Now()
	p.RecordFirstByte()
	p.RecordTokens(100)

	time.Sleep(5 * time.Millisecond)
	tp := p.Throughput()
	if tp <= 0 {
		t.Errorf("expected throughput>0, got %f", tp)
	}
}

func TestStreamProgressThroughputZeroTokens(t *testing.T) {
	p := &StreamProgress{}
	p.StartTime = time.Now()
	p.RecordFirstByte()

	// No tokens recorded -> throughput = 0
	if p.Throughput() != 0 {
		t.Error("expected throughput=0 with no tokens")
	}
}

func TestStreamProgressThroughputNoFirstByte(t *testing.T) {
	p := &StreamProgress{}
	p.StartTime = time.Now()
	p.RecordTokens(100)

	// No first byte recorded -> throughput = 0
	if p.Throughput() != 0 {
		t.Error("expected throughput=0 with no first byte")
	}
}

func TestStreamProgressRecordTokens(t *testing.T) {
	p := &StreamProgress{}
	p.RecordTokens(50)
	p.RecordTokens(30)

	if p.TokensRecv != 80 {
		t.Errorf("expected 80 tokens, got %d", p.TokensRecv)
	}
}
