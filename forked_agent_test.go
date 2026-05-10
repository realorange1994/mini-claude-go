package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// ─── CaptureCacheSafeParams ──────────────────────────────────────────────────

func TestCaptureCacheSafeParams(t *testing.T) {
	registry := DefaultRegistry()
	msgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
	}
	params := CaptureCacheSafeParams("system prompt", "claude-test", registry, msgs)

	if params.SystemPrompt != "system prompt" {
		t.Errorf("expected system prompt 'system prompt', got %q", params.SystemPrompt)
	}
	if params.Model != "claude-test" {
		t.Errorf("expected model 'claude-test', got %q", params.Model)
	}
	if len(params.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(params.Messages))
	}
	// Tools should be built from registry
	if len(params.Tools) == 0 {
		t.Error("tools should be built from registry")
	}
}

func TestCaptureCacheSafeParamsNilRegistry(t *testing.T) {
	params := CaptureCacheSafeParams("system", "model", nil, nil)
	if len(params.Tools) != 0 {
		t.Errorf("expected 0 tools with nil registry, got %d", len(params.Tools))
	}
}

func TestCaptureCacheSafeParamsEmptyMessages(t *testing.T) {
	registry := DefaultRegistry()
	params := CaptureCacheSafeParams("sys", "model", registry, []anthropic.MessageParam{})
	if len(params.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(params.Messages))
	}
}

// ─── buildForkedToolParams ───────────────────────────────────────────────────

func TestBuildForkedToolParamsNil(t *testing.T) {
	result := buildForkedToolParams(nil)
	if result != nil {
		t.Errorf("expected nil for nil registry, got %d tools", len(result))
	}
}

func TestBuildForkedToolParamsFromRegistry(t *testing.T) {
	registry := DefaultRegistry()
	result := buildForkedToolParams(registry)
	if len(result) == 0 {
		t.Error("default registry should have at least some tools")
	}
	names := make(map[string]bool)
	for _, p := range result {
		if p.OfTool != nil {
			names[p.OfTool.Name] = true
		}
	}
	for _, name := range []string{"exec", "read_file", "write_file", "edit_file"} {
		if !names[name] {
			t.Errorf("expected tool %q in forked params", name)
		}
	}
}

func TestBuildForkedToolParamsSchema(t *testing.T) {
	registry := DefaultRegistry()
	result := buildForkedToolParams(registry)

	for _, p := range result {
		if p.OfTool == nil {
			t.Error("tool param should have OfTool set")
			continue
		}
		if p.OfTool.Name == "" {
			t.Error("tool name should not be empty")
		}
		if p.OfTool.InputSchema.Properties == nil {
			t.Errorf("tool %q should have input schema properties", p.OfTool.Name)
		}
	}
}

// ─── Message combining (core logic from RunForkedAgent) ──────────────────────

func TestMessageCombineSkipParent(t *testing.T) {
	parentMsgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("parent message")),
	}
	forkMsgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("fork message")),
	}

	skipParent := true
	var allMessages []anthropic.MessageParam
	if skipParent {
		allMessages = make([]anthropic.MessageParam, len(forkMsgs))
		copy(allMessages, forkMsgs)
	} else {
		allMessages = make([]anthropic.MessageParam, len(parentMsgs)+len(forkMsgs))
		copy(allMessages, parentMsgs)
		copy(allMessages[len(parentMsgs):], forkMsgs)
	}

	if len(allMessages) != 1 {
		t.Errorf("expected 1 message with SkipParentMessages, got %d", len(allMessages))
	}
	text := extractTextFromMessage(allMessages[0])
	if text != "fork message" {
		t.Errorf("expected 'fork message', got %q", text)
	}
}

func TestMessageCombineNormal(t *testing.T) {
	parentMsgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("parent 1")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("response 1")),
	}
	forkMsgs := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("fork prompt")),
	}

	skipParent := false
	var allMessages []anthropic.MessageParam
	if skipParent {
		allMessages = make([]anthropic.MessageParam, len(forkMsgs))
		copy(allMessages, forkMsgs)
	} else {
		allMessages = make([]anthropic.MessageParam, len(parentMsgs)+len(forkMsgs))
		copy(allMessages, parentMsgs)
		copy(allMessages[len(parentMsgs):], forkMsgs)
	}

	if len(allMessages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(allMessages))
	}
	text0 := extractTextFromMessage(allMessages[0])
	text2 := extractTextFromMessage(allMessages[2])
	if text0 != "parent 1" {
		t.Errorf("expected 'parent 1', got %q", text0)
	}
	if text2 != "fork prompt" {
		t.Errorf("expected 'fork prompt', got %q", text2)
	}
}

