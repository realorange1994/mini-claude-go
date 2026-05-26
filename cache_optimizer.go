package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
		if string(msg.Role) != "tool" {
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

// countTokensBounded provides a bounded token count estimate.
// Uses simple heuristic: ~4 chars per token for English, ~2 for CJK.
func countTokensBounded(text string) int {
	if len(text) == 0 {
		return 0
	}

	// Count CJK characters (roughly 1 token per character)
	cjkCount := 0
	for _, r := range text {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) { // Katakana
			cjkCount++
		}
	}

	// Non-CJK chars: ~4 chars per token
	nonCJK := len(text) - cjkCount
	return (nonCJK / 4) + cjkCount
}

// fixToolCallPairing drops both unpaired assistant.tool_calls and stray tool messages.
// DeepSeek returns 400 errors on either case.
//
// Matching DeepSeek-Reasonix's healing.ts fixToolCallPairing
func fixToolCallPairing(messages []anthropic.MessageParam) (droppedAssistantCalls, droppedStrayTools int) {
	out := make([]anthropic.MessageParam, 0, len(messages))
	i := 0

	for i < len(messages) {
		msg := messages[i]

		// Check if this is an assistant message with tool_calls
		if string(msg.Role) == "assistant" && len(msg.Content) > 0 {
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

				// Look for matching tool results
				var candidates []anthropic.MessageParam
				j := i + 1
				for j < len(messages) && len(needed) > 0 {
					nextMsg := messages[j]
					if string(nextMsg.Role) != "tool" {
						break
					}

					// Find tool_call_id by scanning for ToolResult content
					var toolCallID string
					for _, block := range nextMsg.Content {
						if block.OfToolResult != nil {
							toolCallID = block.OfToolResult.ToolUseID
							break
						}
					}

					if toolCallID == "" || !needed[toolCallID] {
						break
					}

					delete(needed, toolCallID)
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
					// Drop unpaired tool_calls and their results
					droppedAssistantCalls++
					droppedStrayTools += len(candidates)
					i = j
					continue
				}
			}
		}

		// Check if this is a stray tool message without matching tool_call
		if string(msg.Role) == "tool" {
			hasValidID := false
			for _, block := range msg.Content {
				if block.OfToolResult != nil && block.OfToolResult.ToolUseID != "" {
					hasValidID = true
					break
				}
			}
			if !hasValidID {
				droppedStrayTools++
				i++
				continue
			}
		}

		out = append(out, msg)
		i++
	}

	// Copy back to messages
	copy(messages, out)
	for i := len(out); i < len(messages); i++ {
		messages[i] = anthropic.MessageParam{}
	}

	return droppedAssistantCalls, droppedStrayTools
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

	// Handle [TOOL_CALL]...[/TOOL_CALL]
	for {
		start := strings.Index(result, "[TOOL_CALL]")
		if start < 0 {
			break
		}
		end := strings.Index(result[start:], "[/TOOL_CALL]")
		if end < 0 {
			break
		}
		end = start + end + len("[/TOOL_CALL]")
		result = result[:start] + result[end:]
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