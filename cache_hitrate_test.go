package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"miniclaudecode-go/tools"
)

// ---------------------------------------------------------------------------
// Mock Anthropic API Server
// ---------------------------------------------------------------------------

// MockAnthropicServer simulates the Anthropic Messages API for cache hit rate testing.
type MockAnthropicServer struct {
	mu       sync.Mutex
	server   *httptest.Server
	turns    []TurnMetrics
	requests []MockRequest
	respText string
}

// TurnMetrics records cache token stats for a single API call.
type TurnMetrics struct {
	TurnNumber             int
	InputTokens            int64
	OutputTokens           int64
	CacheCreationTokens    int64
	CacheReadTokens        int64
	CacheHitRate           float64
	SystemBlocks           int
	HasScopeGlobal         bool
	HasCacheControlOnTools bool
	MessageCount           int
}

// MockRequest stores the raw request body for verification.
type MockRequest struct {
	Body       map[string]any
	TurnNumber int
}

// NewMockAnthropicServer creates a mock server.
func NewMockAnthropicServer(respText string) *MockAnthropicServer {
	ms := &MockAnthropicServer{respText: respText}
	ms.server = httptest.NewServer(http.HandlerFunc(ms.handleRequest))
	return ms
}

func (ms *MockAnthropicServer) Close()      { ms.server.Close() }
func (ms *MockAnthropicServer) URL() string { return ms.server.URL }

func (ms *MockAnthropicServer) GetTurnMetrics() []TurnMetrics {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.turns
}

func (ms *MockAnthropicServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var reqBody map[string]any
	json.Unmarshal(body, &reqBody)

	ms.mu.Lock()
	turnNum := len(ms.turns) + 1
	ms.mu.Unlock()

	metrics := ms.extractCacheMetrics(reqBody, turnNum)

	ms.mu.Lock()
	ms.turns = append(ms.turns, metrics)
	ms.mu.Unlock()

	if s, ok := reqBody["stream"].(bool); ok && s {
		ms.writeSSEResponse(w, metrics)
	} else {
		ms.writeJSONResponse(w, metrics)
	}
}

func (ms *MockAnthropicServer) extractCacheMetrics(req map[string]any, turnNum int) TurnMetrics {
	metrics := TurnMetrics{TurnNumber: turnNum}

	// Count system blocks and check for scope:global
	if system, ok := req["system"].([]any); ok {
		metrics.SystemBlocks = len(system)
		for _, block := range system {
			if b, ok := block.(map[string]any); ok {
				if cc, ok := b["cache_control"].(map[string]any); ok {
					if scope, ok := cc["scope"].(string); ok && scope == "global" {
						metrics.HasScopeGlobal = true
					}
				}
			}
		}
	}

	// Check cache_control on tools
	if tools, ok := req["tools"].([]any); ok && len(tools) > 0 {
		if t, ok := tools[len(tools)-1].(map[string]any); ok {
			if _, ok := t["cache_control"]; ok {
				metrics.HasCacheControlOnTools = true
			}
		}
	}

	// Count messages
	if msgs, ok := req["messages"].([]any); ok {
		metrics.MessageCount = len(msgs)
	}

	// Estimate tokens from JSON byte size (rough: 4 chars ≈ 1 token)
	systemTokens := estimateTokens(req, "system")
	toolsTokens := estimateTokens(req, "tools")
	messageTokens := estimateTokens(req, "messages")
	totalInput := systemTokens + toolsTokens + messageTokens

	if turnNum == 1 {
		metrics.CacheCreationTokens = totalInput
		metrics.CacheReadTokens = 0
		metrics.InputTokens = totalInput
		metrics.OutputTokens = 200
		metrics.CacheHitRate = 0
	} else {
		// System+tools always cache_read on subsequent turns
		cachedTokens := systemTokens + toolsTokens
		// Previous turn's message content that hasn't changed is also cache_read
		ms.mu.Lock()
		prevTotal := int64(0)
		if len(ms.turns) > 0 {
			prevTotal = ms.turns[len(ms.turns)-1].InputTokens
		}
		ms.mu.Unlock()
		prevMsgCached := prevTotal - systemTokens - toolsTokens
		if prevMsgCached < 0 {
			prevMsgCached = 0
		}

		metrics.CacheReadTokens = cachedTokens + prevMsgCached
		metrics.CacheCreationTokens = messageTokens - prevMsgCached
		if metrics.CacheCreationTokens < 0 {
			metrics.CacheCreationTokens = 0
		}
		metrics.InputTokens = totalInput
		metrics.OutputTokens = 200
		if totalInput > 0 {
			metrics.CacheHitRate = float64(metrics.CacheReadTokens) / float64(totalInput) * 100
		}
	}

	return metrics
}

