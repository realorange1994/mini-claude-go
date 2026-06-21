package main

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ─── Round Management (extracted from compact.go) ───────────────────────────

// apiRound represents a single API request-response round.
type apiRound struct {
	messages   []CompactionMessage
	isToolCall bool
	toolPairID string
	ToolName   string
}

// groupMessagesByRound groups consecutive user/assistant message pairs into rounds.
func groupMessagesByRound(messages []CompactionMessage) []apiRound {
	if len(messages) == 0 {
		return nil
	}

	var rounds []apiRound
	var currentRound []CompactionMessage

	for i, msg := range messages {
		// Start new round on user message if current round already has a user
		if msg.Role == "user" && len(currentRound) > 0 && hasUserMessage(currentRound) {
			rounds = append(rounds, apiRound{
				messages:   currentRound,
				isToolCall: hasToolCallMessages(currentRound),
			})
			currentRound = nil
		}

		currentRound = append(currentRound, msg)

		// Check if we should end the round
		isLast := i == len(messages)-1
		nextIsUser := !isLast && messages[i+1].Role == "user"

		// End round on assistant message if it's the last or next is user
		if msg.Role == "assistant" && (isLast || nextIsUser) {
			rounds = append(rounds, apiRound{
				messages:   currentRound,
				isToolCall: hasToolCallMessages(currentRound),
			})
			currentRound = nil
		}
	}

	// Handle remaining messages
	if len(currentRound) > 0 {
		rounds = append(rounds, apiRound{
			messages: currentRound,
		})
	}

	return rounds
}

// hasUserMessage checks if any message in the list is a user message.
func hasUserMessage(messages []CompactionMessage) bool {
	for _, msg := range messages {
		if msg.Role == "user" {
			return true
		}
	}
	return false
}

// hasToolCallMessages checks if any message in the list contains tool calls.
func hasToolCallMessages(messages []CompactionMessage) bool {
	for _, msg := range messages {
		if hasToolCalls(msg) {
			return true
		}
	}
	return false
}

// hasToolCalls checks if a message contains tool calls.
func hasToolCalls(msg CompactionMessage) bool {
	if msg.ToolUseID != "" || msg.ToolName != "" {
		return true
	}
	return strings.Contains(msg.Content, "\"type\":\"tool_use\"") || strings.Contains(msg.Content, `"type": "tool_use"`)
}

// extractToolPairID extracts a tool pair identifier from a message.
func extractToolPairID(msg CompactionMessage) string {
	if msg.ToolUseID != "" {
		return msg.ToolUseID
	}
	return ""
}

// extractToolResultID extracts the tool result ID from a message.
func extractToolResultID(msg CompactionMessage) string {
	if msg.ToolUseID != "" {
		return msg.ToolUseID
	}
	// Try to extract from content
	// Look for "tool_use_id": "value" pattern
	idx := strings.Index(msg.Content, `"tool_use_id":`)
	if idx == -1 {
		return ""
	}
	rest := msg.Content[idx+len(`"tool_use_id":`):]
	// Find the quoted value
	start := strings.Index(rest, `"`)
	if start == -1 {
		return ""
	}
	rest = rest[start+1:]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}

// findSafeCompactionBoundary finds the index in the rounds slice where we can
// safely split without breaking tool_call/tool_result pairs.
func findSafeCompactionBoundary(rounds []apiRound, keepN int) int {
	if len(rounds) <= keepN+1 {
		return 0 // Not enough rounds to compact
	}

	// Start from the beginning; we want to compact the oldest rounds
	// but ensure we don't split a tool_call/tool_result pair.
	cutPoint := len(rounds) - keepN

	// Walk forward from cutPoint to ensure we don't split a tool pair
	for cutPoint < len(rounds) {
		// Check if this round is part of an uncompleted tool pair
		if cutPoint > 0 && rounds[cutPoint].isToolCall {
			// Check if the previous round started a tool call that this round completes
			if rounds[cutPoint-1].isToolCall && rounds[cutPoint-1].toolPairID != "" {
				// This round completes a tool pair; move cutPoint back to include the pair
				if rounds[cutPoint].toolPairID == rounds[cutPoint-1].toolPairID {
					cutPoint--
					continue
				}
			}
		}
		break
	}

	// Ensure we never compact the system message (usually round 0)
	if cutPoint == 0 && len(rounds) > 1 {
		// Check if round 0 is a system message
		if len(rounds[0].messages) > 0 && rounds[0].messages[0].Role == "system" {
			cutPoint = 1
		}
	}

	return cutPoint
}

// messageTokens returns estimated token count for a single message.
func messageTokens(msg CompactionMessage) int {
	tokens := estimateTokens(msg.Content)
	// Tool calls have overhead
	if msg.ToolUseID != "" {
		tokens += 10 // overhead for tool_use/tool_result structure
	}
	return tokens + 3 // role overhead
}

// roundTokens returns estimated token count for a round.
func roundTokens(round apiRound) int {
	total := 0
	for _, msg := range round.messages {
		total += messageTokens(msg)
	}
	return total
}

// totalTokens returns estimated token count for all messages.
func totalTokens(messages []CompactionMessage) int {
	total := 0
	for _, msg := range messages {
		total += messageTokens(msg)
	}
	return total
}

// flattenRounds flattens rounds back to a message slice.
func flattenRounds(rounds []apiRound) []CompactionMessage {
	var messages []CompactionMessage
	for _, round := range rounds {
		messages = append(messages, round.messages...)
	}
	return messages
}

// groupMessageParamsByRound groups API message params by round.
func groupMessageParamsByRound(messages []anthropic.MessageParam) []messageRoundParam {
	var rounds []messageRoundParam
	var current messageRoundParam
	firstMsg := true

	for _, msg := range messages {
		role := string(msg.Role)
		if role == "user" || firstMsg {
			if len(current.msgs) > 0 {
				rounds = append(rounds, current)
			}
			current = messageRoundParam{role: role, msgs: []anthropic.MessageParam{msg}}
			firstMsg = false
		} else {
			current.msgs = append(current.msgs, msg)
		}
	}
	if len(current.msgs) > 0 {
		rounds = append(rounds, current)
	}
	return rounds
}

// messageRoundParam represents a round of API message params.
type messageRoundParam struct {
	role string
	msgs []anthropic.MessageParam
}

// defaultCompactableTools returns the default set of tools whose results can be compacted.
func defaultCompactableTools() map[string]bool {
	return map[string]bool{
		"read_file":  true,
		"glob":       true,
		"grep":       true,
		"list_dir":   true,
		"web_fetch":  true,
		"web_search": true,
	}
}
