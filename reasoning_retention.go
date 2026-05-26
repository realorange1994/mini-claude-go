package main

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// reasoningRetention strips reasoning_content from assistant messages
// that don't have tool_calls. This reduces request size and improves cache
// hit rate by removing stale reasoning from old turns.
//
// Matching DeepSeek-Reasonix's reasoning-retention.ts pattern:
// - Keeps tool-call reasoning (DeepSeek requires it for validation)
// - Only strips from assistant messages BEFORE the last user message
func reasoningRetention(messages []anthropic.MessageParam) (int, int) {
	lastUser := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == anthropic.MessageParamRoleUser {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return 0, 0
	}

	prunedCount := 0
	charsDropped := 0

	for i := 0; i < len(messages); i++ {
		msg := &messages[i]
		if msg.Role != anthropic.MessageParamRoleAssistant || i > lastUser {
			continue
		}

		// Check for tool_calls
		hasToolCalls := false
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				hasToolCalls = true
				break
			}
		}

		// Check for reasoning_content - use ContentAsMap to check
		// ReasoningContent is stored in the thinking block
		if len(msg.Content) == 0 {
			continue
		}

		// If no tool_calls and message has thinking blocks, strip them
		if !hasToolCalls {
			// Count chars we're dropping
			for j := range msg.Content {
				block := &msg.Content[j]
				if block.OfThinking != nil && block.OfThinking.Thinking != "" {
					charsDropped += len(block.OfThinking.Thinking)
				}
			}
			// Strip all thinking blocks
			newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
			for j := range msg.Content {
				block := &msg.Content[j]
				if block.OfThinking == nil {
					newContent = append(newContent, *block)
				}
			}
			if len(newContent) < len(msg.Content) {
				msg.Content = newContent
				prunedCount++
			}
		}
	}

	return prunedCount, charsDropped
}

// thinkingModeStamping ensures all assistant messages have a thinking block
// when in thinking mode. DeepSeek returns 400 error if thinking/reasoning
// is missing on a response that previously had it.
//
// Matching DeepSeek-Reasonix's healing.ts stampMissingReasoningForThinkingMode
func thinkingModeStamping(messages []anthropic.MessageParam, isThinkingMode bool) int {
	if !isThinkingMode {
		return 0
	}

	stampedCount := 0
	for i := range messages {
		msg := &messages[i]
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		// Already has thinking block - skip
		hasThinking := false
		for _, block := range msg.Content {
			if block.OfThinking != nil {
				hasThinking = true
				break
			}
		}

		if hasThinking {
			continue
		}

		// Add empty thinking block
		msg.Content = append([]anthropic.ContentBlockParamUnion{
			anthropic.ContentBlockParamUnion{OfThinking: &anthropic.ThinkingBlockParam{
				Thinking: "",
			}},
		}, msg.Content...)
		stampedCount++
	}

	return stampedCount
}

// stampMissingToolCallIDs adds missing tool_use_id to tool_calls that don't have one.
// DeepSeek returns 400 error on tool_calls without id field.
//
// Matching DeepSeek-Reasonix's healing.ts stampMissingIds
func stampMissingToolCallIDs(messages []anthropic.MessageParam) int {
	stampedCount := 0
	seq := 0

	for i := range messages {
		msg := &messages[i]
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfToolUse == nil {
				continue
			}
			if block.OfToolUse.ID != "" {
				continue
			}
			// Add synthetic id
			block.OfToolUse.ID = fmt.Sprintf("z-ext-%d", seq)
			seq++
			stampedCount++
		}
	}

	return stampedCount
}

// shrinkToolCallArgsByTokens shrinks oversized tool call argument JSON by
// replacing long string values (>300 chars) with placeholder text, while
// keeping short keys/values (paths, IDs) verbatim.
//
// Matching DeepSeek-Reasonix's shrink.ts shrinkOversizedToolCallArgsByTokens
func shrinkToolCallArgsByTokens(messages []anthropic.MessageParam, maxTokenChars int) (int, int) {
	const longThreshold = 300
	healedCount := 0
	charsSaved := 0

	for i := range messages {
		msg := &messages[i]
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}

		for j := range msg.Content {
			block := &msg.Content[j]
			if block.OfToolUse == nil {
				continue
			}

			// Get function.arguments
			if block.OfToolUse.Input == nil {
				continue
			}

			argsBytes, ok := block.OfToolUse.Input.([]byte)
			if !ok {
				argsStr, isStr := block.OfToolUse.Input.(string)
				if !isStr || len(argsStr) <= maxTokenChars {
					continue
				}
				argsBytes = []byte(argsStr)
			}
			if len(argsBytes) <= maxTokenChars {
				continue
			}

			// Shrink long strings in the JSON args
			shrunk, saved := shrinkJSONLongStrings(string(argsBytes), longThreshold)
			if saved > 0 {
				block.OfToolUse.Input = []byte(shrunk)
				healedCount++
				charsSaved += saved
			}
		}
	}

	return healedCount, charsSaved
}

// shrinkJSONLongStrings replaces strings longer than threshold with placeholders.
func shrinkJSONLongStrings(jsonStr string, threshold int) (string, int) {
	var parsed any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return jsonStr, 0
	}

	obj, ok := parsed.(map[string]any)
	if !ok {
		return jsonStr, 0
	}

	output := make(map[string]any)
	saved := 0

	for k, v := range obj {
		if str, isString := v.(string); isString && len(str) > threshold {
			newlines := 0
			for _, c := range str {
				if c == '\n' {
					newlines++
				}
			}
			placeholder := "[...shrunk: " + itoa(len(str)) + " chars, " + itoa(newlines) + " lines - tool already responded, see result]"
			output[k] = placeholder
			saved += len(str) - len(placeholder)
		} else {
			output[k] = v
		}
	}

	result, _ := json.Marshal(output)
	return string(result), saved
}

// itoa is defined in json_repair.go