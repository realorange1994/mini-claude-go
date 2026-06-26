// Package main provides API message normalization for KV cache reuse (Hermes-style).
//
// Normalizes API messages before sending to improve Anthropic prefix cache hit rate:
// 1. Strip virtual/internal messages (internal bookkeeping)
// 2. Enforce role alternation (merge consecutive same-role messages)
// 3. Ensure tool_use/tool_result pairing (fix orphans)
// 4. Filter empty messages (remove whitespace-only assistant messages)
// 5. Strip images from error tool_results (API requirement)
// 6. Validate image blocks (remove invalid images)
// 7. Reorder attachments (bubble image/document to message start)
// 8. Sort JSON keys in tool_call input by alphabetical order
// 9. Normalize whitespace in tool_result content (collapse multiple blank lines)
// These normalizations make identical logical content produce identical API payloads,
// which is critical for Anthropic's prefix caching to work effectively.
package main

import (
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// NormalizeAPIMessages normalizes a list of API messages for KV cache reuse.
// Returns a new slice with normalized messages.
// The normalization order matters:
//  1. StripVirtualMessages (remove internal bookkeeping messages)
//  2. EnforceRoleAlternation (establishes correct message order)
//  3. EnsureToolResultPairing (fixes tool pairs)
//  4. FilterEmptyMessages (cleans up empties)
//  5. StripImagesFromErrorToolResults (strip images from error tool_results)
//  6. ValidateImagesForAPI (remove invalid image blocks)
//  7. ReorderContentForAPI (bubble tool_results + attachments to message start)
//  8. Existing normalizations (sort keys, whitespace)
func NormalizeAPIMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	messages = StripVirtualMessages(messages)
	messages = hoistToolResults(messages)
	messages = EnforceRoleAlternation(messages)
	// Re-hoist after EnforceRoleAlternation: merging two user messages can
	// place the second message's tool_result blocks after the first message's
	// text blocks, breaking the API's tool_result-first requirement.
	// Upstream: mergeAdjacentUserMessages calls hoistToolResults on the
	// merged content (messages.ts:2416-2418).
	messages = hoistToolResults(messages)
	messages = EnsureToolResultPairing(messages)
	messages = FilterEmptyMessages(messages)
	messages = StripImagesFromErrorToolResults(messages)
	messages, _ = ValidateImagesForAPI(messages)
	messages = ReorderContentForAPI(messages)
	messages = smooshSystemReminders(messages)
	messages = stripEmptyTextBlocks(messages)

	// Final safety: filter out messages with empty/invalid roles or zero content.
	// These can be created by edge cases in compaction, session resume, or
	// message construction bugs. They cause API 400 errors and tool pairing issues.
	// Note: we keep "system" role messages (used as boundary markers).
	cleaned := make([]anthropic.MessageParam, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleUser &&
			msg.Role != anthropic.MessageParamRoleAssistant &&
			msg.Role != "system" { // allow system role for boundary markers
			continue // drop messages with empty/invalid roles
		}
		if len(msg.Content) == 0 && msg.Role != "system" {
			continue // drop messages with zero content blocks (except system markers)
		}
		cleaned = append(cleaned, msg)
	}
	messages = cleaned

	// Existing normalizations (sort keys, whitespace)
	result := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		result[i] = normalizeMessage(msg)
	}
	return result
}

// ============================================================================
// P0-1b: Tool Result Hoisting
// ============================================================================

// hoistToolResults moves tool_result blocks to the front of each user message's
// content array. This ensures a stable, deterministic ordering regardless of
// how content blocks were originally appended. Without hoisting, the position
// of tool_result blocks can vary between turns (e.g., when a system-reminder
// text block is injected before tool_results), which changes the structure of
// previously-cached messages and breaks the Anthropic KV cache prefix.
//
// Upstream: messages.ts hoistToolResults() — runs before all other normalization.
func hoistToolResults(messages []anthropic.MessageParam) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleUser || len(msg.Content) <= 1 {
			result[i] = msg
			continue
		}

		// Check if there are tool_result blocks that are not already at the front
		hasToolResult := false
		firstNonTool := -1
		for j, block := range msg.Content {
			if block.OfToolResult != nil {
				hasToolResult = true
			} else if firstNonTool == -1 && !hasToolResult {
				// Non-tool block before any tool_result — need to reorder
				firstNonTool = j
			}
		}

		if !hasToolResult || firstNonTool == -1 {
			// No tool_results, or tool_results are already at the front
			result[i] = msg
			continue
		}

		// Partition: tool_results first, then everything else (preserving relative order)
		var toolResults, others []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				toolResults = append(toolResults, block)
			} else {
				others = append(others, block)
			}
		}

		newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
		newContent = append(newContent, toolResults...)
		newContent = append(newContent, others...)
		result[i] = anthropic.MessageParam{Role: msg.Role, Content: newContent}
	}
	return result
}