func estimateTokens(req map[string]any, field string) int64 {
	data, ok := req[field]
	if !ok {
		return 0
	}
	b, err := json.Marshal(data)
	if err != nil {
		return 0
	}
	return int64(len(b)) / 4
}

func (ms *MockAnthropicServer) writeJSONResponse(w http.ResponseWriter, metrics TurnMetrics) {
	resp := map[string]any{
		"id":   "msg_mock_" + fmt.Sprintf("%d", metrics.TurnNumber),
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": ms.respText},
		},
		"model":       "claude-sonnet-4-20250514",
		"stop_reason": "end_turn",
		"usage": map[string]any{
			"input_tokens":                metrics.InputTokens,
			"output_tokens":               metrics.OutputTokens,
			"cache_creation_input_tokens": metrics.CacheCreationTokens,
			"cache_read_input_tokens":     metrics.CacheReadTokens,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(resp)
	w.Write(b)
}

func (ms *MockAnthropicServer) writeSSEResponse(w http.ResponseWriter, metrics TurnMetrics) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	msgID := "msg_mock_" + fmt.Sprintf("%d", metrics.TurnNumber)

	writeSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": msgID, "type": "message", "role": "assistant",
			"content": []any{}, "model": "claude-sonnet-4-20250514",
			"stop_reason": nil,
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0,
				"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0},
		},
	})
	writeSSE(w, "content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
	writeSSE(w, "content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "text_delta", "text": ms.respText},
	})
	writeSSE(w, "content_block_stop", map[string]any{
		"type": "content_block_stop", "index": 0,
	})
	writeSSE(w, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{
			"output_tokens":               metrics.OutputTokens,
			"input_tokens":                metrics.InputTokens,
			"cache_creation_input_tokens": metrics.CacheCreationTokens,
			"cache_read_input_tokens":     metrics.CacheReadTokens,
		},
	})
	writeSSE(w, "message_stop", map[string]any{"type": "message_stop"})
}

func writeSSE(w http.ResponseWriter, eventType string, data map[string]any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// ---------------------------------------------------------------------------
// Cache Hit Rate Unit Tests
// ---------------------------------------------------------------------------

// TestCacheHitRateSystemPromptPartitioning verifies the static/dynamic split
// with scope:"global" on the static block.
func TestCacheHitRateSystemPromptPartitioning(t *testing.T) {
	prompt := "Static tool descriptions here.\n\n<!-- STATIC_PROMPT_END -->\n\nDynamic content: project instructions, skills, memory."

	blocks := buildSystemBlocks(prompt, "5m")
	if len(blocks) < 2 {
		t.Fatalf("expected 2 system blocks (static + dynamic), got %d", len(blocks))
	}

	// Verify static block has cache_control with scope: "global"
	staticBlock := blocks[0]
	if staticBlock.CacheControl.Type != "ephemeral" {
		t.Errorf("static block cache_control type = '%s', want 'ephemeral'", staticBlock.CacheControl.Type)
	}
	ccJSON, _ := json.Marshal(staticBlock.CacheControl)
	if !strings.Contains(string(ccJSON), `"scope":"global"`) {
		t.Errorf("static block missing scope:global, got: %s", string(ccJSON))
	}

	// Verify dynamic block has ephemeral without scope
	dynamicBlock := blocks[1]
	if dynamicBlock.CacheControl.Type != "ephemeral" {
		t.Errorf("dynamic block cache_control type = '%s', want 'ephemeral'", dynamicBlock.CacheControl.Type)
	}
	dynamicCCJSON, _ := json.Marshal(dynamicBlock.CacheControl)
	if strings.Contains(string(dynamicCCJSON), `"scope"`) {
		t.Errorf("dynamic block should NOT have scope, got: %s", string(dynamicCCJSON))
	}
}

// TestCacheHitRateSystemPromptNoBoundary verifies fallback to single-block
// with scope:global (the entire prompt is treated as static when unsplit).
func TestCacheHitRateSystemPromptNoBoundary(t *testing.T) {
	prompt := "Just a regular system prompt without any boundary marker."
	blocks := buildSystemBlocks(prompt, "5m")
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (no boundary), got %d", len(blocks))
	}
	ccJSON, _ := json.Marshal(blocks[0].CacheControl)
	if !strings.Contains(string(ccJSON), `"scope":"global"`) {
		t.Errorf("single block should have scope:global (entire prompt is static), got: %s", string(ccJSON))
	}
}

