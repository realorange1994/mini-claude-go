package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// EntryContent is a sealed interface for conversation entry content types.
// The unexported method prevents external types from implementing it.
type EntryContent interface {
	entryContent()
}

// TextContent represents plain text in a conversation entry.
type TextContent string

func (TextContent) entryContent() {}

// ToolUseContent represents assistant tool_use blocks.
type ToolUseContent []anthropic.ContentBlockParamUnion

func (ToolUseContent) entryContent() {}

// ToolResultContent represents tool result blocks.
type ToolResultContent []anthropic.ToolResultBlockParam

func (ToolResultContent) entryContent() {}

// CompactBoundaryContent represents a compaction boundary marker.
type CompactBoundaryContent struct {
	Trigger           CompactTrigger
	PreCompactTokens  int
}

func (CompactBoundaryContent) entryContent() {}

// SummaryContent represents a conversation summary inserted after compaction.
type SummaryContent string

func (SummaryContent) entryContent() {}

// AttachmentContent represents post-compact recovery content (file/skill re-injection).
type AttachmentContent string

func (AttachmentContent) entryContent() {}

// conversationEntry represents a single entry in the conversation history.
type conversationEntry struct {
	role    string // "user" or "assistant" (or "system" for boundary markers)
	content EntryContent
}

// ConversationContext manages the conversation message history and system prompt.
type ConversationContext struct {
	mu           sync.RWMutex
	config       Config
	entries      []conversationEntry
	systemPrompt string
}

// NewConversationContext creates a new context.
func NewConversationContext(cfg Config) *ConversationContext {
	return &ConversationContext{config: cfg}
}

// EstimatedTokens returns a rough token estimate for all entries (total chars / 4).
func (c *ConversationContext) EstimatedTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	totalChars := 0
	for _, entry := range c.entries {
		switch v := entry.content.(type) {
		case TextContent:
			totalChars += len(v)
		case ToolUseContent:
			for _, b := range v {
				if b.OfText != nil {
					totalChars += len(b.OfText.Text)
				}
				if b.OfToolUse != nil {
					totalChars += len(b.OfToolUse.ID) + len(b.OfToolUse.Name)
					if m, ok := b.OfToolUse.Input.(map[string]any); ok {
						for k, val := range m {
							totalChars += len(k) + len(fmt.Sprintf("%v", val))
						}
					}
				}
			}
		case ToolResultContent:
			for _, r := range v {
				for _, c := range r.Content {
					if c.OfText != nil {
						totalChars += len(c.OfText.Text)
					}
				}
			}
		case CompactBoundaryContent:
			// Boundary markers are small, ignore for estimation
		case SummaryContent:
			totalChars += len(v)
		}
	}
	if totalChars < 4 {
		return 0
	}
	return totalChars / 4
}

// SetSystemPrompt sets the system prompt.
func (c *ConversationContext) SetSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.systemPrompt = prompt
}

// SystemPrompt returns the system prompt.
func (c *ConversationContext) SystemPrompt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.systemPrompt
}

// AddUserMessage appends a user text message.
func (c *ConversationContext) AddUserMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(content),
	})
	c.truncateIfNeeded()
}

// AddAssistantText appends an assistant text message.
func (c *ConversationContext) AddAssistantText(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if text == "" {
		return
	}
	c.entries = append(c.entries, conversationEntry{
		role:    "assistant",
		content: TextContent(text),
	})
	c.truncateIfNeeded()
}

// AddAssistantToolCalls records assistant tool_use blocks.
func (c *ConversationContext) AddAssistantToolCalls(toolCalls []map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		content: ToolUseContent(blocks),
	})
	c.truncateIfNeeded()
}

// AddToolResults appends tool results as a user message.
func (c *ConversationContext) AddToolResults(results []anthropic.ToolResultBlockParam) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: ToolResultContent(results),
	})
	c.truncateIfNeeded()
}

