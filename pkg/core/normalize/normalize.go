package normalize

import (
	"encoding/json"
	"fmt"
	"strings"
)

// NormalizeAPIMessages converts internal messages to the format expected by the LLM API.
// Mirrors pi's NormalizeAPIMessages function.
func NormalizeAPIMessages(messages []interface{}) ([]map[string]interface{}, error) {
	var result []map[string]interface{}

	for _, msg := range messages {
		switch m := msg.(type) {
		case map[string]interface{}:
			normalized, err := normalizeMessage(m)
			if err != nil {
				continue // skip invalid messages
			}
			result = append(result, normalized)
		case string:
			result = append(result, map[string]interface{}{
				"role":    "user",
				"content": m,
			})
		}
	}

	// Merge consecutive same-role messages
	result = mergeConsecutive(result)

	// Validate final message sequence
	result = validateSequence(result)

	return result, nil
}

func normalizeMessage(msg map[string]interface{}) (map[string]interface{}, error) {
	role, _ := msg["role"].(string)
	if role == "" {
		return nil, fmt.Errorf("message missing role")
	}

	// Handle content blocks
	content, ok := msg["content"]
	if !ok {
		return nil, fmt.Errorf("message missing content")
	}

	switch c := content.(type) {
	case string:
		return map[string]interface{}{
			"role":    role,
			"content": c,
		}, nil
	case []interface{}:
		// Content blocks array
		blocks := normalizeContentBlocks(c)
		return map[string]interface{}{
			"role":    role,
			"content": blocks,
		}, nil
	default:
		data, _ := json.Marshal(c)
		return map[string]interface{}{
			"role":    role,
			"content": string(data),
		}, nil
	}
}

func normalizeContentBlocks(blocks []interface{}) []map[string]interface{} {
	var result []map[string]interface{}
	for _, block := range blocks {
		if b, ok := block.(map[string]interface{}); ok {
			normalized := normalizeContentBlock(b)
			if normalized != nil {
				result = append(result, normalized)
			}
		}
	}
	return result
}

func normalizeContentBlock(block map[string]interface{}) map[string]interface{} {
	blockType, _ := block["type"].(string)
	switch blockType {
	case "text":
		text, _ := block["text"].(string)
		return map[string]interface{}{
			"type": "text",
			"text": text,
		}
	case "tool_use":
		return map[string]interface{}{
			"type":  "tool_use",
			"id":    block["id"],
			"name":  block["name"],
			"input": block["input"],
		}
	case "tool_result":
		return map[string]interface{}{
			"type":        "tool_result",
			"tool_use_id": block["tool_use_id"],
			"content":     block["content"],
			"is_error":    block["is_error"],
		}
	case "image":
		return map[string]interface{}{
			"type":   "image",
			"source": block["source"],
		}
	default:
		return block
	}
}

func mergeConsecutive(messages []map[string]interface{}) []map[string]interface{} {
	if len(messages) <= 1 {
		return messages
	}

	var result []map[string]interface{}
	current := messages[0]

	for i := 1; i < len(messages); i++ {
		if messages[i]["role"] == current["role"] {
			// Merge content
			current = mergeContent(current, messages[i])
		} else {
			result = append(result, current)
			current = messages[i]
		}
	}
	result = append(result, current)
	return result
}

func mergeContent(a, b map[string]interface{}) map[string]interface{} {
	// Simple string merge
	aContent, _ := a["content"].(string)
	bContent, _ := b["content"].(string)
	return map[string]interface{}{
		"role":    a["role"],
		"content": aContent + "\n" + bContent,
	}
}

func validateSequence(messages []map[string]interface{}) []map[string]interface{} {
	// Ensure messages alternate between user and assistant
	if len(messages) == 0 {
		return messages
	}

	var result []map[string]interface{}
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" {
			continue // system messages are handled separately
		}
		if len(result) > 0 {
			lastRole, _ := result[len(result)-1]["role"].(string)
			if role == lastRole && role != "tool" {
				continue // skip duplicate role
			}
		}
		result = append(result, msg)
	}
	return result
}

// EstimateTokens gives a rough estimate of token count for messages
func EstimateTokens(messages []map[string]interface{}) int {
	total := 0
	for _, msg := range messages {
		content, _ := msg["content"].(string)
		// Rough: 1 token ~ 4 chars for English, ~1-2 chars for CJK
		tokens := len(content) / 4
		cjk := 0
		for _, r := range content {
			if r >= 0x4E00 && r <= 0x9FFF {
				cjk++
			}
		}
		tokens += cjk / 2
		total += tokens
	}
	return total
}

// TruncateMessages truncates messages to fit within token budget
func TruncateMessages(messages []map[string]interface{}, maxTokens int) []map[string]interface{} {
	budget := maxTokens
	// Keep from the end (most recent) backwards
	var result []map[string]interface{}
	for i := len(messages) - 1; i >= 0; i-- {
		tokens := EstimateTokens([]map[string]interface{}{messages[i]})
		if budget-tokens < 0 {
			break
		}
		budget -= tokens
		result = append([]map[string]interface{}{messages[i]}, result...)
	}
	return result
}

// BuildSystemPrompt constructs the system prompt from config
func BuildSystemPrompt(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}
