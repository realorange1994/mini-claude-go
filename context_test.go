package main

import (
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

func TestValidateToolPairingRemovesOrphan(t *testing.T) {
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
	// Orphaned tool_result message should be removed
	if len(ctx.entries) != 3 {
		t.Errorf("expected 3 entries (orphan removed), got %d", len(ctx.entries))
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
	if len(ctx.entries) != 1 {
		t.Errorf("expected 1 entry after fix (merged), got %d", len(ctx.entries))
	}
	if _, ok := ctx.entries[0].content.(TextContent); !ok {
		t.Errorf("expected merged entry to be TextContent, got %T", ctx.entries[0].content)
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
