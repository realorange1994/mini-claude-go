package main

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hi", 1},
		{"hello world", 3},
		{stringsRepeat("a", 400), 100},
	}
	for _, tc := range tests {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", short(tc.input), got, tc.want)
		}
	}
}

func TestNeedsCompaction(t *testing.T) {
	// Create enough content to exceed the threshold
	bigContent := stringsRepeat("x", 300)
	msgs := []CompactionMessage{
		{Role: "user", Content: bigContent},
		{Role: "assistant", Content: bigContent},
	}
	cfg := DefaultCompactionConfig()
	cfg.MaxContextTokens = 100
	cfg.Threshold = 0.75

	if !NeedsCompaction(msgs, cfg) {
		t.Error("expected compaction needed for content exceeding threshold")
	}

	cfg.MaxContextTokens = 1000000
	if NeedsCompaction(msgs, cfg) {
		t.Error("expected no compaction needed for large context")
	}
}

func TestContextInfo(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "what is go?"},
		{Role: "assistant", Content: "Go is a programming language"},
	}
	info := ContextInfo(msgs, 200000)
	if info == "" {
		t.Error("expected non-empty context info")
	}
	// Should mention token count
	t.Logf("ContextInfo: %s", info)
}

func TestGroupMessagesByRound(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 2 {
		t.Errorf("expected 2 rounds, got %d", len(rounds))
	}
}

func TestFindSafeCompactionBoundary(t *testing.T) {
	rounds := []apiRound{
		{indices: []int{0, 1}, messages: []CompactionMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}},
		{indices: []int{2, 3}, messages: []CompactionMessage{{Role: "user", Content: "q2"}, {Role: "assistant", Content: "a2"}}},
		{indices: []int{4, 5}, messages: []CompactionMessage{{Role: "user", Content: "q3"}, {Role: "assistant", Content: "a3"}}},
		{indices: []int{6, 7}, messages: []CompactionMessage{{Role: "user", Content: "q4"}, {Role: "assistant", Content: "a4"}}},
	}

	// Keep last 2 rounds, boundary should be at index 2 (third round)
	boundary := findSafeCompactionBoundary(rounds, 2)
	if boundary != 2 {
		t.Errorf("expected boundary at 2, got %d", boundary)
	}
}

func TestSmartCompact(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "original question", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "assistant", Content: "long answer 1", Timestamp: "2024-01-01T00:00:01Z"},
		{Role: "user", Content: "followup", Timestamp: "2024-01-01T00:00:02Z"},
		{Role: "assistant", Content: "long answer 2", Timestamp: "2024-01-01T00:00:03Z"},
		{Role: "user", Content: "latest q", Timestamp: "2024-01-01T00:00:04Z"},
		{Role: "assistant", Content: "latest a", Timestamp: "2024-01-01T00:00:05Z"},
	}

	result := SmartCompact(msgs, 1, 1)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CollapsedTurns < 1 {
		t.Errorf("expected at least 1 collapsed turn, got %d", result.CollapsedTurns)
	}
	if len(result.Messages) == 0 {
		t.Error("expected some messages after compaction")
	}
}

func TestMicroCompact(t *testing.T) {
	// Simulate a conversation with multiple rounds
	msgs := []CompactionMessage{
		{Role: "user", Content: "run a command", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "assistant", Content: "ok", Timestamp: "2024-01-01T00:00:01Z"},
		{Role: "user", Content: "[tool result]", ToolName: "exec", Timestamp: "2024-01-01T00:00:02Z"},
		{Role: "assistant", Content: "done", Timestamp: "2024-01-01T00:00:03Z"},
		{Role: "user", Content: "another command", Timestamp: "2024-01-01T00:00:04Z"},
		{Role: "assistant", Content: "ok2", Timestamp: "2024-01-01T00:00:05Z"},
		{Role: "user", Content: "[tool result 2]", ToolName: "exec", Timestamp: "2024-01-01T00:00:06Z"},
		{Role: "assistant", Content: "done2", Timestamp: "2024-01-01T00:00:07Z"},
		{Role: "user", Content: "final question", Timestamp: "2024-01-01T00:00:08Z"},
	}

	result := MicroCompact(msgs, DefaultMicroCompactConfig())
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
	// Middle tool-heavy turns should be compacted, first user + last messages preserved
	t.Logf("MicroCompact: %d -> %d messages", len(msgs), len(result))
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, n*len(s))
	for i := 0; i < n; i++ {
		copy(out[i*len(s):], s)
	}
	return string(out)
}

