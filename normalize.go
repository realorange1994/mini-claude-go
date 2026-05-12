// Package main provides API message normalization for KV cache reuse (Hermes-style).
//
// Normalizes API messages before sending to improve Anthropic prefix cache hit rate:
// 1. Enforce role alternation (merge consecutive same-role messages)
// 2. Ensure tool_use/tool_result pairing (fix orphans)
// 3. Filter empty messages (remove whitespace-only assistant messages)
// 4. Sort JSON keys in tool_call input by alphabetical order
// 5. Normalize whitespace in tool_result content (collapse multiple blank lines)
// 6. These normalizations make identical logical content produce identical API payloads,
//    which is critical for Anthropic's prefix caching to work effectively.
package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// NormalizeAPIMessages normalizes a list of API messages for KV cache reuse.
// Returns a new slice with normalized messages.
// The normalization order matters:
//  1. EnforceRoleAlternation (establishes correct message order)
//  2. EnsureToolResultPairing (fixes tool pairs)
//  3. FilterEmptyMessages (cleans up empties)
//  4. Existing normalizations (sort keys, whitespace)
func NormalizeAPIMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	messages = EnforceRoleAlternation(messages)
	messages = EnsureToolResultPairing(messages)
	messages = FilterEmptyMessages(messages)

	// Existing normalizations (sort keys, whitespace)
	result := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		result[i] = normalizeMessage(msg)
	}
	return result
}

// ============================================================================
// P0-2: Role Alternation Enforcement
// ============================================================================

// EnforceRoleAlternation ensures messages alternate between user and assistant roles.
// Consecutive same-role messages are merged. If the first message is from the assistant,
// a synthetic user message is prepended.
func EnforceRoleAlternation(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	// If the first message is from assistant, prepend a synthetic user message.
	if messages[0].Role == anthropic.MessageParamRoleAssistant {
		syntheticUser := anthropic.MessageParam{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{
					OfText: &anthropic.TextBlockParam{
						Text: "[System: conversation starts with assistant response]",
					},
				},
			},
		}
		messages = append([]anthropic.MessageParam{syntheticUser}, messages...)
	}

	var result []anthropic.MessageParam
	for _, msg := range messages {
		if len(result) == 0 {
			result = append(result, msg)
			continue
		}

		last := &result[len(result)-1]
		if msg.Role == last.Role {
			// Merge consecutive same-role messages by combining content blocks.
			last.Content = append(last.Content, msg.Content...)
		} else {
			result = append(result, msg)
		}
	}

	return result
}

// ============================================================================
// P0-1: Tool Pairing Validation
// ============================================================================

// EnsureToolResultPairing ensures every tool_use has a matching tool_result and vice versa.
// - Forward pass: insert synthetic error tool_result for orphaned tool_use blocks.
// - Reverse pass: strip tool_result blocks whose tool_use_id doesn't match any tool_use.
// - Cross-message dedup: skip duplicate tool_use IDs.
func EnsureToolResultPairing(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	// Collect all tool_use IDs from assistant messages, tracking duplicates.
	allToolUseIDs := make(map[string]bool) // id -> seen
	for _, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				id := block.OfToolUse.ID
				if id != "" {
					allToolUseIDs[id] = true
				}
			}
		}
	}

	// Collect all tool_result IDs from user messages.
	allToolResultIDs := make(map[string]bool) // id -> seen
	for _, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleUser {
			continue
		}
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				id := block.OfToolResult.ToolUseID
				if id != "" {
					allToolResultIDs[id] = true
				}
			}
		}
	}

	// Forward pass: for each tool_use without a matching tool_result,
	// insert a synthetic error tool_result.
	// Also dedup duplicate tool_use IDs across messages.
	result := make([]anthropic.MessageParam, 0, len(messages))
	seenToolUseIDs := make(map[string]bool)

	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			// Dedup tool_use blocks and collect orphaned IDs.
			var newContent []anthropic.ContentBlockParamUnion
			var orphanedToolUseIDs []string

			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					id := block.OfToolUse.ID
					if id != "" && seenToolUseIDs[id] {
						// Duplicate tool_use ID — skip it
						continue
					}
					if id != "" {
						seenToolUseIDs[id] = true
					}
					// Check if this tool_use has a matching tool_result
					if id != "" && !allToolResultIDs[id] {
						orphanedToolUseIDs = append(orphanedToolUseIDs, id)
					}
					newContent = append(newContent, block)
				} else {
					newContent = append(newContent, block)
				}
			}

			msg.Content = newContent
			result = append(result, msg)

			// Insert synthetic tool_results for orphaned tool_uses.
			// The tool_result must be in a user message following the assistant message.
			if len(orphanedToolUseIDs) > 0 {
				var syntheticBlocks []anthropic.ContentBlockParamUnion
				for _, id := range orphanedToolUseIDs {
					syntheticBlocks = append(syntheticBlocks, anthropic.ContentBlockParamUnion{
						OfToolResult: &anthropic.ToolResultBlockParam{
							ToolUseID: id,
							IsError:   param.NewOpt(true),
							Content: []anthropic.ToolResultBlockParamContentUnion{
								{
									OfText: &anthropic.TextBlockParam{
										Text: "Tool execution was interrupted",
									},
								},
							},
						},
					})
				}
				result = append(result, anthropic.MessageParam{
					Role:    anthropic.MessageParamRoleUser,
					Content: syntheticBlocks,
				})
			}
		} else if msg.Role == anthropic.MessageParamRoleUser {
			result = append(result, msg)
		} else {
			result = append(result, msg)
		}
	}

	// Reverse pass: strip tool_result blocks whose tool_use_id doesn't match
	// any tool_use in the messages. Also dedup duplicate tool_result IDs.
	seenToolResultIDs := make(map[string]bool)
	for i := range result {
		if result[i].Role != anthropic.MessageParamRoleUser {
			continue
		}
		var filtered []anthropic.ContentBlockParamUnion
		for _, block := range result[i].Content {
			if block.OfToolResult != nil {
				id := block.OfToolResult.ToolUseID
				if id == "" {
					filtered = append(filtered, block)
					continue
				}
				// Skip if no matching tool_use exists
				if !allToolUseIDs[id] {
					continue
				}
				// Skip duplicate tool_result IDs
				if seenToolResultIDs[id] {
					continue
				}
				seenToolResultIDs[id] = true
				filtered = append(filtered, block)
			} else {
				filtered = append(filtered, block)
			}
		}
		result[i].Content = filtered
	}

	return result
}

