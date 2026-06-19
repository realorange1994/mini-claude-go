package main

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestPruneOldReasoning(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "question 1"}},
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "old reasoning 1"}},
			{OfText: &anthropic.TextBlockParam{Text: "answer 1"}},
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "question 2"}},
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "old reasoning 2"}},
			{OfText: &anthropic.TextBlockParam{Text: "answer 2"}},
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "question 3"}},
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "recent reasoning"}},
			{OfText: &anthropic.TextBlockParam{Text: "answer 3"}},
		}},
	}

	// keepTurns=1 means only keep reasoning in the last user turn's response
	pruned, dropped := pruneOldReasoning(messages, 1)
	if pruned == 0 {
		t.Error("expected at least 1 pruned block")
	}
	if dropped == 0 {
		t.Error("expected non-zero chars dropped")
	}

	// Recent reasoning should be preserved
	lastAssistant := &messages[5]
	for _, block := range lastAssistant.Content {
		if block.OfThinking != nil && block.OfThinking.Thinking == "" {
			t.Error("recent reasoning should not be blanked")
		}
	}
}

func TestPruneOldReasoning_NoReasoning(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello"}},
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hi"}},
		}},
	}

	pruned, dropped := pruneOldReasoning(messages, 3)
	if pruned != 0 || dropped != 0 {
		t.Errorf("expected 0/0 for no reasoning, got %d/%d", pruned, dropped)
	}
}

func TestDetectThinkOnly_WithText(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "reasoning..."}},
			{OfText: &anthropic.TextBlockParam{Text: "answer"}},
		}},
	}

	if detectThinkOnly(messages) {
		t.Error("should not detect think-only when text present")
	}
}

func TestDetectThinkOnly_WithToolUse(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "reasoning..."}},
			{OfToolUse: &anthropic.ToolUseBlockParam{ID: "1", Name: "test"}},
		}},
	}

	if detectThinkOnly(messages) {
		t.Error("should not detect think-only when tool use present")
	}
}

func TestDetectThinkOnly_OnlyReasoning(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "just thinking..."}},
		}},
	}

	if !detectThinkOnly(messages) {
		t.Error("should detect think-only when only reasoning present")
	}
}

func TestDetectThinkOnly_Empty(t *testing.T) {
	if detectThinkOnly(nil) {
		t.Error("should not detect think-only for empty messages")
	}
}

func TestGetThinkOnlyNudge(t *testing.T) {
	nudge := getThinkOnlyNudge()
	if nudge == "" {
		t.Error("expected non-empty nudge")
	}
}

func TestEstimateReasoningTokens(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "this is about 40 characters of reasoning text!!"}},
		}},
	}

	tokens := estimateReasoningTokens(messages)
	if tokens <= 0 {
		t.Error("expected positive token count")
	}
}

func TestPruneOldReasoning_PreservesStructure(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "q1"}},
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "old reasoning"}},
			{OfText: &anthropic.TextBlockParam{Text: "answer 1"}},
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "q2"}},
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: "recent reasoning"}},
			{OfText: &anthropic.TextBlockParam{Text: "answer 2"}},
		}},
	}

	pruneOldReasoning(messages, 1)

	// Old reasoning should be blanked but text preserved
	oldAssistant := &messages[1]
	hasThinking := false
	hasText := false
	for _, block := range oldAssistant.Content {
		if block.OfThinking != nil {
			hasThinking = true
			if block.OfThinking.Thinking != "" {
				t.Error("old reasoning should be blanked")
			}
		}
		if block.OfText != nil && block.OfText.Text != "" {
			hasText = true
		}
	}
	if !hasThinking {
		t.Error("thinking block should still exist")
	}
	if !hasText {
		t.Error("text block should be preserved")
	}
}