func short(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

func TestCheckReactiveCompact(t *testing.T) {
	tests := []struct {
		name            string
		currentTokens   int
		previousTokens  int
		threshold       int
		expectTriggered bool
		expectDelta     int
	}{
		{
			name:            "no spike, same tokens",
			currentTokens:   50000,
			previousTokens:  50000,
			threshold:       5000,
			expectTriggered: false,
		},
		{
			name:            "no spike, small increase",
			currentTokens:   53000,
			previousTokens:  50000,
			threshold:       5000,
			expectTriggered: false,
		},
		{
			name:            "spike detected, just over threshold",
			currentTokens:   55001,
			previousTokens:  50000,
			threshold:       5000,
			expectTriggered: true,
			expectDelta:     5001,
		},
		{
			name:            "spike detected, large jump",
			currentTokens:   70000,
			previousTokens:  50000,
			threshold:       5000,
			expectTriggered: true,
			expectDelta:     20000,
		},
		{
			name:            "tokens decreased, no trigger",
			currentTokens:   40000,
			previousTokens:  50000,
			threshold:       5000,
			expectTriggered: false,
		},
		{
			name:            "custom threshold",
			currentTokens:   12000,
			previousTokens:  10000,
			threshold:       1000,
			expectTriggered: true,
			expectDelta:     2000,
		},
		{
			name:            "default threshold when zero",
			currentTokens:   10000,
			previousTokens:  4000,
			threshold:       0, // should default to 5000
			expectTriggered: true,
			expectDelta:     6000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := CheckReactiveCompact(tc.currentTokens, tc.previousTokens, tc.threshold)
			if tc.expectTriggered {
				if result == nil {
					t.Errorf("expected reactive compact to trigger, got nil")
					return
				}
				if result.TokenDelta != tc.expectDelta {
					t.Errorf("expected delta %d, got %d", tc.expectDelta, result.TokenDelta)
				}
				if result.PreTokens != tc.currentTokens {
					t.Errorf("expected pre-tokens %d, got %d", tc.currentTokens, result.PreTokens)
				}
				if result.PreviousTokens != tc.previousTokens {
					t.Errorf("expected previous-tokens %d, got %d", tc.previousTokens, result.PreviousTokens)
				}
			} else {
				if result != nil {
					t.Errorf("expected no reactive compact trigger, got %+v", result)
				}
			}
		})
	}
}

