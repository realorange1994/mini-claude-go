package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// CompactContext performs intelligent compaction with multi-phase degradation.
// Returns true if any compaction was performed.
//
// Degradation chain:
//
//	Phase 1: Compact - round-based, keeps last N rounds, omits the rest
//	Phase 2: SmartCompact - turn-based, keeps first 2 + last 2 turns
//	Phase 3: SelectiveCompact - clears readable tool outputs, preserves write/exec
//	Phase 4: Hard truncate - fallback to AggressiveTruncateHistory
func (c *ConversationContext) CompactContext() bool {
	msgs, toolNames := c.entriesToCompactionMessages()
	if len(msgs) == 0 {
		return false
	}

	cfg := DefaultCompactionConfig()
	if !NeedsCompaction(msgs, cfg) {
		return false
	}

	// Phase 1: Compact (round-based, keeps last KeepRounds rounds)
	result, err := Compact(msgs, cfg)
	if err == nil && result.OmittedCount > 0 && !NeedsCompaction(result.Messages, cfg) {
		c.entries = compactionMessagesToEntries(result.Messages, toolNames)
		fmt.Fprintf(os.Stderr, "\n  [compact] %s\n", result.Summary())
		return true
	}

	// Phase 2: SmartCompact (turn-based, keeps first 2 + last 2 turns)
	smart := SmartCompact(msgs, 2, 2)
	if smart.CollapsedTurns > 0 && !NeedsCompaction(smart.Messages, cfg) {
		c.entries = compactionMessagesToEntries(smart.Messages, toolNames)
		fmt.Fprintf(os.Stderr, "\n  [compact] SmartCompact: %d turns collapsed\n", smart.CollapsedTurns)
		return true
	}

	// Phase 3: SelectiveCompact (clear readable tool outputs)
	rounds := groupMessagesByRound(msgs)
	compactable := defaultCompactableTools()
	sel := SelectiveCompact(rounds, compactable, "[content omitted to save context]")
	if sel.Compacted > 0 {
		flat := flattenRounds(sel.Rounds)
		c.entries = compactionMessagesToEntries(flat, toolNames)
		fmt.Fprintf(os.Stderr, "\n  [compact] SelectiveCompact: %d rounds cleared, saved ~%d tokens\n", sel.Compacted, sel.Saved)
		return true
	}

	// Phase 4: Hard truncate (last resort)
	fmt.Fprintf(os.Stderr, "\n  [compact] Compaction insufficient, hard truncating\n")
	c.AggressiveTruncateHistory()
	return true
}

// entriesToCompactionMessages converts internal conversation entries to the
// compact.go message format. Returns the messages and a map of tool names
// indexed by message index (for tool call/result rounds).
func (c *ConversationContext) entriesToCompactionMessages() ([]CompactionMessage, map[string]string) {
	msgs := make([]CompactionMessage, 0, len(c.entries))
	toolNames := make(map[string]string) // key: message index as string

	for idx, entry := range c.entries {
		key := fmt.Sprintf("%d", idx)
		switch v := entry.content.(type) {
		case string:
			msgs = append(msgs, CompactionMessage{
				Role:      entry.role,
				Content:   v,
				Timestamp: time.Now().Format(time.RFC3339),
			})

		case []anthropic.ContentBlockParamUnion:
			// Tool calls from assistant
			content, toolUseID, toolName := serializeContentBlocks(v)
			msg := CompactionMessage{
				Role:      entry.role,
				Content:   content,
				ToolUseID: toolUseID,
				ToolName:  toolName,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			msgs = append(msgs, msg)
			if toolName != "" {
				toolNames[key] = toolName
			}

		case []anthropic.ToolResultBlockParam:
			// Tool results (user role in Anthropic API)
			content, toolUseID, _ := serializeToolResultBlocks(v)
			// Try to extract tool name from the toolNames map by matching toolUseID
			toolName := ""
			for _, m := range msgs {
				if m.ToolUseID == toolUseID && m.ToolName != "" {
					toolName = m.ToolName
					break
				}
			}
			msg := CompactionMessage{
				Role:      entry.role,
				Content:   content,
				ToolUseID: toolUseID,
				ToolName:  toolName,
				Timestamp: time.Now().Format(time.RFC3339),
			}
			msgs = append(msgs, msg)
			if toolName != "" {
				toolNames[key] = toolName
			}
		}
	}

	return msgs, toolNames
}

// compactionMessagesToEntries converts compacted messages back to conversation entries.
func compactionMessagesToEntries(msgs []CompactionMessage, toolNames map[string]string) []conversationEntry {
	entries := make([]conversationEntry, 0, len(msgs))

	for idx, msg := range msgs {
		key := fmt.Sprintf("%d", idx)
		if isToolUseJSON(msg.Content) {
			// Reconstruct tool call blocks
			if blocks, err := deserializeContentBlocks(msg.Content); err == nil {
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: blocks,
				})
				continue
			}
			// Fallback: treat as text
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: msg.Content,
			})
		} else if isToolResultJSON(msg.Content) {
			// Reconstruct tool result blocks
			if results, err := deserializeToolResultBlocks(msg.Content); err == nil {
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: results,
				})
				continue
			}
			// Fallback: treat as text
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: msg.Content,
			})
		} else {
			// Regular text message or omission marker
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: msg.Content,
			})
		}

		// Preserve tool name lookup
		if msg.ToolName != "" {
			toolNames[key] = msg.ToolName
		}
	}

	return entries
}

// AddCompactBoundary inserts a system-role text marker for LLM compaction.
func (c *ConversationContext) AddCompactBoundary(trigger CompactTrigger, preCompactTokens int) {
	text := fmt.Sprintf("[Conversation summary inserted — %d tokens compressed, trigger: %s]", preCompactTokens, trigger)
	c.entries = append(c.entries, conversationEntry{
		role:    "system",
		content: text,
	})
}

// AddSummary inserts a user-role summary message after compaction.
func (c *ConversationContext) AddSummary(content string) {
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: content,
	})
}

// Entries returns the conversation entries (for compactor access).
func (c *ConversationContext) Entries() []conversationEntry {
	return c.entries
}

// ReplaceEntries replaces all conversation entries (used by compactor).
func (c *ConversationContext) ReplaceEntries(entries []conversationEntry) {
	c.entries = entries
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
