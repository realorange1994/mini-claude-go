package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// shrinkOversizedToolResultsByTokens shrinks oversized tool results to stay within token budgets.
// This prevents oversized tool outputs from causing prompt cache misses.
//
// Matching DeepSeek-Reasonix's shrink.ts shrinkOversizedToolResultsByTokens
func shrinkOversizedToolResultsByTokens(messages []anthropic.MessageParam, maxTokens int) (healedCount, tokensSaved, charsSaved int) {
	for i := range messages {
		msg := &messages[i]
		// Tool result blocks are in user-role messages, not "tool" role.
		// Check for OfToolResult blocks instead of checking role.
		hasToolResult := false
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				hasToolResult = true
				break
			}
		}
		if !hasToolResult {
			continue
		}

		// Get tool content from content blocks
		var content string
		for _, block := range msg.Content {
			if block.OfText != nil && block.OfText.Text != "" {
				content = block.OfText.Text
				break
			}
		}
		if content == "" {
			continue
		}

		// Skip if already under threshold
		if len(content) <= maxTokens {
			continue
		}

		beforeTokens := countTokensBounded(content)
		if beforeTokens <= maxTokens {
			continue
		}

		// Truncate to approximate token budget
		truncated := truncateForTokens(content, maxTokens)
		afterTokens := countTokensBounded(truncated)

		if afterTokens >= beforeTokens {
			continue
		}

		// Update the message content
		for j := range msg.Content {
			if msg.Content[j].OfText != nil {
				msg.Content[j].OfText.Text = truncated
				break
			}
		}

		healedCount++
		tokensSaved += intMax(0, beforeTokens-afterTokens)
		charsSaved += intMax(0, len(content)-len(truncated))
	}

	return healedCount, tokensSaved, charsSaved
}

// truncateForTokens truncates text to fit within approximately maxTokens.
func truncateForTokens(text string, maxTokens int) string {
	// Rough estimate: ~4 chars per token
	approxMaxChars := maxTokens * 4
	if len(text) <= approxMaxChars {
		return text
	}

	// Find a good break point (prefer newlines, sentences)
	truncated := text[:approxMaxChars]
	lastNewline := strings.LastIndex(truncated, "\n")
	lastPeriod := strings.LastIndex(truncated, ". ")

	breakPoint := lastNewline
	if lastPeriod > lastNewline && lastPeriod > approxMaxChars-200 {
		breakPoint = lastPeriod + 1
	}

	minBreak := intMax(100, approxMaxChars/4)
	if breakPoint > minBreak {
		truncated = truncated[:breakPoint]
	} else {
		// Just hard truncate at max chars
		truncated = truncated[:intMax(100, approxMaxChars-50)]
	}

	return truncated + "\n\n[... tool output truncated for token budget; use Read to get full content]"
}

// countTokensBounded is an alias for estimateTokens for backwards compatibility.
// Deprecated: use estimateTokens from compact.go instead.
func countTokensBounded(text string) int {
	return estimateTokens(text)
}