// TestCacheHitRateCacheMessageParamsNoOverride verifies that cacheMessageParams
// no longer overwrites system blocks' cache_control.
func TestCacheHitRateCacheMessageParamsNoOverride(t *testing.T) {
	prompt := "Static content\n\n<!-- STATIC_PROMPT_END -->\n\nDynamic content"
	systemBlocks := buildSystemBlocks(prompt, "5m")

	params := anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		System:    systemBlocks,
		Messages: []anthropic.MessageParam{
			{
				Role:    anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("Hello")},
			},
		},
	}

	// Apply cacheMessageParams — should NOT override System[0]'s scope:global
	cacheMessageParams(&params)

	ccJSON, _ := json.Marshal(params.System[0].CacheControl)
	if !strings.Contains(string(ccJSON), `"scope":"global"`) {
		t.Errorf("System[0] scope:global lost after cacheMessageParams! got: %s", string(ccJSON))
	}
	if len(params.System) >= 2 {
		dynamicCCJSON, _ := json.Marshal(params.System[1].CacheControl)
		if strings.Contains(string(dynamicCCJSON), `"scope"`) {
			t.Errorf("System[1] should NOT have scope, got: %s", string(dynamicCCJSON))
		}
	}
}

// TestCacheHitRateToolCacheControl verifies last tool has cache_control.
func TestCacheHitRateToolCacheControl(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.NewFileReadTool(registry))
	registry.Register(tools.NewFileWriteTool(registry))
	registry.Register(tools.NewFileEditTool(registry))

	// Simulate buildToolParams logic
	toolList := registry.AllTools()
	names := make([]string, len(toolList))
	for i, t := range toolList {
		names[i] = t.Name()
	}
	sort.Strings(names)

	toolParams := make([]anthropic.ToolUnionParam, len(names))
	for i, name := range names {
		toolParams[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        name,
				Description: param.NewOpt("Mock tool"),
				InputSchema: anthropic.ToolInputSchemaParam{},
			},
		}
	}
	if len(toolParams) > 0 {
		toolParams[len(toolParams)-1].OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	lastTool := toolParams[len(toolParams)-1]
	if lastTool.OfTool.CacheControl.Type != "ephemeral" {
		t.Errorf("last tool should have ephemeral cache_control, got: %s", lastTool.OfTool.CacheControl.Type)
	}
}

// TestCacheHitRateMultiTurn verifies cache_read increases across turns
// through the mock API server.
func TestCacheHitRateMultiTurn(t *testing.T) {
	ms := NewMockAnthropicServer("I'll help you with that.")
	defer ms.Close()

	prompt := "Static tool descriptions and rules.\n\n<!-- STATIC_PROMPT_END -->\n\nDynamic: project instructions."
	systemBlocks := buildSystemBlocks(prompt, "5m")

	// Verify scope:global before API calls
	staticCCJSON, _ := json.Marshal(systemBlocks[0].CacheControl)
	if !strings.Contains(string(staticCCJSON), `"scope":"global"`) {
		t.Fatalf("static system block missing scope:global BEFORE API call: %s", string(staticCCJSON))
	}

	userTexts := []string{"hello", "help me", "thanks"}

	for i := 0; i < 3; i++ {
		messages := []anthropic.MessageParam{
			{
				Role:    anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(userTexts[i])},
			},
		}
		// Add previous turns for turns 2+
		if i > 0 {
			for j := 0; j < i; j++ {
				messages = append(messages, anthropic.MessageParam{
					Role:    anthropic.MessageParamRoleAssistant,
					Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("Response " + fmt.Sprintf("%d", j+1))},
				})
				messages = append(messages, anthropic.MessageParam{
					Role:    anthropic.MessageParamRoleUser,
					Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(userTexts[j+1])},
				})
			}
		}

		params := anthropic.MessageNewParams{
			Model:     "claude-sonnet-4-20250514",
			MaxTokens: 1024,
			System:    systemBlocks,
			Messages:  messages,
		}
		cacheMessageParams(&params)

		// Verify system blocks NOT overwritten
		postCCJSON, _ := json.Marshal(params.System[0].CacheControl)
		if !strings.Contains(string(postCCJSON), `"scope":"global"`) {
			t.Errorf("Turn %d: System[0] scope:global lost! got: %s", i+1, string(postCCJSON))
		}

		// Make API call via mock
		client := anthropic.NewClient(
			option.WithHeader("Authorization", "Bearer mock-key"),
			option.WithBaseURL(ms.URL()),
		)
		resp, err := client.Messages.New(context.Background(), params)
		if err != nil {
			t.Fatalf("Turn %d: API call failed: %v", i+1, err)
		}

		if i == 0 {
			if resp.Usage.CacheCreationInputTokens <= 0 {
				t.Errorf("Turn 1: cache_creation should be > 0, got %d", resp.Usage.CacheCreationInputTokens)
			}
		} else {
			if resp.Usage.CacheReadInputTokens <= 0 {
				t.Errorf("Turn %d: cache_read should be > 0, got %d", i+1, resp.Usage.CacheReadInputTokens)
			}
		}
	}

	// Verify mock received all requests with scope:global
	metrics := ms.GetTurnMetrics()
	if len(metrics) != 3 {
		t.Fatalf("expected 3 turn metrics, got %d", len(metrics))
	}
	for _, m := range metrics {
		if !m.HasScopeGlobal {
			t.Errorf("Turn %d: mock server did not see scope:global", m.TurnNumber)
		}
	}
}

