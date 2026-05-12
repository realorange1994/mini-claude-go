package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// BuildCompactTranscript builds a compact conversation transcript for the
// auto mode classifier. It includes user messages and tool calls but NOT
// assistant text (security requirement: agent must not influence classifier).
// The transcript is wrapped in <transcript> XML tags, matching upstream's
// buildTranscriptForClassifier() in yoloClassifier.ts:436.
func BuildCompactTranscript(ctx *ConversationContext, maxMessages int) string {
	if maxMessages <= 0 {
		maxMessages = 20
	}

	entries := ctx.Entries()
	if len(entries) == 0 {
		return ""
	}

	start := len(entries) - maxMessages
	if start < 0 {
		start = 0
	}
	recent := entries[start:]

	var sb strings.Builder
	sb.WriteString("<transcript>\n")
	for _, entry := range recent {
		switch v := entry.content.(type) {
		case TextContent:
			if entry.role == "user" {
				text := string(v)
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("[User] %s\n", text))
			}
			// Skip assistant text (security: don't let agent influence classifier)
			// This is the primary injection defense — matches upstream's
			// yoloClassifier.ts:346-348: "Only include tool_use blocks --
			// assistant text is model-authored and could be crafted to
			// influence the classifier's decision."

		case ToolUseContent:
			for _, block := range v {
				if block.OfToolUse != nil {
					inputDesc := formatToolInputCompact(block.OfToolUse.Name, block.OfToolUse.Input)
					sb.WriteString(fmt.Sprintf("[Tool: %s] %s\n", block.OfToolUse.Name, inputDesc))
				}
			}

		case ToolResultContent:
			for _, r := range v {
				content := extractToolResultText(r.Content)
				if len(content) > 100 {
					content = content[:100] + "..."
				}
				// Check if this is an AskUserQuestion result with user approval
				if isAskUserApproval(content) {
					sb.WriteString(fmt.Sprintf("[Result] USER EXPLICITLY APPROVED: %s\n", content))
				} else {
					sb.WriteString(fmt.Sprintf("[Result] %s\n", content))
				}
			}

		// Skip CompactBoundaryContent, SummaryContent, AttachmentContent
		}
	}
	sb.WriteString("</transcript>\n")

	return sb.String()
}

// BuildJSONLTranscript builds a JSONL-format transcript for the auto mode
// classifier. When JSONL mode is enabled (CLAUDE_CODE_JSONL_TRANSCRIPT=1 or
// feature flag), each line is a JSON dict that safely serializes content,
// preventing role-spoofing via injection. Matches upstream's JSONL path in
// yoloClassifier.ts:378-426.
func BuildJSONLTranscript(ctx *ConversationContext, maxMessages int) string {
	if maxMessages <= 0 {
		maxMessages = 20
	}

	entries := ctx.Entries()
	if len(entries) == 0 {
		return ""
	}

	start := len(entries) - maxMessages
	if start < 0 {
		start = 0
	}
	recent := entries[start:]

	var lines []string
	for _, entry := range recent {
		switch v := entry.content.(type) {
		case TextContent:
			if entry.role == "user" {
				text := string(v)
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				line, _ := json.Marshal(map[string]string{"user": text})
				lines = append(lines, string(line))
			}

		case ToolUseContent:
			for _, block := range v {
				if block.OfToolUse != nil {
					inputDesc := formatToolInputCompact(block.OfToolUse.Name, block.OfToolUse.Input)
					line, _ := json.Marshal(map[string]string{block.OfToolUse.Name: inputDesc})
					lines = append(lines, string(line))
				}
			}

		case ToolResultContent:
			for _, r := range v {
				content := extractToolResultText(r.Content)
				if len(content) > 100 {
					content = content[:100] + "..."
				}
				line, _ := json.Marshal(map[string]string{"result": content})
				lines = append(lines, string(line))
			}
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// extractToolResultText extracts plain text from tool result content blocks.
func extractToolResultText(blocks []anthropic.ToolResultBlockParamContentUnion) string {
	var parts []string
	for _, block := range blocks {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, " ")
}

// formatToolInputCompact formats tool input in a compact form.
// Input is typed as any (matching ToolUseBlockParam.Input in anthropic-sdk-go v1.35.0).
func formatToolInputCompact(toolName string, input any) string {
	if input == nil {
		return ""
	}

	// If it's already a map, use it directly; otherwise try JSON round-trip.
	var params map[string]any
	switch m := input.(type) {
	case map[string]any:
		params = m
	default:
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Sprintf("%v", input)
		}
		if err := json.Unmarshal(data, &params); err != nil {
			return "(parse error)"
		}
	}

	switch toolName {
	case "exec":
		if cmd, ok := params["command"].(string); ok {
			if len(cmd) > 200 {
				cmd = cmd[:200] + "..."
			}
			return cmd
		}
	case "write_file":
		if path, ok := params["file_path"].(string); ok {
			return path
		}
	case "edit_file":
		if path, ok := params["file_path"].(string); ok {
			return path
		}
	case "read_file":
		if path, ok := params["file_path"].(string); ok {
			return path
		}
	case "grep":
		pattern, _ := params["pattern"].(string)
		path, _ := params["path"].(string)
		return fmt.Sprintf("%q in %s", pattern, path)
	case "glob":
		if pattern, ok := params["pattern"].(string); ok {
			return pattern
		}
	}

	// Generic
	parts := make([]string, 0, len(params))
	for k, v := range params {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, ", ")
}

// isAskUserApproval detects whether a tool result represents an explicit user
// approval from AskUserQuestion. Returns true if the user selected an option
// that indicates consent (yes/ok/sure/continue/allow/proceed/etc).
func isAskUserApproval(result string) bool {
	lower := strings.ToLower(result)
	// AskUserQuestion format: "Q: ...\nA: <option>"
	if !strings.Contains(lower, "q:") || !strings.Contains(lower, "a:") {
		return false
	}
	// Extract the answer part
	idx := strings.Index(lower, "a:")
	if idx < 0 {
		return false
	}
	answer := strings.TrimSpace(lower[idx+2:])
	// Check for affirmative keywords
	affirmations := []string{
		"yes", "ok", "sure", "continue", "allow", "proceed",
		"go ahead", "fine", "good", "agreed", "approve",
		"do it", "go for it", "yes, continue", "approved",
		"create", "yes allow", "yes approve",
	}
	for _, a := range affirmations {
		if strings.Contains(answer, a) {
			return true
		}
	}
	return false
}