// ============================================================================
// P0-3: Empty Message Filtering
// ============================================================================

// FilterEmptyMessages removes or fixes messages that would cause API 400 errors:
// - Whitespace-only assistant messages are removed.
// - Assistant messages with only empty content blocks get a placeholder.
// - Orphaned thinking-only messages are removed.
// - Trailing thinking blocks are removed from the last assistant message.
func FilterEmptyMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	// Filter whitespace-only assistant messages and fix empty content.
	var result []anthropic.MessageParam
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			if isWhitespaceOnlyAssistant(msg) {
				continue // drop whitespace-only assistant message
			}

			// Ensure assistant message has non-empty content.
			// If all content blocks are empty/whitespace text, insert a placeholder.
			if hasOnlyEmptyContent(msg) {
				msg.Content = []anthropic.ContentBlockParamUnion{
					{
						OfText: &anthropic.TextBlockParam{
							Text: "[thinking...]",
						},
					},
				}
			}

			// Filter orphaned thinking-only messages:
			// assistant messages with only thinking blocks and no other content.
			if isThinkingOnlyAssistant(msg) {
				continue
			}

			result = append(result, msg)
		} else {
			result = append(result, msg)
		}
	}

	// Remove trailing thinking blocks from the last assistant message.
	if len(result) > 0 {
		lastIdx := len(result) - 1
		// Find the last assistant message
		for lastIdx >= 0 && result[lastIdx].Role != anthropic.MessageParamRoleAssistant {
			lastIdx--
		}
		if lastIdx >= 0 {
			result[lastIdx] = stripTrailingThinking(result[lastIdx])
		}
	}

	return result
}

// isWhitespaceOnlyAssistant returns true if an assistant message contains only
// whitespace-only text blocks (no tool_use, no thinking, etc).
func isWhitespaceOnlyAssistant(msg anthropic.MessageParam) bool {
	if msg.Role != anthropic.MessageParamRoleAssistant {
		return false
	}
	if len(msg.Content) == 0 {
		return true
	}
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			return false
		}
		if block.OfThinking != nil {
			return false
		}
		if block.OfRedactedThinking != nil {
			return false
		}
		if block.OfText != nil {
			if strings.TrimSpace(block.OfText.Text) != "" {
				return false
			}
		}
		// Any other block type means non-whitespace
		if block.OfText == nil && block.OfToolUse == nil &&
			block.OfThinking == nil && block.OfRedactedThinking == nil {
			// Other block types (image, document, etc.) — treat as non-whitespace
			return false
		}
	}
	return true
}

// hasOnlyEmptyContent returns true if an assistant message has only empty/whitespace
// text blocks (possibly mixed with thinking blocks), and no substantive content.
func hasOnlyEmptyContent(msg anthropic.MessageParam) bool {
	if msg.Role != anthropic.MessageParamRoleAssistant {
		return false
	}
	if len(msg.Content) == 0 {
		return true
	}
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			return false
		}
		if block.OfText != nil && strings.TrimSpace(block.OfText.Text) != "" {
			return false
		}
		// thinking/redacted_thinking blocks don't count as "content" for this check
		// Other block types (image, document, etc.) count as content
		if block.OfText == nil && block.OfToolUse == nil &&
			block.OfThinking == nil && block.OfRedactedThinking == nil {
			return false
		}
	}
	return true
}

// isThinkingOnlyAssistant returns true if an assistant message contains only
// thinking and/or redacted_thinking blocks with no other substantive content.
func isThinkingOnlyAssistant(msg anthropic.MessageParam) bool {
	if msg.Role != anthropic.MessageParamRoleAssistant {
		return false
	}
	if len(msg.Content) == 0 {
		return false // empty is not "thinking only", it's just empty
	}
	for _, block := range msg.Content {
		if block.OfThinking != nil || block.OfRedactedThinking != nil {
			continue
		}
		// Any non-thinking block means it's not thinking-only
		return false
	}
	return true
}

// stripTrailingThinking removes trailing thinking/redacted_thinking blocks
// from an assistant message. These can cause issues if they're the last
// content in the last assistant message.
func stripTrailingThinking(msg anthropic.MessageParam) anthropic.MessageParam {
	if msg.Role != anthropic.MessageParamRoleAssistant {
		return msg
	}
	if len(msg.Content) == 0 {
		return msg
	}

	// Find the last non-thinking block
	lastNonThinking := -1
	for i, block := range msg.Content {
		if block.OfThinking == nil && block.OfRedactedThinking == nil {
			lastNonThinking = i
		}
	}

	if lastNonThinking < len(msg.Content)-1 {
		// There are trailing thinking blocks — strip them
		result := msg
		result.Content = msg.Content[:lastNonThinking+1]
		return result
	}

	return msg
}

// ============================================================================
// Existing Normalizations (sort keys, whitespace)
// ============================================================================

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