// fixToolCallPairing drops both unpaired assistant.tool_calls and stray tool messages.
// DeepSeek returns 400 errors on either case.
//
// Matching DeepSeek-Reasonix's healing.ts fixToolCallPairing
//
// IMPORTANT: The Anthropic API uses role="user" for tool_result messages, NOT role="tool".
// All tool_result blocks appear inside user-role messages. This function must check
// for OfToolResult blocks in user-role messages, not look for a "tool" role.
//
// Returns the filtered messages slice (may be shorter than input).
func fixToolCallPairing(messages []anthropic.MessageParam) (filtered []anthropic.MessageParam, droppedAssistantCalls, droppedStrayTools int) {
	out := make([]anthropic.MessageParam, 0, len(messages))
	i := 0

	for i < len(messages) {
		msg := messages[i]

		// Check if this is an assistant message with tool_calls
		if msg.Role == anthropic.MessageParamRoleAssistant && len(msg.Content) > 0 {
			hasToolCalls := false
			var toolCalls []anthropic.ToolUseBlockParam
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					hasToolCalls = true
					toolCalls = append(toolCalls, *block.OfToolUse)
				}
			}

			if hasToolCalls && len(toolCalls) > 0 {
				// Stamp missing IDs before validation
				seq := 0
				for idx, call := range toolCalls {
					if call.ID == "" {
						toolCalls[idx].ID = fmt.Sprintf("z-ext-%d", seq)
						seq++
					}
				}

				// Build set of needed tool call IDs
				needed := make(map[string]bool)
				for _, call := range toolCalls {
					if call.ID != "" {
						needed[call.ID] = true
					}
				}

				// Look for matching tool results in subsequent user-role messages.
				// The Anthropic API puts tool_result blocks inside user-role messages,
				// not "tool" role messages. We scan user messages that contain
				// OfToolResult blocks until we've found all needed IDs.
				var candidates []anthropic.MessageParam
				j := i + 1
				for j < len(messages) && len(needed) > 0 {
					nextMsg := messages[j]
					// Tool results are in user-role messages; stop at any other role
					// or at user messages that don't contain tool_result blocks
					// (which are regular user text messages, not tool responses).
					if nextMsg.Role != anthropic.MessageParamRoleUser {
						break
					}
					// Check if this user message contains tool_result blocks
					hasToolResults := false
					var matchedIDs []string
					for _, block := range nextMsg.Content {
						if block.OfToolResult != nil {
							hasToolResults = true
							if needed[block.OfToolResult.ToolUseID] {
								matchedIDs = append(matchedIDs, block.OfToolResult.ToolUseID)
							}
						}
					}
					if !hasToolResults {
						// This is a regular user text message, not a tool response — stop scanning
						break
					}
					// Accept the message if it contains any needed tool_result IDs
					if len(matchedIDs) == 0 {
						// Tool results present but none match our needed IDs — stop scanning
						break
					}
					for _, id := range matchedIDs {
						delete(needed, id)
					}
					candidates = append(candidates, nextMsg)
					j++
				}

				// If we found all needed tool results, keep the pair
				if len(needed) == 0 {
					// Reconstruct assistant message with stamped calls
					newContent := make([]anthropic.ContentBlockParamUnion, len(toolCalls))
					for idx, call := range toolCalls {
						newContent[idx] = anthropic.ContentBlockParamUnion{
							OfToolUse: &call,
						}
					}
					out = append(out, anthropic.MessageParam{
						Role:    msg.Role,
						Content: newContent,
					})
					out = append(out, candidates...)
					i = j
					continue
				} else {
					// Drop unpaired tool_calls and their partial results
					droppedAssistantCalls++
					droppedStrayTools += len(candidates)
					i = j
					continue
				}
			}
		}

		// Check if this is a stray user message containing tool_result blocks
		// without a preceding assistant tool_use message.
		if msg.Role == anthropic.MessageParamRoleUser {
			hasToolResult := false
			hasValidID := false
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					hasToolResult = true
					if block.OfToolResult.ToolUseID != "" {
						hasValidID = true
						break
					}
				}
			}
			// Only check for stray if this message contains tool_result blocks
			// but no valid tool_use_id (completely orphaned).
			if hasToolResult && !hasValidID {
				droppedStrayTools++
				i++
				continue
			}
		}

		out = append(out, msg)
		i++
	}

	return out, droppedAssistantCalls, droppedStrayTools
}

// extractPinnedConstraints extracts pinned constraints from system prompt
// to preserve them across compaction.
//
// Matches DeepSeek-Reasonix's context-manager.ts extractPinnedConstraints
// Pattern: # HIGH PRIORITY constraints, # User memory, # Project memory
func extractPinnedConstraints(systemPrompt string) string {
	headers := []string{
		"HIGH PRIORITY constraints",
		"User memory",
		"Project memory",
	}

	var results []string
	lines := strings.Split(systemPrompt, "\n")
	var current []string
	var activeHeader bool

	for _, line := range lines {
		isHeader := strings.HasPrefix(strings.TrimSpace(line), "# ")

		// Check if this line starts any of our target sections
		headerMatched := false
		if isHeader {
			for _, h := range headers {
				if strings.Contains(line, h) {
					headerMatched = true
					break
				}
			}
		}

		if headerMatched {
			// Save previous block if any
			if len(current) > 0 {
				results = append(results, strings.Join(current, "\n"))
			}
			activeHeader = true
			current = []string{line}
		} else if activeHeader {
			// Check for end of section (new header or end of file)
			if isHeader {
				results = append(results, strings.Join(current, "\n"))
				activeHeader = false
				current = nil
			} else if strings.TrimSpace(line) != "" {
				current = append(current, line)
			}
		}
	}

	// Don't forget the last block
	if len(current) > 0 {
		results = append(results, strings.Join(current, "\n"))
	}

	return strings.Join(results, "\n\n")
}

