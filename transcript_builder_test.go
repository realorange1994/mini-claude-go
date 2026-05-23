package main

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// ─── BuildCompactTranscript additional coverage ──────────────────────────────

func TestBuildCompactTranscriptTruncatesLongUser(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	longText := strings.Repeat("x", 600)
	ctx.AddUserMessage(longText)

	result := BuildCompactTranscript(ctx, 20)
	if !strings.Contains(result, "...") {
		t.Error("long user message should be truncated with ...")
	}
}

func TestBuildCompactTranscriptMaxMessages(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	for i := 0; i < 50; i++ {
		ctx.AddUserMessage("msg " + string(rune('a'+i%26)))
	}

	result := BuildCompactTranscript(ctx, 10)
	count := strings.Count(result, "[User]")
	if count > 10 {
		t.Errorf("expected <= 10 user messages, got %d", count)
	}
}

func TestBuildCompactTranscriptZeroMaxMessages(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("hello")

	result := BuildCompactTranscript(ctx, 0)
	// Should use default of 20
	if !strings.Contains(result, "hello") {
		t.Error("transcript should contain user message with default maxMessages")
	}
}

func TestBuildCompactTranscriptToolCalls(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("list files")
	ctx.AddAssistantToolCalls([]map[string]any{
		{
			"id":    "tu_1",
			"name":  "exec",
			"input": map[string]any{"command": "ls"},
		},
	})

	result := BuildCompactTranscript(ctx, 20)
	if !strings.Contains(result, "[Tool: exec]") {
		t.Errorf("transcript should contain tool call, got %q", result)
	}
}

func TestBuildCompactTranscriptToolResult(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{
			ToolUseID: "tu_1",
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: "file1.txt\nfile2.txt"}},
			},
		},
	})

	result := BuildCompactTranscript(ctx, 20)
	if !strings.Contains(result, "[Result]") {
		t.Errorf("transcript should contain tool result, got %q", result)
	}
}

func TestBuildCompactTranscriptToolResultTruncated(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{
			ToolUseID: "tu_1",
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: strings.Repeat("x", 200)}},
			},
		},
	})

	result := BuildCompactTranscript(ctx, 20)
	if !strings.Contains(result, "...") {
		t.Error("long tool result should be truncated")
	}
}

func TestBuildCompactTranscriptApproval(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddToolResults([]anthropic.ToolResultBlockParam{
		{
			ToolUseID: "tu_1",
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Q: Create file?\nA: yes"}},
			},
		},
	})

	result := BuildCompactTranscript(ctx, 20)
	if !strings.Contains(result, "USER EXPLICITLY APPROVED") {
		t.Errorf("approval result should be marked, got %q", result)
	}
}

// ─── formatToolInputCompact additional coverage ──────────────────────────────

func TestFormatToolInputCompactNilInput(t *testing.T) {
	result := formatToolInputCompact("exec", nil)
	if result != "" {
		t.Errorf("expected empty string for nil input, got %q", result)
	}
}

func TestFormatToolInputCompactExecTruncate(t *testing.T) {
	input := map[string]any{"command": strings.Repeat("x", 300)}
	result := formatToolInputCompact("exec", input)
	if !strings.HasSuffix(result, "...") {
		t.Error("long command should be truncated with ...")
	}
}

func TestFormatToolInputCompactEditFilePath(t *testing.T) {
	input := map[string]any{"file_path": "main.go", "edits": []any{}}
	result := formatToolInputCompact("edit_file", input)
	if result != "main.go" {
		t.Errorf("expected 'main.go', got %q", result)
	}
}

func TestFormatToolInputCompactReadFilePath(t *testing.T) {
	input := map[string]any{"file_path": "config.yaml"}
	result := formatToolInputCompact("read_file", input)
	if result != "config.yaml" {
		t.Errorf("expected 'config.yaml', got %q", result)
	}
}

func TestFormatToolInputCompactGrepFormat(t *testing.T) {
	input := map[string]any{"pattern": "func main", "path": "main.go"}
	result := formatToolInputCompact("grep", input)
	if !strings.Contains(result, "func main") {
		t.Error("result should contain pattern")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("result should contain path")
	}
}

func TestFormatToolInputCompactGenericTruncate(t *testing.T) {
	input := map[string]any{"data": strings.Repeat("x", 100)}
	result := formatToolInputCompact("unknown_tool", input)
	if !strings.HasSuffix(result, "...") {
		t.Error("long param value should be truncated with ...")
	}
}

