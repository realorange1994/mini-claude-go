package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestConversationContextAddUserMessage(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello")
	ctx.AddUserMessage("World")

	if len(ctx.entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(ctx.entries))
	}
	if ctx.entries[0].role != "user" || ctx.entries[1].role != "user" {
		t.Errorf("expected user role for both entries")
	}
}

func TestConversationContextAddAssistantText(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddAssistantText("Response 1")
	ctx.AddAssistantText("Response 2")

	if len(ctx.entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(ctx.entries))
	}
}

func TestConversationContextAddAssistantTextEmpty(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddAssistantText("")
	ctx.AddAssistantText("")

	if len(ctx.entries) != 0 {
		t.Errorf("expected 0 entries for empty text, got %d", len(ctx.entries))
	}
}

func TestConversationContextAddToolResults(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	results := []anthropic.ToolResultBlockParam{
		{ToolUseID: "tool1", Content: []anthropic.ToolResultBlockParamContentUnion{}},
	}
	ctx.AddToolResults(results)

	if len(ctx.entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(ctx.entries))
	}
	if ctx.entries[0].role != "user" {
		t.Errorf("expected user role for tool results")
	}
}

func TestConversationContextBuildMessages(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello")
	ctx.AddAssistantText("Hi there!")

	messages := ctx.BuildMessages()

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected first message to be user")
	}
	if messages[1].Role != anthropic.MessageParamRoleAssistant {
		t.Errorf("expected second message to be assistant")
	}
}

func TestConversationContextTruncateIfNeeded(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextMsgs = 5
	ctx := NewConversationContext(cfg)

	// Add more messages than the limit
	for i := 0; i < 10; i++ {
		ctx.AddUserMessage("message")
	}

	// Should be truncated
	if len(ctx.entries) > 5 {
		t.Errorf("expected entries to be truncated to <= 5, got %d", len(ctx.entries))
	}
}

func TestConversationContextTruncateHistory(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	for i := 0; i < 20; i++ {
		ctx.AddUserMessage("message")
	}

	ctx.TruncateHistory()

	// Should keep first + last 10
	if len(ctx.entries) > 11 {
		t.Errorf("expected entries to be truncated, got %d", len(ctx.entries))
	}
}

func TestConversationContextAggressiveTruncateHistory(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	for i := 0; i < 20; i++ {
		ctx.AddUserMessage("message")
	}

	ctx.AggressiveTruncateHistory()

	// Should keep first + last 5
	if len(ctx.entries) > 6 {
		t.Errorf("expected entries to be truncated aggressively, got %d", len(ctx.entries))
	}
}

func TestConversationContextMinimumHistory(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	for i := 0; i < 20; i++ {
		ctx.AddUserMessage("message")
	}

	ctx.MinimumHistory()

	// Should keep first + last 2
	if len(ctx.entries) > 3 {
		t.Errorf("expected entries to be truncated to minimum, got %d", len(ctx.entries))
	}
}

func TestConversationContextSetSystemPrompt(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.SetSystemPrompt("You are a helpful assistant.")

	if ctx.SystemPrompt() != "You are a helpful assistant." {
		t.Errorf("expected system prompt to be set")
	}
}

// ─── Tool pairing validation ───

func TestValidateToolPairingKeepsValid(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Run ls")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "exec", "input": map[string]any{}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "output"}},
		}},
	})

	ctx.ValidateToolPairing()
	if len(ctx.entries) != 3 {
		t.Errorf("expected 3 entries (nothing should be removed), got %d", len(ctx.entries))
	}
}

func TestValidateToolPairingBackfillsOrphan(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Start")
	// Simulate truncation: tool_use is gone but tool_result remains
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_deleted", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "orphan"}},
		}},
	})
	ctx.AddAssistantText("Response")
	ctx.AddUserMessage("Next")

	if len(ctx.entries) != 4 {
		t.Fatalf("expected 4 entries before validation, got %d", len(ctx.entries))
	}
	ctx.ValidateToolPairing()
	// Orphaned tool_result should be backfilled with a synthetic tool_use,
	// so we get: user, assistant(synth_tool_use), user(tool_result), assistant("Response"), user("Next")
	if len(ctx.entries) != 5 {
		t.Errorf("expected 5 entries (orphan backfilled with synthetic tool_use), got %d", len(ctx.entries))
		for i, e := range ctx.entries {
			t.Logf("  entry %d: role=%s", i, e.role)
		}
	}
	// The synthetic tool_use should have the original tool_use_id
	if ctx.entries[1].role != "assistant" {
		t.Errorf("expected entry 1 to be assistant (synthetic tool_use), got %s", ctx.entries[1].role)
	}
}