// BuildMessages converts entries to []anthropic.MessageParam for the API.
func (c *ConversationContext) BuildMessages() []anthropic.MessageParam {
	c.mu.RLock()
	defer c.mu.RUnlock()
	messages := make([]anthropic.MessageParam, 0, len(c.entries))
	for _, entry := range c.entries {
		msg := anthropic.MessageParam{Role: anthropic.MessageParamRole(entry.role)}

		switch v := entry.content.(type) {
		case TextContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: string(v)}},
			}
		case ToolUseContent:
			msg.Content = v
		case ToolResultContent:
			blocks := make([]anthropic.ContentBlockParamUnion, len(v))
			for i, r := range v {
				blocks[i] = anthropic.ContentBlockParamUnion{OfToolResult: &r}
			}
			msg.Content = blocks
		case CompactBoundaryContent:
			// Compact boundary: discard all messages before this point.
			// Only the summary + messages after the boundary are sent to the API.
			// This is the key mechanism that makes compaction actually reduce
			// token usage — without this reset, old messages would still be
			// included and compaction would be a no-op.
			messages = messages[:0]
			continue
		case SummaryContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: string(v)}},
			}
		case AttachmentContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: string(v)}},
			}
		default:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: ""}},
			}
		}

		messages = append(messages, msg)
	}
	return messages
}

// must hold c.mu write lock
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

		// After truncation, validate tool pairing and fix role alternation.
		// Naive slice truncation can orphan tool_results and leave
		// consecutive same-role messages, both causing API error 2013.
		c.ValidateToolPairing()
		c.FixRoleAlternation()
	}
}

// TruncateHistory drops older messages to recover from context overflow.
// Keeps the first entry (initial user message) and the last 10 entries.
func (c *ConversationContext) TruncateHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) <= 12 {
		return
	}
	keep := 10
	first := c.entries[:1]
	recent := c.entries[len(c.entries)-keep:]
	c.entries = append(first, recent...)
	c.ValidateToolPairing()
	c.FixRoleAlternation()
}

// AggressiveTruncateHistory drops more aggressively - keeps only first and last 5.
func (c *ConversationContext) AggressiveTruncateHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aggressiveTruncateHistory()
}

// must hold c.mu write lock
func (c *ConversationContext) aggressiveTruncateHistory() {
	if len(c.entries) <= 6 {
		return
	}
	keep := 5
	first := c.entries[:1]
	recent := c.entries[len(c.entries)-keep:]
	c.entries = append(first, recent...)
	c.ValidateToolPairing()
	c.FixRoleAlternation()
}

// MinimumHistory drops to bare minimum - only first user message and last 2 entries.
func (c *ConversationContext) MinimumHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) <= 3 {
		return
	}
	first := c.entries[:1]
	recent := c.entries[len(c.entries)-2:]
	c.entries = append(first, recent...)
	c.ValidateToolPairing()
	c.FixRoleAlternation()
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
	c.mu.Lock()
	defer c.mu.Unlock()
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
		c.ValidateToolPairing()
		c.FixRoleAlternation()
		fmt.Fprintf(os.Stderr, "\n  [compact] %s\n", result.Summary())
		return true
	}

	// Phase 2: SmartCompact (turn-based, keeps first 2 + last 2 turns)
	smart := SmartCompact(msgs, 2, 2)
	if smart.CollapsedTurns > 0 && !NeedsCompaction(smart.Messages, cfg) {
		c.entries = compactionMessagesToEntries(smart.Messages, toolNames)
		c.ValidateToolPairing()
		c.FixRoleAlternation()
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
		c.ValidateToolPairing()
		c.FixRoleAlternation()
		fmt.Fprintf(os.Stderr, "\n  [compact] SelectiveCompact: %d rounds cleared, saved ~%d tokens\n", sel.Compacted, sel.Saved)
		return true
	}

	// Phase 4: Hard truncate (last resort)
	fmt.Fprintf(os.Stderr, "\n  [compact] Compaction insufficient, hard truncating\n")
	c.aggressiveTruncateHistory()
	return true
}

