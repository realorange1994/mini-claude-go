package main

import (
	"fmt"
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
			{messages: []CompactionMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}},
			{messages: []CompactionMessage{{Role: "user", Content: "q2"}, {Role: "assistant", Content: "a2"}}},
			{messages: []CompactionMessage{{Role: "user", Content: "q3"}, {Role: "assistant", Content: "a3"}}},
			{messages: []CompactionMessage{{Role: "user", Content: "q4"}, {Role: "assistant", Content: "a4"}}},
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
	result, err := ctx.PartialCompact(PartialCompactUpTo, 4, "", 3, nil)
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
	result, err := ctx.PartialCompact(PartialCompactFrom, 2, "", 3, nil)
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

	_, err := ctx.PartialCompact(PartialCompactDirection("invalid"), 0, "", 3, nil)
	if err == nil {
		t.Error("expected error for invalid direction")
	}
}

func TestPartialCompactEmptyEntries(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	_, err := ctx.PartialCompact(PartialCompactUpTo, 0, "", 3, nil)
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

// ─── groupMessagesByRound boundary tests ────────────────────────────────────
// Ported from upstream grouping.test.ts

func TestGroupByRoundSingleUser(t *testing.T) {
	msgs := []CompactionMessage{{Role: "user", Content: "hello"}}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(rounds))
	}
	if len(rounds[0].messages) != 1 {
		t.Errorf("expected 1 message in round, got %d", len(rounds[0].messages))
	}
}

func TestGroupByRoundAllUserMessages(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "user", Content: "q2"},
	}
	rounds := groupMessagesByRound(msgs)
	// In the Go implementation, each user message starts a new round,
	// so consecutive users produce separate rounds
	if len(rounds) != 2 {
		t.Fatalf("expected 2 rounds for consecutive user messages, got %d", len(rounds))
	}
}

func TestGroupByRoundSystemMessage(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "assistant", Content: "response"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) < 1 {
		t.Fatalf("expected at least 1 round, got %d", len(rounds))
	}
}

func TestGroupByRoundAlternating(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "q3"},
		{Role: "assistant", Content: "a3"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 3 {
		t.Errorf("expected 3 rounds for 3 user+assistant pairs, got %d", len(rounds))
	}
}

func TestGroupByRoundPreservesOrder(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "second"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(rounds))
	}
	if rounds[0].messages[0].Content != "first" {
		t.Errorf("expected first message 'first', got %q", rounds[0].messages[0].Content)
	}
	if rounds[0].messages[1].Content != "second" {
		t.Errorf("expected second message 'second', got %q", rounds[0].messages[1].Content)
	}
}

func TestGroupByRoundToolCallDetection(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: `{"type": "tool_result", "tool_use_id": "abc"}`},
		{Role: "assistant", Content: `{"type": "tool_use", "id": "abc"}`},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 1 {
		t.Fatalf("expected 1 round, got %d", len(rounds))
	}
	if !rounds[0].isToolCall {
		t.Error("expected tool call detection in round")
	}
}

// ─── DetectContentType tests ───────────────────────────────────────────────

func TestDetectContentTypeJSON(t *testing.T) {
	if DetectContentType(`{"key": "value"}`) != "json" {
		t.Error("should detect JSON content")
	}
	if DetectContentType(`[1, 2, 3]`) != "json" {
		t.Error("should detect JSON array content")
	}
}

func TestDetectContentTypeCode(t *testing.T) {
	codeExamples := []string{
		"func main() {}",
		"var x int = 0",
		"const PI = 3.14",
		"type Foo struct{}",
		"class Bar {}",
		"def foo(): pass",
		"import os",
		"package main",
	}
	for _, code := range codeExamples {
		if got := DetectContentType(code); got != "code" {
			t.Errorf("DetectContentType(%q) = %q, want 'code'", code, got)
		}
	}
}

func TestDetectContentTypeNatural(t *testing.T) {
	if DetectContentType("hello world") != "natural" {
		t.Error("should detect natural language")
	}
	if DetectContentType("This is a regular sentence.") != "natural" {
		t.Error("should detect natural language")
	}
}

func TestDetectContentTypeEmpty(t *testing.T) {
	result := DetectContentType("")
	if result != "natural" && result != "code" && result != "json" {
		t.Errorf("expected valid content type for empty, got %q", result)
	}
}

// ─── EstimateContentTokens tests ─────────────────────────────────────────────

func TestEstimateContentTokensCode(t *testing.T) {
	text := stringsRepeat("func test() { return true }\n", 10)
	tokens := EstimateContentTokens(text, "code")
	if tokens <= 0 {
		t.Error("expected positive token count for code")
	}
}