func TestValidateToolPairingPartialRemoval(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Start")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "a", "input": map[string]any{}},
		{"id": "call_2", "name": "b", "input": map[string]any{}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "r1"}},
		}},
		{ToolUseID: "call_2", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "r2"}},
		}},
		{ToolUseID: "call_deleted", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "orphan"}},
		}},
	})

	ctx.ValidateToolPairing()
	if len(ctx.entries) != 3 {
		t.Errorf("expected 3 entries (one orphaned result removed), got %d", len(ctx.entries))
	}
}

// ─── Role alternation fix ───

func TestFixRoleAlternationMergesConsecutiveUser(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("First")
	ctx.AddUserMessage("Second")
	ctx.AddAssistantText("Response")

	if len(ctx.entries) != 3 {
		t.Fatalf("expected 3 entries before fix, got %d", len(ctx.entries))
	}
	ctx.FixRoleAlternation()
	// Two consecutive user messages should be merged
	if len(ctx.entries) != 2 {
		t.Errorf("expected 2 entries after fix, got %d", len(ctx.entries))
	}
}

func TestFixRoleAlternationMergesConsecutiveAssistant(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello")
	ctx.AddAssistantText("Part 1")
	ctx.AddAssistantText("Part 2")

	if len(ctx.entries) != 3 {
		t.Fatalf("expected 3 entries before fix, got %d", len(ctx.entries))
	}
	ctx.FixRoleAlternation()
	if len(ctx.entries) != 2 {
		t.Errorf("expected 2 entries after fix, got %d", len(ctx.entries))
	}
}

func TestFixRoleAlternationPreservesValidSequence(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("A")
	ctx.AddAssistantText("B")
	ctx.AddUserMessage("C")
	ctx.AddAssistantText("D")

	before := len(ctx.entries)
	ctx.FixRoleAlternation()
	if len(ctx.entries) != before {
		t.Errorf("expected %d entries (no change), got %d", before, len(ctx.entries))
	}
}

func TestFixRoleAlternationPreservesSystemMessages(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("A")
	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	ctx.AddUserMessage("B")

	ctx.FixRoleAlternation()
	// System messages should be preserved
	if len(ctx.entries) < 2 {
		t.Errorf("expected at least 2 entries (system preserved), got %d", len(ctx.entries))
	}
}

// --- FixRoleAlternation type mismatch tests ---

func TestFixRoleAlternationTypeMismatchTextToolResult(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("First text")
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "result"}},
		}},
	})

	if len(ctx.entries) != 2 {
		t.Fatalf("expected 2 entries before fix, got %d", len(ctx.entries))
	}

	ctx.FixRoleAlternation()
	// Type-mismatched entries (TextContent + ToolResultContent) are now kept
	// separate instead of converting to TextContent, which destroyed tool pairing.
	if len(ctx.entries) != 2 {
		t.Errorf("expected 2 entries after fix (kept separate), got %d", len(ctx.entries))
	}
	// First entry should still be TextContent
	if _, ok := ctx.entries[0].content.(TextContent); !ok {
		t.Errorf("expected first entry to be TextContent, got %T", ctx.entries[0].content)
	}
	// Second entry should still be ToolResultContent (not converted to TextContent)
	if _, ok := ctx.entries[1].content.(ToolResultContent); !ok {
		t.Errorf("expected second entry to be ToolResultContent, got %T", ctx.entries[1].content)
	}
}