// ============================================================================
// P0-2: Role Alternation Enforcement
// ============================================================================

// EnforceRoleAlternation ensures messages alternate between user and assistant roles.
// Consecutive same-role messages are merged. If the first message is from the assistant,
// a synthetic user message is prepended.
//
// IMPORTANT: user messages containing tool_results are NOT merged with other user
// messages, because merging them can break tool_use/tool_result pairing and cause
// API error 2013. Tool_result messages must immediately follow their tool_use.
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
			// Check if either message contains tool_results or tool_use — merging
			// them can break tool_use/tool_result pairing (causes API error 2013).
			lastHasToolResults := hasToolResultBlocks(*last)
			msgHasToolResults := hasToolResultBlocks(msg)
			lastHasToolUse := hasToolUseBlocks(*last)
			msgHasToolUse := hasToolUseBlocks(msg)
			if lastHasToolResults || msgHasToolResults || lastHasToolUse || msgHasToolUse {
				// Don't merge: tool_result/tool_use messages must stay separate
				// to preserve pairing with their tool_use blocks.
				result = append(result, msg)
				continue
			}

			// Merge consecutive same-role messages by combining content blocks.
			last.Content = append(last.Content, msg.Content...)
		} else {
			result = append(result, msg)
		}
	}

	return result
}

// hasToolResultBlocks returns true if a message contains tool_result content blocks.
func hasToolResultBlocks(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return true
		}
	}
	return false
}