func TestEstimateContentTokensJSON(t *testing.T) {
	text := stringsRepeat(`{"key": "value"}`, 10)
	tokens := EstimateContentTokens(text, "json")
	if tokens <= 0 {
		t.Error("expected positive token count for JSON")
	}
}

func TestEstimateContentTokensToolUse(t *testing.T) {
	text := `{"type": "tool_use", "id": "abc", "name": "Read"}`
	tokens := EstimateContentTokens(text, "tool_use")
	if tokens <= 0 {
		t.Error("expected positive token count for tool_use")
	}
}

func TestEstimateContentTokensToolResult(t *testing.T) {
	text := `{"type": "tool_result", "tool_use_id": "abc", "content": "ok"}`
	tokens := EstimateContentTokens(text, "tool_result")
	if tokens <= 0 {
		t.Error("expected positive token count for tool_result")
	}
}

func TestEstimateContentTokensEmpty(t *testing.T) {
	if EstimateContentTokens("", "code") != 0 {
		t.Error("expected 0 tokens for empty content")
	}
}

func TestEstimateContentTokensDefault(t *testing.T) {
	text := "hello world this is natural language"
	tokens := EstimateContentTokens(text, "default")
	expected := len(text) / 4
	if tokens < expected-1 || tokens > expected+1 {
		t.Errorf("expected ~%d tokens for natural language, got %d", expected, tokens)
	}
}

// ─── Compact boundary conditions ────────────────────────────────────────────
// Ported from upstream compact test patterns

func TestCompactSingleMessage(t *testing.T) {
	msgs := []CompactionMessage{{Role: "user", Content: "hello"}}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 1
	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OmittedCount != 0 {
		t.Errorf("expected 0 omitted for single message, got %d", result.OmittedCount)
	}
}

func TestCompactSystemMessagePreserved(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 2
	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	systemFound := false
	for _, m := range result.Messages {
		if m.Role == "system" && strings.Contains(m.Content, "helpful assistant") {
			systemFound = true
			break
		}
	}
	if !systemFound {
		t.Error("system message should be preserved after compaction")
	}
}

func TestCompactKeepsRecentRounds(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "oldest question"},
		{Role: "assistant", Content: "oldest answer"},
		{Role: "user", Content: "middle question"},
		{Role: "assistant", Content: "middle answer"},
		{Role: "user", Content: "latest question"},
		{Role: "assistant", Content: "latest answer"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 2
	cfg.OmissionMarker = OmissionMarker
	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	keptContent := ""
	for _, m := range result.Messages {
		keptContent += m.Content
	}
	if !strings.Contains(keptContent, "latest") {
		t.Error("latest round should be kept")
	}
}

func TestCompactOmissionMarkerInsertedUpstream(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "old1"},
		{Role: "assistant", Content: "old1a"},
		{Role: "user", Content: "old2"},
		{Role: "assistant", Content: "old2a"},
		{Role: "user", Content: "old3"},
		{Role: "assistant", Content: "old3a"},
		{Role: "user", Content: "old4"},
		{Role: "assistant", Content: "old4a"},
		{Role: "user", Content: "latest"},
		{Role: "assistant", Content: "latest answer"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 2
	cfg.OmissionMarker = OmissionMarker
	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OmittedCount > 0 {
		found := false
		for _, m := range result.Messages {
			if m.Role == "system" && strings.Contains(m.Content, "omitted") {
				found = true
				break
			}
		}
		if !found {
			t.Error("omission marker should be present when messages are omitted")
		}
	}
}

// ─── NeedsCompaction boundary conditions ────────────────────────────────────

func TestNeedsCompactionZeroMaxTokens(t *testing.T) {
	msgs := []CompactionMessage{{Role: "user", Content: "hello"}}
	cfg := DefaultCompactionConfig()
	cfg.MaxContextTokens = 0
	if NeedsCompaction(msgs, cfg) {
		t.Error("should not compact when MaxContextTokens is 0")
	}
}

func TestNeedsCompactionAtThreshold(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: stringsRepeat("a", 300)},
		{Role: "assistant", Content: stringsRepeat("b", 300)},
	}
	cfg := DefaultCompactionConfig()
	cfg.MaxContextTokens = 150
	cfg.Threshold = 1.0
	if !NeedsCompaction(msgs, cfg) {
		t.Error("should need compaction at threshold")
	}
}

func TestNeedsCompactionBelowThreshold(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	cfg := DefaultCompactionConfig()
	cfg.MaxContextTokens = 100000
	cfg.Threshold = 0.75
	if NeedsCompaction(msgs, cfg) {
		t.Error("should not need compaction below threshold")
	}
}

// ─── totalTokens / messageTokens invariants ─────────────────────────────────

func TestMessageTokensNonNegative(t *testing.T) {
	msg := CompactionMessage{Role: "user", Content: ""}
	if messageTokens(msg) < 0 {
		t.Error("messageTokens should never be negative")
	}
}

