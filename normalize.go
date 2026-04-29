// Package main provides API message normalization for KV cache reuse (Hermes-style).
//
// Normalizes API messages before sending to improve Anthropic prefix cache hit rate:
// 1. Sort JSON keys in tool_call input by alphabetical order
// 2. Normalize whitespace in tool_result content (collapse multiple blank lines)
// 3. These normalizations make identical logical content produce identical API payloads,
//    which is critical for Anthropic's prefix caching to work effectively.
package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// NormalizeAPIMessages normalizes a list of API messages for KV cache reuse.
// Returns a new slice with normalized messages.
func NormalizeAPIMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		result[i] = normalizeMessage(msg)
	}
	return result
}

// normalizeMessage normalizes a single API message.
func normalizeMessage(msg anthropic.MessageParam) anthropic.MessageParam {
	switch msg.Role {
	case anthropic.MessageParamRoleAssistant:
		return normalizeAssistantMessage(msg)
	case anthropic.MessageParamRoleUser:
		return normalizeUserMessage(msg)
	default:
		return msg
	}
}

// normalizeAssistantMessage normalizes tool_use blocks: sort input JSON keys.
func normalizeAssistantMessage(msg anthropic.MessageParam) anthropic.MessageParam {
	hasToolUse := false
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			hasToolUse = true
			break
		}
	}
	if !hasToolUse {
		return msg
	}

	result := msg
	result.Content = make([]anthropic.ContentBlockParamUnion, len(msg.Content))
	for i, block := range msg.Content {
		if block.OfToolUse != nil {
			result.Content[i] = normalizeToolUseBlock(block)
		} else {
			result.Content[i] = block
		}
	}
	return result
}

// normalizeUserMessage normalizes tool_result content: collapse whitespace.
func normalizeUserMessage(msg anthropic.MessageParam) anthropic.MessageParam {
	hasToolResult := false
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			hasToolResult = true
			break
		}
	}
	if !hasToolResult {
		return msg
	}

	result := msg
	result.Content = make([]anthropic.ContentBlockParamUnion, len(msg.Content))
	for i, block := range msg.Content {
		if block.OfToolResult != nil {
			result.Content[i] = normalizeToolResultBlock(block)
		} else {
			result.Content[i] = block
		}
	}
	return result
}

// normalizeToolUseBlock sorts input JSON keys alphabetically.
func normalizeToolUseBlock(block anthropic.ContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	if block.OfToolUse == nil || block.OfToolUse.Input == nil {
		return block
	}

	inputMap, ok := block.OfToolUse.Input.(map[string]any)
	if !ok {
		return block
	}

	result := block
	result.OfToolUse = &anthropic.ToolUseBlockParam{
		ID:    block.OfToolUse.ID,
		Name:  block.OfToolUse.Name,
		Input: sortMapKeys(inputMap),
	}
	return result
}

// normalizeToolResultBlock normalizes whitespace in tool_result text content.
func normalizeToolResultBlock(block anthropic.ContentBlockParamUnion) anthropic.ContentBlockParamUnion {
	if block.OfToolResult == nil {
		return block
	}

	result := block
	result.OfToolResult = &anthropic.ToolResultBlockParam{
		ToolUseID: block.OfToolResult.ToolUseID,
		IsError:   block.OfToolResult.IsError,
	}

	newContent := make([]anthropic.ToolResultBlockParamContentUnion, len(block.OfToolResult.Content))
	for i, c := range block.OfToolResult.Content {
		if c.OfText != nil {
			newContent[i] = anthropic.ToolResultBlockParamContentUnion{
				OfText: &anthropic.TextBlockParam{
					Text: normalizeWhitespace(c.OfText.Text),
				},
			}
		} else {
			newContent[i] = c
		}
	}
	result.OfToolResult.Content = newContent
	return result
}

// sortMapKeys recursively sorts JSON object keys alphabetically.
func sortMapKeys(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make(map[string]any, len(m))
	for _, k := range keys {
		result[k] = sortValueKeys(m[k])
	}
	return result
}

// sortValueKeys recursively sorts value keys.
func sortValueKeys(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return sortMapKeys(val)
	case []any:
		result := make([]any, len(val))
		for i, elem := range val {
			result[i] = sortValueKeys(elem)
		}
		return result
	default:
		return v
	}
}

// normalizeWhitespace collapses 3+ consecutive blank lines into 1,
// trims trailing whitespace from lines, and removes trailing blank lines.
func normalizeWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	consecutiveBlank := 0

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")

		if trimmed == "" {
			consecutiveBlank++
			if consecutiveBlank <= 1 {
				result = append(result, trimmed)
			}
			// Skip 2nd+ consecutive blank line (keep at most 1 blank line)
		} else {
			consecutiveBlank = 0
			result = append(result, trimmed)
		}
	}

	// Remove trailing blank lines
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	return strings.Join(result, "\n")
}

// NormalizeJSONBytes sorts JSON object keys in a byte slice.
// Useful for normalizing raw JSON before sending to API.
func NormalizeJSONBytes(data []byte) []byte {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return data
	}
	normalized := sortValueKeys(v)
	result, err := json.Marshal(normalized)
	if err != nil {
		return data
	}
	return result
}