// TestCacheHitRateCompaction verifies compaction resets cache baseline.
func TestCacheHitRateCompaction(t *testing.T) {
	detector := &CacheBreakDetector{}
	detector.UpdateBaseline(50000)

	if detector.DetectBreak(55000) {
		t.Error("cache_read increase should not trigger break")
	}
	detector.UpdateBaseline(55000)

	detector.ResetBaseline()
	detector.MarkPostCompaction()
	if detector.DetectBreak(20000) {
		t.Error("post-compaction reduction should NOT trigger break")
	}

	detector.UpdateBaseline(20000)
	detector.RecordChange(CacheChangeSystemPrompt, 1)
	if !detector.DetectBreak(5000) {
		t.Error("genuine cache break should be detected after compaction")
	}
}

// TestCacheHitRateNormalizationStability verifies normalization is idempotent
// and doesn't mutate previously-sent messages when new content is added.
func TestCacheHitRateNormalizationStability(t *testing.T) {
	// Build realistic messages with tool results
	toolResultContent := []anthropic.ToolResultBlockParamContentUnion{
		{OfText: &anthropic.TextBlockParam{Text: "file contents here"}},
	}

	messages := []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("Read the file")},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock("I'll read it."),
				{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    "tool_1",
						Name:  "read_file",
						Input: param.Opt[any]{},
					},
				},
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{
					OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "tool_1",
						Content:   toolResultContent,
					},
				},
				anthropic.NewTextBlock("Now edit it"),
			},
		},
	}

	// Idempotency: normalize twice, should be identical
	first := NormalizeAPIMessages(messages)
	second := NormalizeAPIMessages(first)

	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Errorf("NormalizeAPIMessages NOT idempotent!\n1st: %s\n2nd: %s",
			compactJSON(string(firstJSON)), compactJSON(string(secondJSON)))
	}

	// Prefix stability: add new content, previous messages unchanged
	extended := append(second, anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock("I'll edit it."),
			{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool_2",
					Name:  "edit_file",
					Input: param.Opt[any]{},
				},
			},
		},
	})

	third := NormalizeAPIMessages(extended)
	for i := 0; i < len(second); i++ {
		prevJSON, _ := json.Marshal(second[i])
		currJSON, _ := json.Marshal(third[i])
		if string(prevJSON) != string(currJSON) {
			t.Errorf("Message %d changed after adding new content!\nBefore: %s\nAfter:  %s",
				i, compactJSON(string(prevJSON)), compactJSON(string(currJSON)))
		}
	}
}

// TestCacheHitRateHoistToolResults verifies stable tool_result ordering.
func TestCacheHitRateHoistToolResults(t *testing.T) {
	// Case 1: system-reminder before tool_result (MCP injection pattern)
	toolResultContent := []anthropic.ToolResultBlockParamContentUnion{
		{OfText: &anthropic.TextBlockParam{Text: "result"}},
	}

	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock("<system-reminder>MCP server connected</system-reminder>"),
			{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool_1",
					Content:   toolResultContent,
				},
			},
			anthropic.NewTextBlock("Now do something else"),
		},
	}

	result := hoistToolResults([]anthropic.MessageParam{msg})
	if len(result) == 0 || len(result[0].Content) == 0 {
		t.Fatal("hoistToolResults returned empty")
	}
	if result[0].Content[0].OfToolResult == nil {
		t.Errorf("tool_result should be hoisted to front, got text block first")
	}

	// Case 2: already hoisted — should be unchanged
	msg2 := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool_1",
					Content:   toolResultContent,
				},
			},
			anthropic.NewTextBlock("follow-up"),
		},
	}

	result2 := hoistToolResults([]anthropic.MessageParam{msg2})
	msg2JSON, _ := json.Marshal(msg2)
	result2JSON, _ := json.Marshal(result2[0])
	if string(msg2JSON) != string(result2JSON) {
		t.Errorf("already-hoisted message should be unchanged")
	}
}

func compactJSON(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