func TestTotalTokensAdditive(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	total := totalTokens(msgs)
	indiv1 := messageTokens(msgs[0])
	indiv2 := messageTokens(msgs[1])
	if total != indiv1+indiv2 {
		t.Errorf("totalTokens(%d) != sum of individual(%d + %d = %d)", total, indiv1, indiv2, indiv1+indiv2)
	}
}

func TestTotalTokensEmptyMessages(t *testing.T) {
	if totalTokens([]CompactionMessage{}) != 0 {
		t.Error("totalTokens of empty slice should be 0")
	}
}

func TestRoundTokensNonNegative(t *testing.T) {
	round := apiRound{
		messages: []CompactionMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		},
	}
	if roundTokens(round) < 0 {
		t.Error("roundTokens should never be negative")
	}
}

// ─── ContextInfo tests ──────────────────────────────────────────────────────

func TestContextInfoNonEmpty(t *testing.T) {
	msgs := []CompactionMessage{{Role: "user", Content: "hello"}}
	info := ContextInfo(msgs, 200000)
	if info == "" {
		t.Error("ContextInfo should not be empty")
	}
	if !strings.Contains(info, "Context:") {
		t.Errorf("ContextInfo should contain 'Context:', got %q", info)
	}
	if !strings.Contains(info, "tokens") {
		t.Errorf("ContextInfo should contain 'tokens', got %q", info)
	}
}

func TestContextInfoZeroMaxTokens(t *testing.T) {
	msgs := []CompactionMessage{{Role: "user", Content: "hello"}}
	info := ContextInfo(msgs, 0)
	if info == "" {
		t.Error("ContextInfo should not be empty for zero max tokens")
	}
}

func TestContextInfoManyMessages(t *testing.T) {
	var msgs []CompactionMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, CompactionMessage{Role: "user", Content: "message"})
	}
	info := ContextInfo(msgs, 200000)
	if !strings.Contains(info, "100 messages") {
		t.Errorf("ContextInfo should report 100 messages, got %q", info)
	}
}

// ─── Progressive Micro-Compression Tests ────────────────────────────────────

func TestSoftTrimEntriesTrimsLargeOutput(t *testing.T) {
	ctx := &ConversationContext{
		entries:             make([]conversationEntry, 0),
		toolResultReplacements: make(map[string]string),
		clearedToolResults:     make(map[string]bool),
	}

	// Add multiple tool_use/tool_result pairs to exceed keepRecent=5
	largeText := stringsRepeat("x", 8000) // 8K chars, well above SOFT_TRIM_THRESHOLD
	for i := 0; i < 7; i++ {
		toolID := fmt.Sprintf("tool-%d", i)
		ctx.entries = append(ctx.entries, conversationEntry{
			role: "assistant",
			content: ToolUseContent{
				{OfToolUse: &anthropic.ToolUseBlockParam{ID: toolID, Name: "read_file"}},
			},
		})
		ctx.entries = append(ctx.entries, conversationEntry{
			role: "user",
			content: ToolResultContent{
				{ToolUseID: toolID, Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: largeText}},
				}},
			},
		})
	}

	trimmed := ctx.SoftTrimEntries(0) // keepRecent=0 -> defaults to 5
	if trimmed != 2 { // 7 results - 5 recent = 2 eligible
		t.Errorf("expected 2 trimmed, got %d", trimmed)
	}

	// Verify the oldest entries (tool-0, tool-1) were trimmed
	// The recent entries (tool-2 through tool-6) should be unchanged
	oldestResult := ctx.entries[1] // tool-0's result
	if results, ok := oldestResult.content.(ToolResultContent); ok {
		for _, r := range results {
			for _, cb := range r.Content {
				if cb.OfText != nil {
					if len(cb.OfText.Text) >= 8000 {
						t.Error("oldest text should have been trimmed")
					}
					if !strings.Contains(cb.OfText.Text, "[... trimmed") {
						t.Error("trimmed text should contain marker")
					}
				}
			}
		}
	}

	// Verify the newest entry (tool-6) was NOT trimmed
	newestResult := ctx.entries[len(ctx.entries)-1] // tool-6's result
	if results, ok := newestResult.content.(ToolResultContent); ok {
		for _, r := range results {
			for _, cb := range r.Content {
				if cb.OfText != nil {
					if len(cb.OfText.Text) < 8000 {
						t.Error("newest text should NOT have been trimmed")
					}
					if strings.Contains(cb.OfText.Text, "[... trimmed") {
						t.Error("newest text should not contain trim marker")
					}
				}
			}
		}
	}
}

