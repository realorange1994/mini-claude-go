package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ─── Serialization (extracted from compact.go) ─────────────────────────────

// serializeContentBlocks serializes content blocks to a JSON string.
// IMPORTANT: When serialization fails, it still extracts and preserves tool metadata
// so that the tool_use/tool_result pairing is not lost during compaction.
func serializeContentBlocks(blocks []anthropic.ContentBlockParamUnion) (content string, toolUseID string, toolName string) {
	// Always extract tool info from original blocks first, regardless of marshal outcome.
	for _, b := range blocks {
		if b.OfToolUse != nil {
			toolUseID = b.OfToolUse.ID
			toolName = b.OfToolUse.Name
			break
		}
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		// Marshal failed (e.g. unsupported type in Input). Preserve tool metadata
		// in a minimal JSON structure so compactionMessagesToEntries can still
		// reconstruct proper ToolUseContent instead of degrading to TextContent.
		if toolUseID != "" {
			content = fmt.Sprintf(`[{"type":"tool_use","id":"%s","name":"%s","input":{}}]`, toolUseID, toolName)
		} else {
			content = "{}"
		}
		return
	}
	content = string(data)
	return
}

// serializeToolResultBlocks serializes []anthropic.ToolResultBlockParam to a JSON string.
// IMPORTANT: When serialization fails, it still extracts and preserves tool_use_id
// so that the tool_result pairing is not lost during compaction.
func serializeToolResultBlocks(results []anthropic.ToolResultBlockParam) (content string, toolUseID string, toolName string) {
	// Always extract tool_use_id from original results first, regardless of marshal outcome.
	for _, r := range results {
		if r.ToolUseID != "" {
			toolUseID = r.ToolUseID
			// Try to extract tool name from the toolNames map by matching toolUseID
			for _, c := range r.Content {
				if c.OfText != nil && strings.Contains(c.OfText.Text, toolUseID) {
					// Tool name not available in result text, leave empty
				}
			}
			break
		}
	}
	data, err := json.Marshal(results)
	if err != nil {
		// Marshal failed (e.g. unsupported type in Content). Preserve tool_use_id
		// in a minimal JSON structure so compactionMessagesToEntries can still
		// reconstruct proper ToolResultContent instead of degrading to TextContent.
		if toolUseID != "" {
			// Extract text content for a truncated preview
			textPreview := ""
			for _, r := range results {
				for _, c := range r.Content {
					if c.OfText != nil {
						t := c.OfText.Text
						if len(t) > 500 {
							t = t[:500] + "...[truncated]"
						}
						textPreview = t
						break
					}
				}
				if textPreview != "" {
					break
				}
			}
			// Escape the text for JSON embedding
			escaped, _ := json.Marshal(textPreview)
			content = fmt.Sprintf(`[{"type":"tool_result","tool_use_id":"%s","content":[{"type":"text","text":%s}]}]`, toolUseID, string(escaped))
		} else {
			content = "{}"
		}
		return
	}
	content = string(data)
	return
}

// deserializeContentBlocks attempts to rebuild []anthropic.ContentBlockParamUnion from a JSON string.
func deserializeContentBlocks(content string) ([]anthropic.ContentBlockParamUnion, error) {
	var blocks []anthropic.ContentBlockParamUnion
	if err := json.Unmarshal([]byte(content), &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// deserializeToolResultBlocks attempts to rebuild []anthropic.ToolResultBlockParam from a JSON string.
func deserializeToolResultBlocks(content string) ([]anthropic.ToolResultBlockParam, error) {
	var results []anthropic.ToolResultBlockParam
	if err := json.Unmarshal([]byte(content), &results); err != nil {
		return nil, err
	}
	return results, nil
}

// isToolUseJSON detects if a string looks like serialized tool_use content.
func isToolUseJSON(s string) bool {
	return strings.Contains(s, `"type":"tool_use"`) || strings.Contains(s, `"type": "tool_use"`)
}

// isToolResultJSON detects if a string looks like serialized tool_result content.
func isToolResultJSON(s string) bool {
	return strings.Contains(s, `"type":"tool_result"`) || strings.Contains(s, `"type": "tool_result"`)
}

// detectToolNameFromJSON tries to extract tool name from JSON content.
func detectToolNameFromJSON(s string) string {
	// Look for "name":"xxx" pattern in tool_use blocks
	idx := strings.Index(s, `"name":`)
	if idx == -1 {
		return ""
	}
	rest := s[idx:]
	// Find the quoted value
	colon := strings.Index(rest, `"`)
	if colon == -1 {
		return ""
	}
	rest = rest[colon+1:]
	end := strings.Index(rest, `"`)
	if end == -1 {
		return ""
	}
	return rest[:end]
}