func TestEntryContentToText(t *testing.T) {
	if entryContentToText(TextContent("hello")) != "hello" {
		t.Error("TextContent conversion failed")
	}

	tc := ToolUseContent([]anthropic.ContentBlockParamUnion{
		{OfToolUse: &anthropic.ToolUseBlockParam{ID: "c1", Name: "exec", Input: map[string]any{}}},
	})
	result := entryContentToText(tc)
	if result == "" {
		t.Error("ToolUseContent conversion should not be empty")
	}

	boundary := CompactBoundaryContent{Trigger: CompactTriggerAuto, PreCompactTokens: 5000}
	result2 := entryContentToText(boundary)
	if result2 == "" {
		t.Error("CompactBoundaryContent conversion should not be empty")
	}

	if entryContentToText(SummaryContent("test summary")) != "test summary" {
		t.Error("SummaryContent conversion failed")
	}
}

// ─── Turn interruption detection tests ───

func TestDetectTurnInterruptionEmpty(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedNone {
		t.Errorf("expected none for empty context, got %v", state.Kind)
	}
}

func TestDetectTurnInterruptionCompletedTurn(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	ctx.AddAssistantText("Response")
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedNone {
		t.Errorf("expected none for completed turn, got %v", state.Kind)
	}
}

func TestDetectTurnInterruptionInterruptedPrompt(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	// No assistant response — user prompt was never acted upon
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedPrompt {
		t.Errorf("expected interrupted_prompt, got %v", state.Kind)
	}
	if state.PromptText != "Hello" {
		t.Errorf("expected prompt text 'Hello', got %q", state.PromptText)
	}
}

func TestDetectTurnInterruptionInterruptedTurn(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "exec", "input": map[string]any{}},
	})
	// No tool result — assistant was mid-response when interrupted
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedTurn {
		t.Errorf("expected interrupted_turn, got %v", state.Kind)
	}
}

func TestDetectTurnInterruptionToolResultWithoutFollowUp(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "exec", "input": map[string]any{}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "output"}},
		}},
	})
	// Tool result but no follow-up assistant text
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedTurn {
		t.Errorf("expected interrupted_turn (tool result without follow-up), got %v", state.Kind)
	}
}

func TestDetectTurnInterruptionSkipsSystemMessages(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	ctx.AddAssistantText("Response")
	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	// Last non-system entry is assistant — completed turn
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedNone {
		t.Errorf("expected none (assistant is last non-system), got %v", state.Kind)
	}
}

func TestDetectTurnInterruptionSummaryIsNotInterrupted(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddSummary("Compact summary")
	// Summary is a meta message — not a user prompt
	state := ctx.DetectTurnInterruption()
	if state.Kind != TurnInterruptedNone {
		t.Errorf("expected none for summary, got %v", state.Kind)
	}
}

func TestApplyTurnInterruptionResumeInterruptedTurn(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "exec", "input": map[string]any{}},
	})
	// No tool result — interrupted turn

	state := ctx.DetectTurnInterruption()
	ctx.ApplyTurnInterruptionResume(state)

	// Should inject "Continue from where you left off." user message
	lastEntry := ctx.entries[len(ctx.entries)-1]
	if lastEntry.role != "user" {
		t.Errorf("expected last entry to be user, got %s", lastEntry.role)
	}
	if tc, ok := lastEntry.content.(TextContent); ok {
		if string(tc) != "Continue from where you left off." {
			t.Errorf("expected continuation message, got %q", string(tc))
		}
	} else {
		t.Errorf("expected TextContent, got %T", lastEntry.content)
	}
}

func TestApplyTurnInterruptionResumeInterruptedPrompt(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	// No assistant response — interrupted prompt

	state := ctx.DetectTurnInterruption()
	ctx.ApplyTurnInterruptionResume(state)

	// Should append a synthetic assistant sentinel
	lastEntry := ctx.entries[len(ctx.entries)-1]
	if lastEntry.role != "assistant" {
		t.Errorf("expected last entry to be assistant sentinel, got %s", lastEntry.role)
	}
	if tc, ok := lastEntry.content.(TextContent); ok {
		if string(tc) != NO_RESPONSE_REQUESTED {
			t.Errorf("expected NO_RESPONSE_REQUESTED sentinel, got %q", string(tc))
		}
	} else {
		t.Errorf("expected TextContent, got %T", lastEntry.content)
	}
}

// ─── EntryContent sealed interface tests ───