// stripHallucinatedToolMarkup strips hallucinated tool-call markup from model output.
// DeepSeek R1 can hallucinate DSML-style function call markup.
//
// Matches DeepSeek-Reasonix's thinking.ts stripHallucinatedToolMarkup
func stripHallucinatedToolMarkup(content string) string {
	result := content

	// Handle DSML blocks: <|DSML|function_calls>...</|function_calls|>
	for {
		start := strings.Index(result, "<|DSML|")
		if start < 0 {
			break
		}
		endTag := strings.Index(result[start:], "<|/function_calls|>")
		if endTag < 0 {
			// Try alternative end tag
			endTag = strings.Index(result[start:], "|>")
			if endTag < 0 {
				break
			}
			endTag = start + endTag + 2
		} else {
			endTag = start + endTag + len("<|/function_calls|>")
		}
		result = result[:start] + result[endTag:]
	}

	// Handle [TOOL_CALL]...[/TOOL_CALL] (case-insensitive)
	for {
		idx := strings.Index(strings.ToLower(result), "[tool_call]")
		if idx < 0 {
			break
		}
		endIdx := strings.Index(strings.ToLower(result[idx:]), "[/tool_call]")
		if endIdx < 0 {
			break
		}
		result = result[:idx] + result[idx+endIdx+len("[/tool_call]"):]
	}

	// Handle <function_call>...</function_call>
	for {
		start := strings.Index(result, "<function_call>")
		if start < 0 {
			break
		}
		end := strings.Index(result[start:], "</function_call>")
		if end < 0 {
			break
		}
		end = start + end + len("</function_call>")
		result = result[:start] + result[end:]
	}

	return result
}

// computePrefixFingerprint computes SHA-256 fingerprint of system + tools + fewshots.
// Used to detect cache drift that would cause cache misses.
//
// Matching DeepSeek-Reasonix's runtime.ts computeFingerprint
func computePrefixFingerprint(system string, toolSchemas map[string]string, fewshots []map[string]any) string {
	data := map[string]any{
		"system": system,
		"tools":  toolSchemas,
		"shots":  fewshots,
	}
	jsonBytes, _ := json.Marshal(data)
	h := sha256.Sum256(jsonBytes)
	return fmt.Sprintf("%x", h[:16])
}

// intMax returns the maximum of two integers
func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// preflightValidateMessages performs a final safety check on messages before
// sending to the API. It removes empty messages (zero content blocks) and
// validates tool_use/tool_result pairing. This catches edge cases where
// post-normalization modifications (injectCacheEdits, cacheMessageParams)
// create invalid message sequences that cause API error 2013.
func preflightValidateMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	cleaned := make([]anthropic.MessageParam, 0, len(messages))
	dropped := 0
	merged := 0
	for i, msg := range messages {
		// Drop messages with zero content blocks — these cause API 2013 errors
		// by breaking the tool_use/tool_result alternation chain.
		if len(msg.Content) == 0 {
			dropped++
			fmt.Fprintf(os.Stderr, "[preflight] DROP msg[%d] role=%s blocks=0 (EMPTY)\n", i, msg.Role)
			continue
		}
		// Drop messages with invalid/empty roles
		if msg.Role != anthropic.MessageParamRoleUser && msg.Role != anthropic.MessageParamRoleAssistant {
			dropped++
			fmt.Fprintf(os.Stderr, "[preflight] DROP msg[%d] invalid role=%s\n", i, msg.Role)
			continue
		}
		// Enforce strict role alternation: merge consecutive same-role messages.
		// The Anthropic API REQUIRES strict user/assistant alternation. Even
		// messages containing tool blocks must be merged — the API allows
		// multiple tool_use blocks in one assistant message and mixed
		// text+tool_result blocks in one user message.
		if len(cleaned) > 0 && cleaned[len(cleaned)-1].Role == msg.Role {
			lastIdx := len(cleaned) - 1
			cleaned[lastIdx].Content = append(cleaned[lastIdx].Content, msg.Content...)
			merged++
			continue
		}
		cleaned = append(cleaned, msg)
	}
	if dropped > 0 || merged > 0 {
		fmt.Fprintf(os.Stderr, "[preflight] stats: dropped=%d merged=%d in=%d out=%d\n",
			dropped, merged, len(messages), len(cleaned))
	}
	return cleaned
}