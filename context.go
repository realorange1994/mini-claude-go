package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// conversationEntry represents a single entry in the conversation history.
type conversationEntry struct {
	role    string // "user" or "assistant"
	content any    // string, []anthropic.ContentBlockParamUnion, or []anthropic.ToolResultBlockParam
}

// ConversationContext manages the conversation message history and system prompt.
type ConversationContext struct {
	config       Config
	entries      []conversationEntry
	systemPrompt string
}

// NewConversationContext creates a new context.
func NewConversationContext(cfg Config) *ConversationContext {
	return &ConversationContext{config: cfg}
}

// SetSystemPrompt sets the system prompt.
func (c *ConversationContext) SetSystemPrompt(prompt string) {
	c.systemPrompt = prompt
}

// SystemPrompt returns the system prompt.
func (c *ConversationContext) SystemPrompt() string {
	return c.systemPrompt
}

// AddUserMessage appends a user text message.
func (c *ConversationContext) AddUserMessage(content string) {
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: content,
	})
	c.truncateIfNeeded()
}

// AddAssistantText appends an assistant text message.
func (c *ConversationContext) AddAssistantText(text string) {
	if text == "" {
		return
	}
	c.entries = append(c.entries, conversationEntry{
		role:    "assistant",
		content: text,
	})
	c.truncateIfNeeded()
}

// AddAssistantToolCalls records assistant tool_use blocks.
func (c *ConversationContext) AddAssistantToolCalls(toolCalls []map[string]any) {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(toolCalls))
	for _, call := range toolCalls {
		id, _ := call["id"].(string)
		name, _ := call["name"].(string)
		input, _ := call["input"].(map[string]any)

		blocks = append(blocks, anthropic.ContentBlockParamUnion{
			OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    id,
				Name:  name,
				Input: input,
			},
		})
	}
	c.entries = append(c.entries, conversationEntry{
		role:    "assistant",
		content: blocks,
	})
	c.truncateIfNeeded()
}

// AddToolResults appends tool results as a user message.
func (c *ConversationContext) AddToolResults(results []anthropic.ToolResultBlockParam) {
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: results,
	})
	c.truncateIfNeeded()
}

// BuildMessages converts entries to []anthropic.MessageParam for the API.
func (c *ConversationContext) BuildMessages() []anthropic.MessageParam {
	messages := make([]anthropic.MessageParam, 0, len(c.entries))
	for _, entry := range c.entries {
		msg := anthropic.MessageParam{Role: anthropic.MessageParamRole(entry.role)}

		switch v := entry.content.(type) {
		case string:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: v}},
			}
		case []anthropic.ContentBlockParamUnion:
			msg.Content = v
		case []anthropic.ToolResultBlockParam:
			blocks := make([]anthropic.ContentBlockParamUnion, len(v))
			for i, r := range v {
				blocks[i] = anthropic.ContentBlockParamUnion{OfToolResult: &r}
			}
			msg.Content = blocks
		default:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: ""}},
			}
		}

		messages = append(messages, msg)
	}
	return messages
}

func (c *ConversationContext) truncateIfNeeded() {
	maxMsgs := c.config.MaxContextMsgs
	if len(c.entries) > maxMsgs {
		keep := maxMsgs - 1
		if keep < 0 {
			keep = 0
		}
		first := c.entries[:1]
		recent := c.entries[len(c.entries)-keep:]
		// Preserve user/assistant alternation: if the first entry and the
		// first kept-recent entry share the same role, drop the recent one.
		if len(recent) > 0 && first[0].role == recent[0].role {
			recent = recent[1:]
		}
		c.entries = append(first, recent...)
	}
}

// TruncateHistory drops older messages to recover from context overflow.
// Keeps the first entry (initial user message) and the last 10 entries.
func (c *ConversationContext) TruncateHistory() {
	if len(c.entries) <= 12 {
		return
	}
	keep := 10
	first := c.entries[:1]
	recent := c.entries[len(c.entries)-keep:]
	c.entries = append(first, recent...)
}

// AggressiveTruncateHistory drops more aggressively - keeps only first and last 5.
func (c *ConversationContext) AggressiveTruncateHistory() {
	if len(c.entries) <= 6 {
		return
	}
	keep := 5
	first := c.entries[:1]
	recent := c.entries[len(c.entries)-keep:]
	c.entries = append(first, recent...)
}

// MinimumHistory drops to bare minimum - only first user message and last 2 entries.
func (c *ConversationContext) MinimumHistory() {
	if len(c.entries) <= 3 {
		return
	}
	first := c.entries[:1]
	recent := c.entries[len(c.entries)-2:]
	c.entries = append(first, recent...)
}

// LoadProjectInstructions reads CLAUDE.md from the project root.
func LoadProjectInstructions(projectDir string) string {
	if projectDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		projectDir = wd
	}

	p := filepath.Join(projectDir, "CLAUDE.md")
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