// entriesToCompactionMessages converts internal conversation entries to the
// compact.go message format. Returns the messages and a map of tool names
// indexed by message index (for tool call/result rounds).
// must hold c.mu at least read lock
func (c *ConversationContext) entriesToCompactionMessages() ([]CompactionMessage, map[string]string) {
	msgs := make([]CompactionMessage, 0, len(c.entries))
	toolNames := make(map[string]string) // key: message index as string

	for idx, entry := range c.entries {
		key := fmt.Sprintf("%d", idx)
		switch v := entry.content.(type) {
		case TextContent:
			msgs = append(msgs, CompactionMessage{
				Role:      entry.role,
				Content:   string(v),
				Timestamp: time.Now().Format(time.RFC3339),
			})

		case ToolUseContent:
			// Tool calls from assistant
			content, toolUseID, toolName := serializeContentBlocks([]anthropic.ContentBlockParamUnion(v))
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

		case ToolResultContent:
			// Tool results (user role in Anthropic API)
			content, toolUseID, _ := serializeToolResultBlocks([]anthropic.ToolResultBlockParam(v))
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

		case CompactBoundaryContent:
			// Compact boundary: discard all messages before this point.
			// This matches BuildMessages() behavior where the boundary resets
			// the message list. Only entries AFTER the boundary are sent to
			// the compactor, preventing re-compaction of already-compacted content.
			msgs = msgs[:0]
			toolNames = make(map[string]string)
		case SummaryContent:
			msgs = append(msgs, CompactionMessage{
				Role:      entry.role,
				Content:   string(v),
				Timestamp: time.Now().Format(time.RFC3339),
			})
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
					content: ToolUseContent(blocks),
				})
				continue
			}
			// Fallback: treat as text
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: TextContent(msg.Content),
			})
		} else if isToolResultJSON(msg.Content) {
			// Reconstruct tool result blocks
			if results, err := deserializeToolResultBlocks(msg.Content); err == nil {
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: ToolResultContent(results),
				})
				continue
			}
			// Fallback: treat as text
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: TextContent(msg.Content),
			})
		} else {
			// Regular text message or omission marker
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: TextContent(msg.Content),
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role: "system",
		content: CompactBoundaryContent{
			Trigger:          trigger,
			PreCompactTokens: preCompactTokens,
		},
	})
}

// AddSummary inserts a user-role summary message after compaction.
func (c *ConversationContext) AddSummary(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: SummaryContent(content),
	})
}

// AddAttachment inserts a user-role attachment message after compaction.
// Used for post-compact recovery of file content, skill content, etc.
func (c *ConversationContext) AddAttachment(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: AttachmentContent(content),
	})
}

// AddHistorySnip preserves the most recent conversation entries verbatim
// after compaction. This ensures the latest context is not lost in the summary.
// Entries are added as user-role text messages with a [history-snip] prefix.
// skipPaths contains file paths recovered by PostCompactRecovery; ToolResultContent
// entries referencing those paths are skipped to avoid duplication.
func (c *ConversationContext) AddHistorySnip(count int, skipPaths []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count <= 0 {
		count = 3
	}

	// Find the most recent CompactBoundaryContent
	boundaryIdx := -1
	for i := len(c.entries) - 1; i >= 0; i-- {
		if _, ok := c.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}
	if boundaryIdx < 0 {
		return
	}

	// Collect up to 'count' entries before the boundary
	var snipEntries []conversationEntry
	for i := boundaryIdx - 1; i >= 0 && len(snipEntries) < count; i-- {
		entry := c.entries[i]
		switch entry.content.(type) {
		case CompactBoundaryContent, SummaryContent, AttachmentContent:
			continue
		default:
			// Skip ToolResultContent entries that reference recovered file paths
			if skipPaths != nil && len(skipPaths) > 0 {
				if results, ok := entry.content.(ToolResultContent); ok {
					skip := false
					for _, r := range results {
						for _, contentBlock := range r.Content {
							if contentBlock.OfText != nil {
								for _, path := range skipPaths {
									if strings.Contains(contentBlock.OfText.Text, path) {
										skip = true
										break
									}
								}
							}
							if skip {
								break
							}
						}
						if skip {
							break
						}
					}
					if skip {
						continue
					}
				}
			}
			snipEntries = append([]conversationEntry{entry}, snipEntries...)
		}
	}

	// Append snip entries after the boundary as preserved messages
	for _, entry := range snipEntries {
		text := entryContentToText(entry.content)
		if text == "" {
			continue
		}
		c.entries = append(c.entries, conversationEntry{
			role:    "user",
			content: TextContent(fmt.Sprintf("[history-snip %s] %s", entry.role, text)),
		})
	}
}

// compactableToolNames is the set of tool names whose results should be cleared
// during micro-compaction. These are read/search/web/write tools where the raw
// output is large and not needed for context after the turn passes.
// Tools like git, memory, skill, list_dir, etc. are NOT compacted because their
// results contain structural information the model may need later.
var compactableToolNames = map[string]bool{
	"read_file":    true,
	"exec":         true,
	"edit_file":    true,
	"write_file":   true,
	"multi_edit":   true,
	"grep":         true,
	"glob":         true,
	"web_fetch":    true,
	"web_search":   true,
}

