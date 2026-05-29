// Package messages provides message types and transformers for the coding agent.
package messages

import (
	"fmt"
	"strings"
)

const (
	// CompactionSummaryPrefix/Suffix wrap compaction summaries so the model treats them as metadata.
	CompactionSummaryPrefix = `The conversation history before this point was compacted into the following summary:

<summary>
`
	CompactionSummarySuffix = `
</summary>`

	// BranchSummaryPrefix/Suffix wrap branch return summaries.
	BranchSummaryPrefix = `The following is a summary of a branch that this conversation came back from:

<summary>
`
	BranchSummarySuffix = `</summary>`
)

// ---------------------------------------------------------------------------
// LLM Message types (for API calls)
// ---------------------------------------------------------------------------

// LLMMessage represents a message to send to the LLM API.
type LLMMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a content block in LLM messages.
type ContentBlock struct {
	Type      string `json:"type"` // "text", "tool_use", "tool_result"
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// ---------------------------------------------------------------------------
// Message conversion functions
// ---------------------------------------------------------------------------

// BashExecutionText formats a bash execution result as user-facing text for LLM context.
func BashExecutionText(command, output string, exitCode int, cancelled bool, truncated bool, fullPath string) string {
	var b strings.Builder
	b.WriteString("Ran `" + command + "`\n")
	if output != "" {
		b.WriteString("```\n" + output + "\n```")
	} else {
		b.WriteString("(no output)")
	}
	if cancelled {
		b.WriteString("\n\n(command cancelled)")
	} else if exitCode != 0 {
		b.WriteString(fmt.Sprintf("\n\nCommand exited with code %d", exitCode))
	}
	if truncated && fullPath != "" {
		b.WriteString(fmt.Sprintf("\n\n[Output truncated. Full output: %s]", fullPath))
	}
	return b.String()
}

// ConvertToLlm transforms AgentMessages (including custom types) to LLM-compatible messages.
// This is used by:
// - Agent's transformToLlm option (for prompt calls and queued messages)
// - Compaction's generateSummary (for summarization)
// - Custom extensions and tools
func ConvertToLlm(messages []any) []LLMMessage {
	result := make([]LLMMessage, 0, len(messages))

	for _, m := range messages {
		var msg LLMMessage

		switch mt := m.(type) {
		case *BashExecutionMessage:
			// Skip messages excluded from context (!! prefix)
			if mt.ExcludeFromContext() {
				continue
			}
			output := mt.Stdout()
			if mt.Stderr() != "" {
				output += "\n[stderr]\n" + mt.Stderr()
			}
			msg = LLMMessage{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: BashExecutionText(mt.Command(), output, mt.ExitCode(), mt.Cancelled(), mt.Truncated(), mt.FullOutputPath())},
				},
			}

		case *CustomMessage:
			content := mt.GetContent()
			if content == "" {
				content = " "
			}
			msg = LLMMessage{
				Role:    "user",
				Content: []ContentBlock{{Type: "text", Text: content}},
			}

		case *BranchSummaryMessage:
			msg = LLMMessage{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: BranchSummaryPrefix + mt.Summary() + BranchSummarySuffix},
				},
			}

		case *CompactionSummaryMessage:
			msg = LLMMessage{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: CompactionSummaryPrefix + mt.Summary() + CompactionSummarySuffix},
				},
			}

		case *TextMessage:
			msg = LLMMessage{
				Role:    string(mt.GetRole()),
				Content: mt.GetContent(),
			}

		case *ToolMessage:
			msg = LLMMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "tool_use", ID: mt.ID(), Name: mt.Name(), Input: mt.Input()},
				},
			}

		case *ToolResultMessage:
			content := mt.GetContent()
			if mt.IsError() {
				content = "Error: " + content
			}
			msg = LLMMessage{
				Role: "user",
				Content: []ContentBlock{
					{Type: "tool_result", ToolUseID: mt.ToolCallId(), Text: content, IsError: mt.IsError()},
				},
			}

		// Handle extension Message type (from extensions package)
		default:
			// Check if it's an extensions.Message using type assertion
			if extMsg, ok := m.(struct {
				Role   string
				Content []struct {
					Type string
					Text string
				}
			}); ok {
				var content interface{}
				if len(extMsg.Content) == 1 && extMsg.Content[0].Type == "text" {
					content = extMsg.Content[0].Text
				} else {
					blocks := make([]ContentBlock, len(extMsg.Content))
					for i, c := range extMsg.Content {
						blocks[i] = ContentBlock{Type: c.Type, Text: c.Text}
					}
					content = blocks
				}
				msg = LLMMessage{
					Role:    extMsg.Role,
					Content: content,
				}
			}
		}

		if msg.Role != "" {
			result = append(result, msg)
		}
	}

	return result
}

// CreateBranchSummaryMessage creates a BranchSummaryMessage with TS-aligned fields.
func CreateBranchSummaryMessage(summary, fromID string, timestamp int64) *BranchSummaryMessage {
	return &BranchSummaryMessage{
		summary:   summary,
		fromId:    fromID,
		timestamp: timestamp,
	}
}

// CreateCompactionSummaryMessage creates a CompactionSummaryMessage with TS-aligned fields.
func CreateCompactionSummaryMessage(summary string, tokensBefore int, timestamp int64) *CompactionSummaryMessage {
	return &CompactionSummaryMessage{
		summary:      summary,
		tokensBefore: tokensBefore,
		timestamp:    timestamp,
	}
}

// CreateCustomMessage creates a CustomMessage with TS-aligned fields.
func CreateCustomMessage(customType, content string, display bool, details any, timestamp int64) *CustomMessage {
	return &CustomMessage{
		customType: customType,
		content:    content,
		display:    display,
		details:    details,
		timestamp:  timestamp,
	}
}