func TestSoftTrimEntriesSkipsSmallOutput(t *testing.T) {
	ctx := &ConversationContext{
		entries:             make([]conversationEntry, 0),
		toolResultReplacements: make(map[string]string),
		clearedToolResults:     make(map[string]bool),
	}

	// Add a tool_use entry
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "assistant",
		content: ToolUseContent{
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "tool-1", Name: "read_file"}},
		},
	})

	// Add a tool_result entry with small output (below threshold)
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "user",
		content: ToolResultContent{
			{ToolUseID: "tool-1", Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: "small output"}},
			}},
		},
	})

	trimmed := ctx.SoftTrimEntries(0)
	if trimmed != 0 {
		t.Errorf("expected 0 trimmed (below threshold), got %d", trimmed)
	}
}

func TestSoftTrimEntriesSkipsRecentEntries(t *testing.T) {
	ctx := &ConversationContext{
		entries:             make([]conversationEntry, 0),
		toolResultReplacements: make(map[string]string),
		clearedToolResults:     make(map[string]bool),
	}

	// Add a tool_use entry
	largeText := stringsRepeat("x", 8000)
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "assistant",
		content: ToolUseContent{
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "tool-1", Name: "read_file"}},
		},
	})

	// Add a tool_result entry
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "user",
		content: ToolResultContent{
			{ToolUseID: "tool-1", Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: largeText}},
			}},
		},
	})

	// keepRecent=5 means this result is protected
	trimmed := ctx.SoftTrimEntries(5)
	if trimmed != 0 {
		t.Errorf("expected 0 trimmed (protected by keepRecent), got %d", trimmed)
	}
}

func TestSoftTrimEntriesSkipsErrorResults(t *testing.T) {
	ctx := &ConversationContext{
		entries:             make([]conversationEntry, 0),
		toolResultReplacements: make(map[string]string),
		clearedToolResults:     make(map[string]bool),
	}

	// Add a tool_use entry
	largeText := stringsRepeat("x", 8000)
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "assistant",
		content: ToolUseContent{
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "tool-1", Name: "exec"}},
		},
	})

	// Add a tool_result entry with error
	isError := anthropic.Bool(true)
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "user",
		content: ToolResultContent{
			{ToolUseID: "tool-1", IsError: isError, Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: largeText}},
			}},
		},
	})

	trimmed := ctx.SoftTrimEntries(0)
	if trimmed != 0 {
		t.Errorf("expected 0 trimmed (error result protected), got %d", trimmed)
	}
}

func TestSoftTrimEntriesSkipsNonCompactableTools(t *testing.T) {
	ctx := &ConversationContext{
		entries:             make([]conversationEntry, 0),
		toolResultReplacements: make(map[string]string),
		clearedToolResults:     make(map[string]bool),
	}

	// Add a tool_use entry for a non-compactable tool
	largeText := stringsRepeat("x", 8000)
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "assistant",
		content: ToolUseContent{
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "tool-1", Name: "memory_add"}},
		},
	})

	// Add a tool_result entry
	ctx.entries = append(ctx.entries, conversationEntry{
		role: "user",
		content: ToolResultContent{
			{ToolUseID: "tool-1", Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: largeText}},
			}},
		},
	})

	trimmed := ctx.SoftTrimEntries(0)
	if trimmed != 0 {
		t.Errorf("expected 0 trimmed (non-compactable tool), got %d", trimmed)
	}
}

func TestSoftTrimPreservesHeadAndTail(t *testing.T) {
	ctx := &ConversationContext{
		entries:             make([]conversationEntry, 0),
		toolResultReplacements: make(map[string]string),
		clearedToolResults:     make(map[string]bool),
	}

	// Create a large output with distinctive head and tail
	head := "BEGIN_OUTPUT_MARKER"
	tail := "END_OUTPUT_MARKER"
	middle := stringsRepeat("x", 8000)
	largeText := head + middle + tail

	ctx.entries = append(ctx.entries, conversationEntry{
		role: "assistant",
		content: ToolUseContent{
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "tool-1", Name: "read_file"}},
		},
	})

	ctx.entries = append(ctx.entries, conversationEntry{
		role: "user",
		content: ToolResultContent{
			{ToolUseID: "tool-1", Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: largeText}},
			}},
		},
	})

	ctx.SoftTrimEntries(0)

	// Verify head is preserved
	for _, entry := range ctx.entries {
		if results, ok := entry.content.(ToolResultContent); ok {
			for _, r := range results {
				for _, cb := range r.Content {
					if cb.OfText != nil {
						if !strings.HasPrefix(cb.OfText.Text, head) {
							t.Error("trimmed text should preserve head")
						}
						if !strings.HasSuffix(cb.OfText.Text, tail) {
							t.Error("trimmed text should preserve tail")
						}
					}
				}
			}
		}
	}
}