func TestTextContentType(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello")
	entry := ctx.entries[0]
	if _, ok := entry.content.(TextContent); !ok {
		t.Errorf("expected TextContent, got %T", entry.content)
	}
	if string(entry.content.(TextContent)) != "Hello" {
		t.Errorf("expected 'Hello', got %q", entry.content.(TextContent))
	}
}

func TestToolUseContentType(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "call_1", "name": "exec", "input": map[string]any{}},
	})
	entry := ctx.entries[0]
	if _, ok := entry.content.(ToolUseContent); !ok {
		t.Errorf("expected ToolUseContent, got %T", entry.content)
	}
}

func TestToolResultContentType(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1"},
	})
	entry := ctx.entries[0]
	if _, ok := entry.content.(ToolResultContent); !ok {
		t.Errorf("expected ToolResultContent, got %T", entry.content)
	}
}

func TestCompactBoundaryContentType(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	entry := ctx.entries[0]
	boundary, ok := entry.content.(CompactBoundaryContent)
	if !ok {
		t.Fatalf("expected CompactBoundaryContent, got %T", entry.content)
	}
	if boundary.Trigger != CompactTriggerAuto {
		t.Errorf("expected trigger Auto, got %v", boundary.Trigger)
	}
	if boundary.PreCompactTokens != 1000 {
		t.Errorf("expected 1000 tokens, got %d", boundary.PreCompactTokens)
	}
}

func TestSummaryContentType(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddSummary("Short summary")
	entry := ctx.entries[0]
	summary, ok := entry.content.(SummaryContent)
	if !ok {
		t.Fatalf("expected SummaryContent, got %T", entry.content)
	}
	if string(summary) != "Short summary" {
		t.Errorf("expected 'Short summary', got %q", summary)
	}
}

func TestBuildMessagesCompactBoundaryDiscardsPrior(t *testing.T) {
	// CompactBoundary should discard all messages before it.
	// Only summary + messages after the boundary are sent to the API.
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello")
	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	ctx.AddUserMessage("World")

	messages := ctx.BuildMessages()
	// Should have 1 message: "World" (the boundary discards "Hello")
	// The boundary marker itself is also skipped
	if len(messages) != 1 {
		t.Errorf("expected 1 message (boundary discards prior), got %d", len(messages))
	}
	if messages[0].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected first message to be user")
	}
}

func TestEstimatedTokensWithNewTypes(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello World")       // 11 chars
	ctx.AddSummary("Short summary")          // 13 chars

	tokens := ctx.EstimatedTokens()
	// ~24 chars / 4 = ~6 tokens
	if tokens < 4 || tokens > 10 {
		t.Errorf("expected ~6 tokens, got %d", tokens)
	}
}

func TestEntriesReplace(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("First")
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "c1", "name": "exec", "input": map[string]any{"cmd": "ls"}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "c1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "file.go"}},
		}},
	})

	entries := ctx.entries
	ctx.ReplaceEntries(entries)
	if len(ctx.entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(ctx.entries))
	}
}

func TestContentEntrySealedInterface(t *testing.T) {
	// Verify that EntryContent is a valid interface
	var content EntryContent

	content = TextContent("hello")
	if _, ok := content.(TextContent); !ok {
		t.Error("TextContent should implement EntryContent")
	}

	content = ToolUseContent(nil)
	if _, ok := content.(ToolUseContent); !ok {
		t.Error("ToolUseContent should implement EntryContent")
	}

	content = ToolResultContent(nil)
	if _, ok := content.(ToolResultContent); !ok {
		t.Error("ToolResultContent should implement EntryContent")
	}

	content = CompactBoundaryContent{}
	if _, ok := content.(CompactBoundaryContent); !ok {
		t.Error("CompactBoundaryContent should implement EntryContent")
	}

	content = SummaryContent("test")
	if _, ok := content.(SummaryContent); !ok {
		t.Error("SummaryContent should implement EntryContent")
	}
}