// MicroCompactEntries clears content of old tool results beyond the keepRecent
// window. Operates directly on conversation entries (no serialization round-trip).
// Returns the number of tool result entries that were cleared.
// ToolUseID is preserved in cleared results to maintain pairing validity.
//
// Two improvements over the original:
//  1. Dedup: skips tool results already cleared to the placeholder string.
//  2. Whitelist: only clears results from compactable tools (read/exec/edit/grep/glob/web/write).
func (c *ConversationContext) MicroCompactEntries(keepRecent int, placeholder string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if keepRecent <= 0 {
		keepRecent = 5
	}
	if placeholder == "" {
		placeholder = "[Old tool result content cleared]"
	}

	// Pass 1: Build tool_use_id -> tool_name mapping from ToolUseContent entries.
	toolNameMap := make(map[string]string) // tool_use_id -> tool_name
	for _, entry := range c.entries {
		blocks, ok := entry.content.(ToolUseContent)
		if !ok {
			continue
		}
		for _, b := range blocks {
			if b.OfToolUse != nil && b.OfToolUse.ID != "" {
				toolNameMap[b.OfToolUse.ID] = b.OfToolUse.Name
			}
		}
	}

	// Pass 2: Iterate backwards, clearing eligible tool results.
	recentCount := 0
	cleared := 0
	for i := len(c.entries) - 1; i >= 0; i-- {
		entry := &c.entries[i]
		results, ok := entry.content.(ToolResultContent)
		if !ok {
			continue
		}
		if recentCount < keepRecent {
			recentCount++
			continue
		}

		// Check each block: is it already cleared? is it a compactable tool?
		allCleared := true
		hasCompactable := false
		for _, r := range results {
			// Check if already cleared to placeholder
			alreadyCleared := false
			for _, c := range r.Content {
				if c.OfText != nil && c.OfText.Text == placeholder {
					alreadyCleared = true
					break
				}
			}
			if !alreadyCleared {
				allCleared = false
			}

			// Check if this tool is compactable
			if toolName, ok := toolNameMap[r.ToolUseID]; ok && compactableToolNames[toolName] {
				hasCompactable = true
			}
		}

		// Skip if all blocks are already cleared, or none are compactable
		if allCleared || !hasCompactable {
			continue
		}

		// Clear only compactable tool results; leave others untouched
		var clearedResults []anthropic.ToolResultBlockParam
		for _, r := range results {
			if toolName, ok := toolNameMap[r.ToolUseID]; ok && compactableToolNames[toolName] {
				clearedResults = append(clearedResults, anthropic.ToolResultBlockParam{
					ToolUseID: r.ToolUseID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: placeholder}},
					},
					IsError: r.IsError,
				})
			} else {
				// Not a compactable tool — keep the original result
				clearedResults = append(clearedResults, r)
			}
		}
		entry.content = ToolResultContent(clearedResults)
		cleared++
	}
	return cleared
}

// Entries returns the conversation entries (for compactor access).
func (c *ConversationContext) Entries() []conversationEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries
}

// BuildCompactTranscript satisfies the TranscriptSource interface for the
// auto mode classifier. Delegates to the standalone function in transcript_builder.go.
func (c *ConversationContext) BuildCompactTranscript(maxMessages int) string {
	return BuildCompactTranscript(c, maxMessages)
}