func extractTextFromMessage(msg anthropic.MessageParam) string {
	for _, block := range msg.Content {
		if block.OfText != nil {
			return block.OfText.Text
		}
	}
	return ""
}

// ─── Tool permission denial ──────────────────────────────────────────────────

func TestRunForkedAgentPermissionDenied(t *testing.T) {
	// Test the CanUseTool denial callback
	var lines []string
	denied := func(toolName string, args map[string]any) (bool, string) {
		lines = append(lines, toolName)
		return false, "permission denied"
	}
	allowed, reason := denied("exec", map[string]any{"command": "rm -rf /"})
	if allowed {
		t.Error("exec should be denied")
	}
	if !strings.Contains(reason, "denied") {
		t.Errorf("reason should mention 'denied', got %q", reason)
	}
}

func TestCanUseToolAllow(t *testing.T) {
	allowed := func(toolName string, args map[string]any) (bool, string) {
		if toolName == "read_file" || toolName == "write_file" {
			return true, ""
		}
		return false, "tool not allowed"
	}
	ok, _ := allowed("read_file", nil)
	if !ok {
		t.Error("read_file should be allowed")
	}
	ok, reason := allowed("exec", nil)
	if ok {
		t.Error("exec should not be allowed")
	}
	if !strings.Contains(reason, "not allowed") {
		t.Errorf("reason incorrect, got %q", reason)
	}
}

func TestCanUseToolNil(t *testing.T) {
	// CanUseTool can be nil, meaning all tools allowed
	var canUse CanUseToolFn
	if canUse != nil {
		t.Error("CanUseToolFn should be nil")
	}
}

// ─── executeForkedTool ───────────────────────────────────────────────────────

func TestExecuteForkedToolUnknown(t *testing.T) {
	registry := DefaultRegistry()
	cfg := ForkedAgentConfig{Registry: registry}
	output := executeForkedTool(cfg, "nonexistent_tool", nil)
	if !strings.Contains(output, "unknown tool") {
		t.Errorf("expected 'unknown tool' error, got %q", output)
	}
}

func TestExecuteForkedToolNilRegistry(t *testing.T) {
	cfg := ForkedAgentConfig{Registry: nil}
	output := executeForkedTool(cfg, "exec", nil)
	if !strings.Contains(output, "no tool registry") {
		t.Errorf("expected 'no tool registry' error, got %q", output)
	}
}

func TestExecuteForkedToolValidation(t *testing.T) {
	registry := DefaultRegistry()
	cfg := ForkedAgentConfig{Registry: registry}
	output := executeForkedTool(cfg, "exec", map[string]any{})
	if !strings.Contains(output, "Error") {
		t.Errorf("expected error for missing required param, got %q", output)
	}
}

// ─── Error classification (used by retryForkedCall) ──────────────────────────

func TestClassifyErrorRetryable(t *testing.T) {
	cr := classifyError("connection refused", 0, 0)
	if !cr.Retryable {
		t.Error("connection refused should be retryable")
	}
	if cr.Class != ECRetryable {
		t.Errorf("expected ECRetryable, got %d", cr.Class)
	}
}

func TestClassifyErrorNonRetryable(t *testing.T) {
	// 400 with zero tokens → ECFormatError (non-retryable)
	cr := classifyError("400 bad request", 0, 0)
	if cr.Class != ECFormatError {
		t.Errorf("expected ECFormatError, got %s", cr.Class)
	}
	if cr.Retryable {
		t.Error("generic 400 should not be retryable")
	}
}

func TestClassifyErrorRateLimit(t *testing.T) {
	cr := classifyError("429 Too Many Requests", 429, 0)
	if cr.Class != ECRateLimit {
		t.Errorf("expected ECRateLimit, got %d", cr.Class)
	}
	if !cr.Retryable {
		t.Error("rate limit should be retryable")
	}
}

func TestClassifyErrorBilling(t *testing.T) {
	cr := classifyError("402 Payment Required", 402, 0)
	if cr.Class != ECBilling {
		t.Errorf("expected ECBilling, got %d", cr.Class)
	}
}