func TestMicroCompactEntries(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add a user message first
	ctx.AddUserMessage("initial question")

	// Add 10 tool result entries, each with a unique ToolUseID
	for i := 0; i < 10; i++ {
		// Add assistant tool_use
		toolCalls := []map[string]any{
			{
				"id":    fmt.Sprintf("tool_%d", i),
				"name":  "read_file",
				"input": map[string]any{"path": fmt.Sprintf("file_%d.go", i)},
			},
		}
		ctx.AddAssistantToolCalls(toolCalls)

		// Add tool result
		results := []anthropic.ToolResultBlockParam{
			{
				ToolUseID: fmt.Sprintf("tool_%d", i),
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: fmt.Sprintf("Content of file_%d.go - this is a long output that should be cleared", i)}},
				},
			},
		}
		ctx.AddToolResults(results)
	}

	// Verify we have entries
	if ctx.Len() < 20 {
		t.Errorf("expected at least 20 entries, got %d", ctx.Len())
	}

	// Run micro-compact with keepRecent=5, minCharCount=1 (clear everything beyond recent)
	cleared := ctx.MicroCompactEntries(5, "[cleared]", 1)
	if cleared != 5 {
		t.Errorf("expected 5 entries cleared, got %d", cleared)
	}

	// Verify the last 5 tool results still have original content
	entries := ctx.Entries()
	toolResultCount := 0
	clearedCount := 0
	for _, entry := range entries {
		if results, ok := entry.content.(ToolResultContent); ok {
			for _, r := range results {
				for _, c := range r.Content {
					if c.OfText != nil {
						if c.OfText.Text == "[cleared]" {
							clearedCount++
						} else if strings.Contains(c.OfText.Text, "Content of file_") {
							toolResultCount++
						}
					}
				}
				// Verify ToolUseID is preserved (pairing intact)
				if r.ToolUseID == "" {
					t.Error("ToolUseID should be preserved after micro-compact")
				}
			}
		}
	}

	if toolResultCount != 5 {
		t.Errorf("expected 5 recent tool results preserved, got %d", toolResultCount)
	}
	if clearedCount != 5 {
		t.Errorf("expected 5 cleared tool results, got %d", clearedCount)
	}
}

func TestMicroCompactEntriesKeepAll(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add only 3 tool results (each with toolcall + toolresult)
	for i := 0; i < 3; i++ {
		toolCalls := []map[string]any{
			{"id": fmt.Sprintf("tool_%d", i), "name": "read_file", "input": map[string]any{"path": fmt.Sprintf("file_%d.go", i)}},
		}
		ctx.AddAssistantToolCalls(toolCalls)
		results := []anthropic.ToolResultBlockParam{
			{
				ToolUseID: fmt.Sprintf("tool_%d", i),
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: strings.Repeat("x", 3000)}},
				},
			},
		}
		ctx.AddToolResults(results)
	}

	// With keepRecent=5, nothing should be cleared (3 tool results < 5)
	cleared := ctx.MicroCompactEntries(5, "[cleared]", 1)
	if cleared != 0 {
		t.Errorf("expected 0 entries cleared (all recent), got %d", cleared)
	}
}

func TestMicroCompactEntriesDefaultValues(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add 8 tool_use + tool_result pairs (tool results need matching tool_use entries for compactable check)
	for i := 0; i < 8; i++ {
		toolCalls := []map[string]any{
			{
				"id":   fmt.Sprintf("tool_%d", i),
				"name": "read_file",
				"input": map[string]any{"path": fmt.Sprintf("file_%d.go", i)},
			},
		}
		ctx.AddAssistantToolCalls(toolCalls)

		results := []anthropic.ToolResultBlockParam{
			{
				ToolUseID: fmt.Sprintf("tool_%d", i),
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: strings.Repeat("x", 3000)}},
				},
			},
		}
		ctx.AddToolResults(results)
	}

	// Call with keepRecent=0 (should default to 5), minCharCount=1
	cleared := ctx.MicroCompactEntries(0, "", 1)
	if cleared != 3 {
		t.Errorf("expected 3 entries cleared (8-5), got %d", cleared)
	}
}