// Len returns the number of conversation entries.
func (c *ConversationContext) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Clear removes all conversation entries.
func (c *ConversationContext) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// ReplaceEntries replaces all conversation entries (used by compactor).
func (c *ConversationContext) ReplaceEntries(entries []conversationEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

// ValidateToolPairing validates bidirectional tool_use/tool_result pairing.
// Handles two failure modes after truncation:
// 1. Orphaned tool_results: result references a tool_use that was removed → delete result
// 2. Orphaned tool_uses: tool_use has no matching result (result was truncated) →
//    insert stub result or delete the tool_use block
// This prevents Anthropic API error 2013.
// must hold c.mu write lock
func (c *ConversationContext) ValidateToolPairing() {
	// Pass 1: Collect all tool_use IDs from assistant messages
	callIDs := make(map[string]bool)
	for _, entry := range c.entries {
		if entry.role != "assistant" {
			continue
		}
		if blocks, ok := entry.content.(ToolUseContent); ok {
			for _, b := range blocks {
				if b.OfToolUse != nil && b.OfToolUse.ID != "" {
					callIDs[b.OfToolUse.ID] = true
				}
			}
		}
	}

	// Pass 2: Remove orphaned tool_results (those without matching tool_use).
	// This is the critical fix for 2013 error: "tool call result does not follow tool call".
	// We ONLY remove orphaned tool_results. We do NOT remove tool_use blocks without
	// results (Pass 3 removed) — removing tool_use blocks while leaving tool_results
	// in the API's view causes a worse structural mismatch.
	resultIDs := make(map[string]bool)
	for i, entry := range c.entries {
		if entry.role == "user" {
			if results, ok := entry.content.(ToolResultContent); ok {
				var kept []anthropic.ToolResultBlockParam
				for _, r := range results {
					if callIDs[r.ToolUseID] {
						kept = append(kept, r)
						resultIDs[r.ToolUseID] = true
					}
				}
				if len(kept) == 0 {
					c.entries[i].content = nil // mark for removal
				} else {
					c.entries[i].content = ToolResultContent(kept)
				}
			}
		}
	}

	// Remove nil entries (fully orphaned tool_result messages)
	compacted := make([]conversationEntry, 0, len(c.entries))
	for _, e := range c.entries {
		if e.content != nil {
			compacted = append(compacted, e)
		}
	}
	c.entries = compacted

	// Pass 3 (REMOVED): Previously removed tool_use blocks without matching results.
	// This was counterproductive: when the API returns 2013, it's because a
	// tool_result doesn't match a tool_use. Removing the tool_use block makes
	// the orphaned tool_result even more orphaned, worsening the mismatch.
	// Instead, we keep tool_use blocks intact and only remove orphaned results.
	// The API will handle any tool_use without results gracefully.
}

// FixRoleAlternation ensures strict user/assistant alternation by merging
// consecutive messages with the same role. Critical for Anthropic API
// compliance after naive slice truncation.
// must hold c.mu write lock
func (c *ConversationContext) FixRoleAlternation() {
	if len(c.entries) == 0 {
		return
	}

	var merged []conversationEntry
	for _, entry := range c.entries {
		// Skip system messages — they are boundary markers
		if entry.role == "system" {
			merged = append(merged, entry)
			continue
		}

		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if last.role == entry.role {
				// Merge same-role consecutive messages
				switch a := last.content.(type) {
				case TextContent:
					if b, ok := entry.content.(TextContent); ok {
						last.content = TextContent(string(a) + "\n\n" + string(b))
						continue
					}
				case ToolUseContent:
					if b, ok := entry.content.(ToolUseContent); ok {
						last.content = append(a, b...)
						continue
					}
				case ToolResultContent:
					if b, ok := entry.content.(ToolResultContent); ok {
						last.content = append(a, b...)
						continue
					}
				}
				// Type mismatch - keep entries separate instead of converting.
				// CRITICAL: Never convert ToolResultContent to TextContent.
				// Doing so destroys the tool_use/tool_result pairing, causing 2013.
				// Just fall through to append entry at line 887.
		}
	}
	merged = append(merged, entry)
}
c.entries = merged
}

// entryContentToText serializes any EntryContent to a plain text string.
// Used by FixRoleAlternation to handle type-mismatched same-role entries.
func entryContentToText(c EntryContent) string {
	switch v := c.(type) {
	case TextContent:
		return string(v)
	case ToolUseContent:
		var parts []string
		for _, b := range v {
			if b.OfText != nil {
				parts = append(parts, b.OfText.Text)
			}
			if b.OfToolUse != nil {
				name := b.OfToolUse.Name
				id := b.OfToolUse.ID
				parts = append(parts, fmt.Sprintf("[tool call %s: %s]", id, name))
			}
		}
		return strings.Join(parts, " ")
	case ToolResultContent:
		var parts []string
		for _, r := range v {
			id := r.ToolUseID
			for _, c := range r.Content {
				if c.OfText != nil {
					text := c.OfText.Text
					if len(text) > 500 {
						text = text[:500] + "..."
					}
					parts = append(parts, fmt.Sprintf("[result %s: %s]", id, text))
				}
			}
		}
		return strings.Join(parts, " ")
	case CompactBoundaryContent:
		return fmt.Sprintf("[compaction boundary: %d tokens]", v.PreCompactTokens)
	case SummaryContent:
		return string(v)
	case AttachmentContent:
		return string(v)
	default:
		return ""
	}
}