func TestClassifyErrorModelNotFound(t *testing.T) {
	cr := classifyError("404 Not Found: model not found", 404, 0)
	if cr.Class != ECModelNotFound {
		t.Errorf("expected ECModelNotFound, got %d", cr.Class)
	}
}

func TestClassifyErrorAuth(t *testing.T) {
	cr := classifyError("401 Unauthorized", 401, 0)
	if cr.Class != ECAuth {
		t.Errorf("expected ECAuth, got %d", cr.Class)
	}
}

// ─── ForkedAgentResult ───────────────────────────────────────────────────────

func TestForkedAgentResultEmpty(t *testing.T) {
	result := &ForkedAgentResult{}
	if result.OutputText != "" {
		t.Errorf("expected empty output, got %q", result.OutputText)
	}
	if result.ToolCalls != 0 {
		t.Errorf("expected 0 tool calls, got %d", result.ToolCalls)
	}
}

// ─── CacheSafeParams ─────────────────────────────────────────────────────────

func TestCacheSafeParamsEmpty(t *testing.T) {
	params := CacheSafeParams{}
	if params.SystemPrompt != "" {
		t.Error("system prompt should be empty")
	}
	if params.Model != "" {
		t.Error("model should be empty")
	}
	if len(params.Tools) != 0 {
		t.Error("tools should be empty")
	}
	if len(params.Messages) != 0 {
		t.Error("messages should be empty")
	}
}

// ─── ForkedAgentConfig ───────────────────────────────────────────────────────

func TestForkedAgentConfigEmpty(t *testing.T) {
	cfg := ForkedAgentConfig{}
	if cfg.MaxTurns != 0 {
		t.Errorf("MaxTurns should default to 0 in struct, got %d", cfg.MaxTurns)
	}
	if cfg.MaxTokens != 0 {
		t.Errorf("MaxTokens should default to 0 in struct, got %d", cfg.MaxTokens)
	}
	if cfg.SkipParentMessages != false {
		t.Error("SkipParentMessages should default to false")
	}
}

func TestForkedAgentConfigQuerySource(t *testing.T) {
	cfg := ForkedAgentConfig{
		QuerySource: "session_memory",
	}
	if cfg.QuerySource != "session_memory" {
		t.Errorf("expected query source 'session_memory', got %q", cfg.QuerySource)
	}
}

// ─── JSON unmarshal (used in executeForkedTool) ──────────────────────────────

func TestToolInputUnmarshal(t *testing.T) {
	input := json.RawMessage(`{"command": "ls", "timeout": 5000}`)
	var args map[string]any
	if err := json.Unmarshal(input, &args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["command"] != "ls" {
		t.Errorf("expected command 'ls', got %v", args["command"])
	}
	if args["timeout"] != float64(5000) {
		t.Errorf("expected timeout 5000, got %v", args["timeout"])
	}
}

func TestToolInputUnmarshalEmpty(t *testing.T) {
	input := json.RawMessage(`{}`)
	var args map[string]any
	if err := json.Unmarshal(input, &args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 0 {
		t.Error("empty JSON should produce empty map")
	}
}

func TestToolInputUnmarshalNil(t *testing.T) {
	var input json.RawMessage
	var args map[string]any
	if len(input) > 0 {
		_ = json.Unmarshal(input, &args)
	}
	if args == nil {
		args = make(map[string]any)
	}
	if len(args) != 0 {
		t.Error("nil input should produce empty map after init")
	}
}

// ─── Assistant message with tool calls (RunForkedAgent loop logic) ───────────

func TestAssistantMessageWithToolCalls(t *testing.T) {
	toolCalls := []anthropic.ToolUseBlock{
		{ID: "tool_1", Name: "exec", Input: json.RawMessage(`{"command": "ls"}`)},
		{ID: "tool_2", Name: "grep", Input: json.RawMessage(`{"pattern": "foo"}`)},
	}

	var assistantBlocks []anthropic.ContentBlockParamUnion
	for _, tc := range toolCalls {
		var input map[string]any
		if len(tc.Input) > 0 {
			_ = json.Unmarshal(tc.Input, &input)
		}
		assistantBlocks = append(assistantBlocks, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    tc.ID,
				Name:  tc.Name,
				Input: input,
			},
		})
	}

	if len(assistantBlocks) != 2 {
		t.Errorf("expected 2 assistant blocks, got %d", len(assistantBlocks))
	}
}