func TestFormatToolInputCompactGenericParams(t *testing.T) {
	input := map[string]any{"foo": "bar", "count": 42}
	result := formatToolInputCompact("unknown_tool", input)
	if !strings.Contains(result, "foo=bar") && !strings.Contains(result, "count=42") {
		t.Errorf("generic format should contain params, got %q", result)
	}
}

// ─── isAskUserApproval ───────────────────────────────────────────────────────

func TestIsAskUserApprovalYes(t *testing.T) {
	if !isAskUserApproval("Q: Continue?\nA: yes") {
		t.Error("yes should be approval")
	}
}

func TestIsAskUserApprovalOk(t *testing.T) {
	if !isAskUserApproval("Q: Proceed?\nA: ok") {
		t.Error("ok should be approval")
	}
}

func TestIsAskUserApprovalContinue(t *testing.T) {
	if !isAskUserApproval("Q: Continue?\nA: continue") {
		t.Error("continue should be approval")
	}
}

func TestIsAskUserApprovalAllow(t *testing.T) {
	if !isAskUserApproval("Q: Allow?\nA: allow") {
		t.Error("allow should be approval")
	}
}

func TestIsAskUserApprovalProceed(t *testing.T) {
	if !isAskUserApproval("Q: Proceed?\nA: proceed") {
		t.Error("proceed should be approval")
	}
}

func TestIsAskUserApprovalGoAhead(t *testing.T) {
	if !isAskUserApproval("Q: Go?\nA: go ahead") {
		t.Error("go ahead should be approval")
	}
}

func TestIsAskUserApprovalApproved(t *testing.T) {
	if !isAskUserApproval("Q: Approve?\nA: approved") {
		t.Error("approved should be approval")
	}
}

func TestIsAskUserApprovalNo(t *testing.T) {
	if isAskUserApproval("Q: Continue?\nA: no") {
		t.Error("no should not be approval")
	}
}

func TestIsAskUserApprovalCancel(t *testing.T) {
	if isAskUserApproval("Q: Continue?\nA: cancel") {
		t.Error("cancel should not be approval")
	}
}

func TestIsAskUserApprovalNoQuestion(t *testing.T) {
	if isAskUserApproval("just some random text") {
		t.Error("text without Q:/A: format should not be approval")
	}
}

func TestIsAskUserApprovalEmpty(t *testing.T) {
	if isAskUserApproval("") {
		t.Error("empty string should not be approval")
	}
}

func TestIsAskUserApprovalCaseInsensitive(t *testing.T) {
	if !isAskUserApproval("Q: Continue?\nA: YES") {
		t.Error("YES should be recognized as approval")
	}
	if !isAskUserApproval("Q: Continue?\nA: Ok") {
		t.Error("Ok should be recognized as approval")
	}
}

func TestIsAskUserApprovalCreate(t *testing.T) {
	if !isAskUserApproval("Q: Create file?\nA: create") {
		t.Error("create should be approval")
	}
}

func TestIsAskUserApprovalYesAllow(t *testing.T) {
	if !isAskUserApproval("Q: Allow?\nA: yes allow") {
		t.Error("yes allow should be approval")
	}
}

// ─── extractToolResultText ───────────────────────────────────────────────────

func TestExtractToolResultTextNil(t *testing.T) {
	result := extractToolResultText(nil)
	if result != "" {
		t.Errorf("expected empty string for nil, got %q", result)
	}
}

func TestExtractToolResultTextEmpty(t *testing.T) {
	result := extractToolResultText([]anthropic.ToolResultBlockParamContentUnion{})
	if result != "" {
		t.Errorf("expected empty string for empty slice, got %q", result)
	}
}

func TestExtractToolResultTextSingle(t *testing.T) {
	blocks := []anthropic.ToolResultBlockParamContentUnion{
		{OfText: &anthropic.TextBlockParam{Text: "hello"}},
	}
	result := extractToolResultText(blocks)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestExtractToolResultTextMultiple(t *testing.T) {
	blocks := []anthropic.ToolResultBlockParamContentUnion{
		{OfText: &anthropic.TextBlockParam{Text: "hello"}},
		{OfText: &anthropic.TextBlockParam{Text: "world"}},
	}
	result := extractToolResultText(blocks)
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}