func TestMicroCompactMinCharCount(t *testing.T) {
	ctx := NewConversationContext(DefaultConfig())

	// Add a user message first
	ctx.AddUserMessage("checking")

	// Add a small tool call + small result (below minCharCount, should NOT be cleared)
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "tool_small", "name": "read_file", "input": map[string]any{"path": "a.txt"}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "tool_small", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "short output"}},
		}},
	})

	// Add a large tool call + large result (above minCharCount, should be cleared if beyond keepRecent)
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "tool_large", "name": "read_file", "input": map[string]any{"path": "b.txt"}},
	})
	largeResult := strings.Repeat("x", 500)
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "tool_large", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: largeResult}},
		}},
	})

	// Add a recent tool call + result (protected by keepRecent)
	ctx.AddAssistantToolCalls([]map[string]any{
		{"id": "tool_recent", "name": "read_file", "input": map[string]any{"path": "c.txt"}},
	})
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "tool_recent", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "recent"}},
		}},
	})

	// With keepRecent=1, minCharCount=100: large result is old and large enough → cleared,
	// small result is old but too small → preserved, recent result is within keepRecent → preserved
	cleared := ctx.MicroCompactEntries(1, "[cleared]", 100)
	if cleared != 1 {
		t.Errorf("expected 1 entry cleared (large only), got %d", cleared)
	}

	// Verify small result is still intact
	entries := ctx.Entries()
	foundSmall := false
	for _, e := range entries {
		if results, ok := e.content.(ToolResultContent); ok {
			for _, r := range results {
				if r.ToolUseID == "tool_small" {
					for _, c := range r.Content {
						if c.OfText != nil && c.OfText.Text == "short output" {
							foundSmall = true
						}
					}
				}
			}
		}
	}
	if !foundSmall {
		t.Error("small tool result was incorrectly cleared")
	}
}


func TestAttachmentContentType(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddAttachment("[Post-compact file recovery: main.go]\n```\npackage main\n```")
	entry := ctx.entries[0]
	if _, ok := entry.content.(AttachmentContent); !ok {
		t.Fatalf("expected AttachmentContent, got %T", entry.content)
	}
	if string(entry.content.(AttachmentContent)) != "[Post-compact file recovery: main.go]\n```\npackage main\n```" {
		t.Errorf("expected attachment content, got %q", entry.content.(AttachmentContent))
	}
}

func TestBuildMessagesWithAttachment(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("Hello")
	ctx.AddAttachment("[Post-compact file recovery: main.go]\n```\npackage main\n```")

	messages := ctx.BuildMessages()
	// Both entries are user-role and get merged into a single message
	// by the consecutive same-role merge step.
	if len(messages) != 1 {
		t.Errorf("expected 1 merged message, got %d", len(messages))
	}
	// The merged message should contain both the user text and attachment
	if messages[0].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected merged message to be user role")
	}
}

func TestAttachmentContentSealedInterface(t *testing.T) {
	var content EntryContent
	content = AttachmentContent("test attachment")
	if _, ok := content.(AttachmentContent); !ok {
		t.Error("AttachmentContent should implement EntryContent")
	}
}

// ─── Incremental compaction (summarized flag) ────────────────────────────────

func TestKeepRecentMessagesAdaptiveSkipsSummarized(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// Add entries: some already summarized, some not
	ctx.AddUserMessage("old summarized 1")
	ctx.entries[0].summarized = true
	ctx.AddAssistantText("old summarized response 1")
	ctx.entries[1].summarized = true
	ctx.AddUserMessage("old summarized 2")
	ctx.entries[2].summarized = true
	ctx.AddUserMessage("new not summarized 1")
	ctx.AddAssistantText("new response 1")
	ctx.AddUserMessage("new not summarized 2")

	// Add compact boundary
	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	ctx.AddSummary("Summary of old content")

	// Keep recent: should skip summarized entries and only keep new ones
	ctx.KeepRecentMessagesAdaptive(100, 2, 10000)

	// Verify that summarized entries were skipped
	messages := ctx.BuildMessages()
	// Should contain: summary + new entries (not summarized ones)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				if strings.Contains(text, "old summarized") {
					t.Errorf("summarized entries should not appear in messages: %q", text)
				}
			}
		}
	}
}