func TestPartialCompactUpTo(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Build a conversation: 3 user messages + 3 assistant tool calls + 3 tool results
	// Total: 9 entries
	ctx.AddUserMessage("First question about the project structure")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "list_dir", "input": map[string]any{"path": "."}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: stringsRepeat("file listing output ", 100)}},
		}},
	})
	ctx.AddUserMessage("Now read the main.go file")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_2", "name": "read_file", "input": map[string]any{"path": "main.go"}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_2", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: stringsRepeat("package main\nfunc main() ", 50)}},
		}},
	})
	ctx.AddUserMessage("Latest question: how do I add a new feature?")

	totalEntries := ctx.Len()
	if totalEntries < 7 {
		t.Fatalf("expected at least 7 entries, got %d", totalEntries)
	}

	// Partial compact "up_to" at pivot 4 (summarize first 4 entries, keep rest)
	result, err := ctx.PartialCompact(PartialCompactUpTo, 4, 3)
	if err != nil {
		t.Fatalf("partial compact failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Direction != PartialCompactUpTo {
		t.Errorf("expected direction up_to, got %s", result.Direction)
	}
	if result.MessagesSummarized <= 0 {
		t.Errorf("expected some messages summarized, got %d", result.MessagesSummarized)
	}
	if result.MessagesKept <= 0 {
		t.Errorf("expected some messages kept, got %d", result.MessagesKept)
	}
	if result.TokensSaved < 0 {
		t.Errorf("expected non-negative tokens saved, got %d", result.TokensSaved)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestPartialCompactFrom(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Build conversation with early important context and later tool-heavy output
	ctx.AddUserMessage("Important goal: Implement a new feature for the project")
	ctx.AddAssistantText("I understand. Let me first examine the project structure.")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "list_dir", "input": map[string]any{"path": "."}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: stringsRepeat("lots of directory output ", 50)}},
		}},
	})
	// More tool output in the middle
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_2", "name": "read_file", "input": map[string]any{"path": "config.go"}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_2", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: stringsRepeat("config file content ", 80)}},
		}},
	})
	// Keep these recent messages
	ctx.AddUserMessage("OK, I think I understand now. Let's proceed with the implementation.")

	totalEntries := ctx.Len()
	if totalEntries < 7 {
		t.Fatalf("expected at least 7 entries, got %d", totalEntries)
	}

	// Partial compact "from" at pivot 2 (keep first 2 entries + last 3, summarize the middle)
	result, err := ctx.PartialCompact(PartialCompactFrom, 2, 3)
	if err != nil {
		t.Fatalf("partial compact failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Direction != PartialCompactFrom {
		t.Errorf("expected direction from, got %s", result.Direction)
	}
	if result.MessagesSummarized <= 0 {
		t.Errorf("expected some messages summarized, got %d", result.MessagesSummarized)
	}
	if result.MessagesKept <= 0 {
		t.Errorf("expected some messages kept, got %d", result.MessagesKept)
	}
}

func TestPartialCompactInvalidDirection(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("hello")

	_, err := ctx.PartialCompact(PartialCompactDirection("invalid"), 0, 3)
	if err == nil {
		t.Error("expected error for invalid direction")
	}
}

func TestPartialCompactEmptyEntries(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	_, err := ctx.PartialCompact(PartialCompactUpTo, 0, 3)
	if err == nil {
		t.Error("expected error for empty entries")
	}
}

func TestEntriesToSummaryText(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello, this is a test question")
	ctx.AddAssistantText("Sure, let me help with that")
	ctx.AddUserMessage("Follow up question")

	entries := ctx.Entries()
	summary := entriesToSummaryText(entries)

	if summary == "" {
		t.Error("expected non-empty summary text")
	}
	if !strings.Contains(summary, "Hello, this is a test question") {
		t.Error("expected summary to contain first user message")
	}
	if !strings.Contains(summary, "Sure, let me help with that") {
		t.Error("expected summary to contain assistant response")
	}
}

func TestAdjustPivotForToolPairs(t *testing.T) {
	// Test that pivot adjustment doesn't go out of bounds
	entries := []conversationEntry{
		{role: "user", content: TextContent("q1")},
		{role: "assistant", content: TextContent("a1")},
	}

	// Boundary cases
	got := adjustPivotForToolPairs(entries, 0, PartialCompactUpTo)
	if got != 0 {
		t.Errorf("expected pivot 0, got %d", got)
	}

	got = adjustPivotForToolPairs(entries, 2, PartialCompactUpTo)
	if got != 2 {
		t.Errorf("expected pivot 2, got %d", got)
	}
}

func TestEstimateEntriesTokens(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("hello world")
	ctx.AddAssistantText("hi there")

	entries := ctx.Entries()
	tokens := ctx.estimateEntriesTokens(entries)

	if tokens <= 0 {
		t.Errorf("expected positive token count, got %d", tokens)
	}
}