// hasToolUseBlocks returns true if the message contains tool_use blocks.
func hasToolUseBlocks(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			return true
		}
	}
	return false
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
// - Empty user messages (zero content blocks) are removed — these can be created
//   by edge cases in FixRoleAlternation, EnsureToolResultPairing, or compaction.
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
		} else if msg.Role == anthropic.MessageParamRoleUser {
			// Filter empty user messages — zero content blocks cause API error 2013
			// by breaking the tool_use/tool_result pairing chain.
			if len(msg.Content) == 0 {
				continue
			}
			// Also filter user messages that are purely whitespace/empty text blocks
			// with no tool_results or attachments.
			if isWhitespaceOnlyUser(msg) {
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

// isWhitespaceOnlyUser returns true if a user message contains only
// whitespace-only text blocks (no tool_results, images, documents, etc).
// Such messages serve no purpose and can cause API error 2013 by breaking
// the tool_use/tool_result alternation chain.
func isWhitespaceOnlyUser(msg anthropic.MessageParam) bool {
	if msg.Role != anthropic.MessageParamRoleUser {
		return false
	}
	if len(msg.Content) == 0 {
		return true
	}
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false // tool_results are substantive content
		}
		if block.OfImage != nil || block.OfDocument != nil {
			return false // attachments are substantive content
		}
		if block.OfText != nil {
			if strings.TrimSpace(block.OfText.Text) != "" {
				return false
			}
		} else {
			// Any other block type (image, document, etc.) — treat as substantive
			return false
		}
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
// P1-6a: Virtual Message Stripping
// ============================================================================

// StripVirtualMessages removes messages that are marked as virtual/internal.
// These are used for internal bookkeeping and should not reach the API.
// A virtual message is a user message where every text block contains only
// "[virtual]", "[system]", or is empty/whitespace-only.
func StripVirtualMessages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	var result []anthropic.MessageParam
	for _, msg := range messages {
		if msg.Role == anthropic.MessageParamRoleUser && isVirtualUserMessage(msg) {
			continue
		}
		result = append(result, msg)
	}
	return result
}

// isVirtualUserMessage returns true if a user message is purely virtual/internal.
// A message is virtual if all its content blocks are text blocks containing
// only "[virtual]", "[system]", or whitespace.
func isVirtualUserMessage(msg anthropic.MessageParam) bool {
	if len(msg.Content) == 0 {
		return true
	}
	for _, block := range msg.Content {
		// Any non-text block means this is not a virtual message
		if block.OfText == nil {
			return false
		}
		text := strings.TrimSpace(block.OfText.Text)
		if text != "" && text != "[virtual]" && text != "[system]" {
			return false
		}
	}
	return true
}

// ============================================================================
// P1-6b: Attachment Reordering
// ============================================================================

// ReorderContentForAPI sorts content blocks within user messages to a canonical
// order: tool_results first (API requirement), then image/document, then text.
//
// IMPORTANT: We do NOT reorder assistant messages. The API returns assistant
// messages in a specific block order (e.g., thinking → text → tool_use).
// Reordering those blocks would change the structure of previously-cached
// messages, breaking the Anthropic KV cache prefix. Only user messages are
// reordered since we control their construction.
func ReorderContentForAPI(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	result := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		if len(msg.Content) <= 1 {
			result[i] = msg
			continue
		}

		if msg.Role == anthropic.MessageParamRoleUser {
			// User messages: tool_results first (API requirement), then attachments, then text
			var toolResults, attachments, others []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					toolResults = append(toolResults, block)
				} else if block.OfImage != nil || block.OfDocument != nil {
					attachments = append(attachments, block)
				} else {
					others = append(others, block)
				}
			}

			if len(toolResults) == 0 && len(attachments) == 0 {
				result[i] = msg
				continue
			}

			newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
			newContent = append(newContent, toolResults...)
			newContent = append(newContent, attachments...)
			newContent = append(newContent, others...)
			result[i] = anthropic.MessageParam{Role: msg.Role, Content: newContent}
		} else {
			// Assistant messages: DO NOT reorder. The API returns them in a
			// specific order that we must preserve for cache stability.
			result[i] = msg
		}
	}
	return result
}

// smooshSystemReminders is disabled for cache stability.
// Folding <system-reminder> text blocks into tool_result content changes the content
// block count of previously-cached messages, which breaks the Anthropic KV cache
// prefix. Since NormalizeAPIMessages re-runs on all messages every turn, any fold
// applied to a previously-sent message would change its structure from what the API
// cached (2 blocks → 1 block), invalidating the entire cache prefix from that point.
//
// Upstream: messages.ts smooshSystemReminderSiblings() — gated by tengu_chair_sermon.
// In the Go version, we skip this optimization entirely to preserve cache stability.
func smooshSystemReminders(messages []anthropic.MessageParam) []anthropic.MessageParam {
	return messages // disabled — see comment above for rationale
}

// stripEmptyTextBlocks removes empty text blocks from user messages only.
// We do NOT strip from assistant messages because that would change the structure
// of previously-cached assistant responses blocks, breaking the Anthropic KV
// cache prefix. Since NormalizeAPIMessages re-runs on all messages every turn,
// removing an empty block from a cached assistant message would change its
// content block count from what the API cached, invalidating the cache prefix.
func stripEmptyTextBlocks(messages []anthropic.MessageParam) []anthropic.MessageParam {
	for i, msg := range messages {
		// Only process user messages — assistant messages are API-returned
		// and must be kept verbatim for cache stability
		if msg.Role != anthropic.MessageParamRoleUser {
			continue
		}
		changed := false
		newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
		for _, block := range msg.Content {
			if block.OfText != nil && strings.TrimSpace(block.OfText.Text) == "" {
				changed = true
				continue
			}
			newContent = append(newContent, block)
		}
		if changed {
			messages[i] = anthropic.MessageParam{Role: msg.Role, Content: newContent}
		}
	}
	return messages
}

// ============================================================================
// P1-6c: Image Validation
// ============================================================================