func TestKeepRecentMessagesAdaptiveMarksKeptAsSummarized(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("entry A")
	ctx.AddAssistantText("response A")
	ctx.AddUserMessage("entry B")
	ctx.AddAssistantText("response B")
	ctx.AddUserMessage("entry C")

	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	ctx.AddSummary("Summary")

	// Keep recent messages
	ctx.KeepRecentMessagesAdaptive(100, 2, 10000)

	// Check that kept entries are marked as summarized
	summarizedCount := 0
	for _, entry := range ctx.entries {
		if entry.summarized {
			summarizedCount++
		}
	}
	if summarizedCount == 0 {
		t.Error("kept entries should be marked as summarized for incremental compaction")
	}
}

func TestKeepRecentMessagesAdaptiveIncrementalCompact(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	// First compaction: summarize entries A-D, keep E-F
	ctx.AddUserMessage("entry A")
	ctx.AddAssistantText("response A")
	ctx.AddUserMessage("entry B")
	ctx.AddAssistantText("response B")
	ctx.AddUserMessage("entry C")
	ctx.AddAssistantText("response C")
	ctx.AddUserMessage("entry D")
	ctx.AddAssistantText("response D")
	ctx.AddUserMessage("entry E")
	ctx.AddAssistantText("response E")
	ctx.AddUserMessage("entry F")

	ctx.AddCompactBoundary(CompactTriggerAuto, 1000)
	ctx.AddSummary("First summary")

	ctx.KeepRecentMessagesAdaptive(100, 2, 10000)

	// KeepRecentMessagesAdaptive appends kept entries after the boundary
	// and marks them as summarized=true. Check that entries after boundary
	// are marked as summarized.
	boundaryIdx := -1
	for i := len(ctx.entries) - 1; i >= 0; i-- {
		if _, ok := ctx.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}
	if boundaryIdx < 0 {
		t.Fatal("expected a compact boundary")
	}

	// Entries after boundary+summary should be marked as summarized
	summarizedAfter := 0
	for i := boundaryIdx + 1; i < len(ctx.entries); i++ {
		if ctx.entries[i].summarized {
			summarizedAfter++
		}
	}
	if summarizedAfter == 0 {
		t.Error("entries after boundary should be marked as summarized")
	}

	// Now add more entries after first compaction
	ctx.AddUserMessage("entry G")
	ctx.AddAssistantText("response G")
	ctx.AddUserMessage("entry H")

	// Second compaction: should skip already-summarized entries after boundary
	ctx.AddCompactBoundary(CompactTriggerAuto, 500)
	ctx.AddSummary("Second summary")
	ctx.KeepRecentMessagesAdaptive(100, 2, 10000)

	// Verify second compaction skipped summarized entries
	boundaryIdx2 := -1
	for i := len(ctx.entries) - 1; i >= 0; i-- {
		if _, ok := ctx.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx2 = i
			break
		}
	}
	skippedSummarized := 0
	for i := boundaryIdx2 - 1; i >= 0; i-- {
		if ctx.entries[i].summarized {
			skippedSummarized++
		}
	}
	if skippedSummarized == 0 {
		t.Error("second compaction should find some already-summarized entries to skip")
	}
}

func TestConversationEntrySummarizedField(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("test")
	if ctx.entries[0].summarized {
		t.Error("new entries should not be summarized by default")
	}

	ctx.entries[0].summarized = true
	if !ctx.entries[0].summarized {
		t.Error("should be able to set summarized flag")
	}
}

func TestBuildMessagesSameRoleMerge(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)

	ctx.AddUserMessage("text A")
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "result"}},
		}},
	})

	messages := ctx.BuildMessages()
	// Both are user-role, should be merged into one message
	if len(messages) != 1 {
		t.Errorf("expected 1 merged message, got %d", len(messages))
	}
	if messages[0].Role != anthropic.MessageParamRoleUser {
		t.Errorf("expected user role, got %v", messages[0].Role)
	}
	// Should contain both text and tool_result blocks
	hasText := false
	hasToolResult := false
	for _, block := range messages[0].Content {
		if block.OfText != nil {
			hasText = true
		}
		if block.OfToolResult != nil {
			hasToolResult = true
		}
	}
	if !hasText {
		t.Error("merged message should contain text block")
	}
	if !hasToolResult {
		t.Error("merged message should contain tool_result block")
	}
}
