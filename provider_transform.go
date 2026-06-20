package main

import (
	"regexp"
)

// ─── Provider Message Transform Layer (MiMo-Code 1A) ───────────────────────
//
// Pre-send message normalization pipeline that handles provider-specific quirks.
// Covers Anthropic, Bedrock, and Mistral message format requirements.
//
// MiMo-Code source: provider/transform.ts (1322 lines, simplified to ~200)

// ProviderType represents the API provider type.
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderBedrock   ProviderType = "bedrock"
	ProviderMistral   ProviderType = "mistral"
	ProviderOpenAI    ProviderType = "openai"
)

// TransformConfig holds provider-specific transform configuration.
type TransformConfig struct {
	Provider      ProviderType
	ScrubToolIDs  bool // scrub tool call IDs to alphanumeric
	ReorderTools  bool // reorder tool_use/text blocks
	FilterEmpty   bool // filter empty content parts
	InsertFiller  bool // insert filler messages between tool/user turns
}

// NewTransformConfig creates a transform config for a provider.
func NewTransformConfig(provider ProviderType) *TransformConfig {
	switch provider {
	case ProviderAnthropic:
		return &TransformConfig{
			Provider:     provider,
			ScrubToolIDs: true,
			ReorderTools: true,
			FilterEmpty:  true,
		}
	case ProviderBedrock:
		return &TransformConfig{
			Provider:     provider,
			ScrubToolIDs: true,
			ReorderTools: true,
			FilterEmpty:  true,
		}
	case ProviderMistral:
		return &TransformConfig{
			Provider:     provider,
			ScrubToolIDs: true,
			InsertFiller: true,
		}
	default:
		return &TransformConfig{
			Provider: provider,
		}
	}
}

// TransformMessage represents a message for transformation.
type TransformMessage struct {
	Role    string
	Content []TransformContent
}

// TransformContent represents a content block in a message.
type TransformContent struct {
	Type      string // "text", "tool_use", "tool_result", "thinking"
	Text      string
	ID        string // for tool_use
	ToolUseID string // for tool_result
}

// TransformMessages applies provider-specific transforms to messages.
func TransformMessages(config *TransformConfig, messages []TransformMessage) []TransformMessage {
	if config == nil {
		return messages
	}

	result := make([]TransformMessage, len(messages))
	copy(result, messages)

	// Filter empty content parts (Anthropic/Bedrock)
	if config.FilterEmpty {
		result = filterEmptyContent(result)
	}

	// Scrub tool call IDs (Claude/Mistral)
	if config.ScrubToolIDs {
		result = scrubToolIDs(result, config.Provider)
	}

	// Reorder tool_use/text blocks (Anthropic/Bedrock)
	if config.ReorderTools {
		result = reorderToolBlocks(result)
	}

	// Insert filler messages (Mistral)
	if config.InsertFiller {
		result = insertFillerMessages(result)
	}

	return result
}

// filterEmptyContent removes empty text/reasoning parts.
func filterEmptyContent(messages []TransformMessage) []TransformMessage {
	var result []TransformMessage
	for _, msg := range messages {
		if len(msg.Content) == 0 {
			continue
		}

		var filtered []TransformContent
		for _, part := range msg.Content {
			if part.Type == "text" || part.Type == "thinking" {
				if part.Text == "" {
					continue
				}
			}
			filtered = append(filtered, part)
		}

		if len(filtered) > 0 {
			msg.Content = filtered
			result = append(result, msg)
		}
	}
	return result
}

// toolIDScrubber removes non-alphanumeric characters from tool IDs.
var toolIDScrubber = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// scrubToolIDs scrubs tool call IDs to alphanumeric only.
func scrubToolIDs(messages []TransformMessage, provider ProviderType) []TransformMessage {
	for i, msg := range messages {
		for j, part := range msg.Content {
			if part.Type == "tool_use" || part.Type == "tool_result" {
				if provider == ProviderMistral {
					// Mistral: 9-char alphanumeric
					messages[i].Content[j].ID = scrubMistralID(part.ID)
					messages[i].Content[j].ToolUseID = scrubMistralID(part.ToolUseID)
				} else {
					// Anthropic/Bedrock: alphanumeric + underscore + hyphen
					messages[i].Content[j].ID = toolIDScrubber.ReplaceAllString(part.ID, "_")
					messages[i].Content[j].ToolUseID = toolIDScrubber.ReplaceAllString(part.ToolUseID, "_")
				}
			}
		}
	}
	return messages
}

// scrubMistralID scrubs an ID for Mistral (9-char alphanumeric).
func scrubMistralID(id string) string {
	cleaned := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(id, "")
	if len(cleaned) > 9 {
		cleaned = cleaned[:9]
	}
	for len(cleaned) < 9 {
		cleaned += "0"
	}
	return cleaned
}

// reorderToolBlocks reorders assistant messages so text comes before tool_use.
// Anthropic rejects [tool_use, tool_use, text] but accepts [text, tool_use, tool_use].
func reorderToolBlocks(messages []TransformMessage) []TransformMessage {
	for i, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}

		hasToolUse := false
		hasNonTool := false
		for _, part := range msg.Content {
			if part.Type == "tool_use" {
				hasToolUse = true
			} else {
				hasNonTool = true
			}
		}

		if !hasToolUse || !hasNonTool {
			continue
		}

		// Check if there's a tool_use followed by non-tool content
		needsReorder := false
		foundToolUse := false
		for _, part := range msg.Content {
			if part.Type == "tool_use" {
				foundToolUse = true
			} else if foundToolUse {
				needsReorder = true
				break
			}
		}

		if needsReorder {
			var textParts []TransformContent
			var toolParts []TransformContent
			for _, part := range msg.Content {
				if part.Type == "tool_use" {
					toolParts = append(toolParts, part)
				} else {
					textParts = append(textParts, part)
				}
			}
			messages[i].Content = append(textParts, toolParts...)
		}
	}
	return messages
}

// insertFillerMessages inserts filler assistant messages between tool/user turns.
// Mistral requires assistant message between consecutive tool results.
func insertFillerMessages(messages []TransformMessage) []TransformMessage {
	var result []TransformMessage
	for i, msg := range messages {
		result = append(result, msg)

		// Check if this is a tool result followed by a user message
		if msg.Role == "user" && i > 0 {
			prev := messages[i-1]
			if prev.Role == "user" {
				// Insert filler assistant message
				result = append(result, TransformMessage{
					Role:    "assistant",
					Content: []TransformContent{{Type: "text", Text: "Continuing..."}},
				})
			}
		}
	}
	return result
}

// SanitizeToolCallID sanitizes a tool call ID for a specific provider.
func SanitizeToolCallID(provider ProviderType, id string) string {
	switch provider {
	case ProviderMistral:
		return scrubMistralID(id)
	case ProviderAnthropic, ProviderBedrock:
		return toolIDScrubber.ReplaceAllString(id, "_")
	default:
		return id
	}
}