// supportedMediaTypes is the set of media types that the Anthropic API
// accepts for image blocks.
var supportedMediaTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// ValidateImagesForAPI removes invalid image blocks from messages.
// Returns the cleaned messages and a list of reasons for each removed image.
// An image block is invalid if:
//   - It has no source data (neither base64 nor URL source is set)
//   - For base64 source: media_type is missing or unsupported, or data is empty
//   - For URL source: the URL is empty
func ValidateImagesForAPI(messages []anthropic.MessageParam) ([]anthropic.MessageParam, []string) {
	if len(messages) == 0 {
		return messages, nil
	}

	var reasons []string
	result := make([]anthropic.MessageParam, len(messages))

	for i, msg := range messages {
		var newContent []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			if block.OfImage != nil {
				if valid, reason := isValidImageBlock(block.OfImage); !valid {
					reasons = append(reasons, reason)
					continue
				}
			}
			newContent = append(newContent, block)
		}

		if len(newContent) == len(msg.Content) {
			result[i] = msg
		} else {
			result[i] = anthropic.MessageParam{
				Role:    msg.Role,
				Content: newContent,
			}
		}
	}

	return result, reasons
}

// isValidImageBlock checks whether an image block has valid source data.
// Returns (true, "") if valid, or (false, reason) if invalid.
func isValidImageBlock(img *anthropic.ImageBlockParam) (bool, string) {
	if img == nil {
		return false, "image block is nil"
	}

	src := img.Source

	// Check base64 source
	if !param.IsOmitted(src.OfBase64) && src.OfBase64 != nil {
		if src.OfBase64.Data == "" {
			return false, "image base64 source has empty data"
		}
		mediaType := string(src.OfBase64.MediaType)
		if mediaType == "" {
			return false, "image base64 source has empty media_type"
		}
		if !supportedMediaTypes[mediaType] {
			return false, "image base64 source has unsupported media_type: " + mediaType
		}
		return true, ""
	}

	// Check URL source
	if !param.IsOmitted(src.OfURL) && src.OfURL != nil {
		if src.OfURL.URL == "" {
			return false, "image URL source has empty url"
		}
		return true, ""
	}

	// Neither source is set
	return false, "image block has no source (neither base64 nor URL)"
}

// ============================================================================
// P1-6d: Error Tool Result Image Stripping
// ============================================================================

// StripImagesFromErrorToolResults removes image/document blocks from tool_result
// blocks that have is_error=true. The API does not accept images in error
// tool_results — only text content is allowed.
func StripImagesFromErrorToolResults(messages []anthropic.MessageParam) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	result := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		hasErrorToolResult := false
		for _, block := range msg.Content {
			if block.OfToolResult != nil && block.OfToolResult.IsError.Valid() && block.OfToolResult.IsError.Value {
				hasErrorToolResult = true
				break
			}
		}

		if !hasErrorToolResult {
			result[i] = msg
			continue
		}

		newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
		for _, block := range msg.Content {
			if block.OfToolResult != nil && block.OfToolResult.IsError.Valid() && block.OfToolResult.IsError.Value {
				// Strip image/document from this error tool_result
				filtered := stripImagesFromToolResultContent(block.OfToolResult)
				newContent = append(newContent, anthropic.ContentBlockParamUnion{
					OfToolResult: filtered,
				})
			} else {
				newContent = append(newContent, block)
			}
		}
		result[i] = anthropic.MessageParam{
			Role:    msg.Role,
			Content: newContent,
		}
	}
	return result
}

// stripImagesFromToolResultContent removes image and document blocks from
// a tool_result's content, keeping only text and other non-attachment blocks.
func stripImagesFromToolResultContent(tr *anthropic.ToolResultBlockParam) *anthropic.ToolResultBlockParam {
	var filtered []anthropic.ToolResultBlockParamContentUnion
	for _, c := range tr.Content {
		if c.OfImage != nil || c.OfDocument != nil {
			continue
		}
		filtered = append(filtered, c)
	}
	return &anthropic.ToolResultBlockParam{
		ToolUseID:    tr.ToolUseID,
		IsError:      tr.IsError,
		CacheControl: tr.CacheControl,
		Content:      filtered,
	}
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
		ToolUseID:    block.OfToolResult.ToolUseID,
		IsError:      block.OfToolResult.IsError,
		CacheControl: block.OfToolResult.CacheControl,
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

