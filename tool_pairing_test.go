package main

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestToolUseToolResultSerialization(t *testing.T) {
	// Test that tool_use and tool_result blocks serialize correctly
	toolUse := anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:    "call_function_test_1",
			Name:  "write_file",
			Input: map[string]any{"path": "test.txt", "content": "hello"},
		},
	}

	toolResult := anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: "call_function_test_1",
			Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "ok"}}},
			IsError:   anthropic.Bool(false),
		},
	}

	// Serialize tool_use
	toolUseJSON, err := json.Marshal(toolUse)
	if err != nil {
		t.Fatalf("ToolUse marshal error: %v", err)
	}
	t.Logf("ToolUse: %s", toolUseJSON)

	// Verify tool_use has type, id, name fields
	var toolUseMap map[string]any
	json.Unmarshal(toolUseJSON, &toolUseMap)
	if toolUseMap["type"] != "tool_use" {
		t.Errorf("Expected type=tool_use, got %v", toolUseMap["type"])
	}
	if toolUseMap["id"] != "call_function_test_1" {
		t.Errorf("Expected id=call_function_test_1, got %v", toolUseMap["id"])
	}

	// Serialize tool_result
	toolResultJSON, err := json.Marshal(toolResult)
	if err != nil {
		t.Fatalf("ToolResult marshal error: %v", err)
	}
	t.Logf("ToolResult: %s", toolResultJSON)

	// Verify tool_result has type, tool_use_id fields
	var toolResultMap map[string]any
	json.Unmarshal(toolResultJSON, &toolResultMap)
	if toolResultMap["type"] != "tool_result" {
		t.Errorf("Expected type=tool_result, got %v", toolResultMap["type"])
	}
	if toolResultMap["tool_use_id"] != "call_function_test_1" {
		t.Errorf("Expected tool_use_id=call_function_test_1, got %v", toolResultMap["tool_use_id"])
	}
}

func TestFullConversationSerialization(t *testing.T) {
	// Simulate the full conversation flow
	assistantMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    "call_function_test_1",
				Name:  "write_file",
				Input: map[string]any{"path": "test.txt", "content": "hello"},
			}},
		},
	}

	userMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: "call_function_test_1",
				Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "ok"}}},
				IsError:   anthropic.Bool(false),
			}},
		},
	}

	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "Write a file"}},
		}},
		assistantMsg,
		userMsg,
	}

	convJSON, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("Conversation marshal error: %v", err)
	}
	t.Logf("Conversation: %s", convJSON)

	// Verify pairing
	var parsed []map[string]any
	json.Unmarshal(convJSON, &parsed)

	toolUseIDs := map[string]bool{}
	toolResultIDs := map[string]bool{}

	for _, msg := range parsed {
		content, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		for _, block := range content {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			if blockMap["type"] == "tool_use" {
				toolUseIDs[blockMap["id"].(string)] = true
			}
			if blockMap["type"] == "tool_result" {
				toolResultIDs[blockMap["tool_use_id"].(string)] = true
			}
		}
	}

	for id := range toolResultIDs {
		if !toolUseIDs[id] {
			t.Errorf("tool_result references tool_use_id '%s' NOT FOUND in messages!", id)
		}
	}
}

func TestNormalizeAPIMessagesPreservesToolPairing(t *testing.T) {
	// Test that NormalizeAPIMessages doesn't break tool pairing
	assistantMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "I'll write a file."}},
			{OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    "call_function_test_1",
				Name:  "write_file",
				Input: map[string]any{"path": "test.txt", "content": "hello"},
			}},
		},
	}

	userMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: "call_function_test_1",
				Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "ok"}}},
				IsError:   anthropic.Bool(false),
			}},
		},
	}

	// Add a follow-up user message (common pattern)
	followUpMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "Now read it back"}},
		},
	}

	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "Write a file"}},
		}},
		assistantMsg,
		userMsg,
		followUpMsg,
	}

	// Apply normalization
	normalized := NormalizeAPIMessages(messages)

	// Verify pairing still intact
	toolUseIDs := map[string]bool{}
	toolResultIDs := map[string]bool{}

	for _, msg := range normalized {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			for _, block := range msg.Content {
				if block.OfToolUse != nil && block.OfToolUse.ID != "" {
					toolUseIDs[block.OfToolUse.ID] = true
				}
			}
		}
		if msg.Role == anthropic.MessageParamRoleUser {
			for _, block := range msg.Content {
				if block.OfToolResult != nil && block.OfToolResult.ToolUseID != "" {
					toolResultIDs[block.OfToolResult.ToolUseID] = true
				}
			}
		}
	}

	for id := range toolResultIDs {
		if !toolUseIDs[id] {
			t.Errorf("After normalization: tool_result references tool_use_id '%s' NOT FOUND!", id)
		} else {
			t.Logf("OK: tool_result for '%s' has matching tool_use after normalization", id)
		}
	}

	// Also verify the final JSON
	finalJSON, _ := json.Marshal(normalized)
	t.Logf("Normalized JSON: %s", finalJSON)
}