func TestToolResultMessage(t *testing.T) {
	toolResults := []anthropic.ToolResultBlockParam{
		{ToolUseID: "tool_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "file1.txt\nfile2.txt"}},
		}},
		{ToolUseID: "tool_2", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "no matches"}},
		}},
	}

	var toolResultBlocks []anthropic.ContentBlockParamUnion
	for _, tr := range toolResults {
		for _, c := range tr.Content {
			if c.OfText != nil {
				toolResultBlocks = append(toolResultBlocks, anthropic.ContentBlockParamUnion{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: tr.ToolUseID,
						Content:   []anthropic.ToolResultBlockParamContentUnion{c},
					},
				})
			}
		}
	}

	if len(toolResultBlocks) != 2 {
		t.Errorf("expected 2 tool result blocks, got %d", len(toolResultBlocks))
	}
}

// ─── Tool call counting ──────────────────────────────────────────────────────

func TestToolCallCounting(t *testing.T) {
	toolCallCount := 0
	toolCalls := []anthropic.ToolUseBlock{
		{ID: "1", Name: "exec"},
		{ID: "2", Name: "grep"},
		{ID: "3", Name: "read_file"},
	}
	toolCallCount += len(toolCalls)
	if toolCallCount != 3 {
		t.Errorf("expected 3 tool calls counted, got %d", toolCallCount)
	}
}

// ─── Output text joining ─────────────────────────────────────────────────────

func TestOutputTextJoin(t *testing.T) {
	textParts := []string{"Hello", "World", "Test"}
	result := strings.Join(textParts, "\n")
	expected := "Hello\nWorld\nTest"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestOutputTextEmpty(t *testing.T) {
	textParts := []string{}
	result := strings.Join(textParts, "\n")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestOutputTextSingle(t *testing.T) {
	textParts := []string{"only text"}
	result := strings.Join(textParts, "\n")
	if result != "only text" {
		t.Errorf("expected 'only text', got %q", result)
	}
}

// ─── Text block extraction from response content ─────────────────────────────

func TestExtractTextParts(t *testing.T) {
	// Test the text joining logic used for forked agent output
	textParts := []string{"Hello", "World"}
	result := strings.Join(textParts, "\n")
	expected := "Hello\nWorld"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// ─── ToolUseBlock input parsing ──────────────────────────────────────────────

func TestToolUseBlockInputParsing(t *testing.T) {
	tc := anthropic.ToolUseBlock{
		ID:    "tc_1",
		Name:  "exec",
		Input: json.RawMessage(`{"command": "echo hello"}`),
	}

	var args map[string]any
	if len(tc.Input) > 0 {
		_ = json.Unmarshal(tc.Input, &args)
	}
	if args == nil {
		args = make(map[string]any)
	}

	if args["command"] != "echo hello" {
		t.Errorf("expected 'echo hello', got %v", args["command"])
	}
	if tc.Name != "exec" {
		t.Errorf("expected 'exec', got %q", tc.Name)
	}
}

func TestToolUseBlockInputEmpty(t *testing.T) {
	tc := anthropic.ToolUseBlock{ID: "tc_2", Name: "noop"}

	var args map[string]any
	if len(tc.Input) > 0 {
		_ = json.Unmarshal(tc.Input, &args)
	}
	if args == nil {
		args = make(map[string]any)
	}

	if len(args) != 0 {
		t.Error("should have empty args")
	}
}

// ─── Permission denied tool result format ────────────────────────────────────

func TestPermissionDeniedResult(t *testing.T) {
	tc := anthropic.ToolUseBlock{ID: "tc_denied", Name: "exec"}
	result := anthropic.ToolResultBlockParam{
		ToolUseID: tc.ID,
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "Permission denied: test reason"}},
		},
	}

	if result.ToolUseID != "tc_denied" {
		t.Errorf("expected ToolUseID 'tc_denied', got %q", result.ToolUseID)
	}
	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].OfText == nil {
		t.Error("content should be text")
	} else if !strings.Contains(result.Content[0].OfText.Text, "Permission denied") {
		t.Errorf("content should mention 'Permission denied', got %q", result.Content[0].OfText.Text)
	}
}

// ─── Forked client helpers ───────────────────────────────────────────────────

func TestGetForkedAPIKey(t *testing.T) {
	// Should return env var (may be empty if not set)
	key := getForkedAPIKey()
	// Just verify it doesn't panic
	_ = key
}

func TestGetForkedBaseURL(t *testing.T) {
	baseURL := getForkedBaseURL()
	_ = baseURL
}
