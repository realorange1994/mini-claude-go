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
