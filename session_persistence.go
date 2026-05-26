package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// SerializedConversation is a JSON-serializable snapshot of a full conversation.
// This enables session persistence: save on exit, restore on next launch.
type SerializedConversation struct {
	SessionID      string `json:"session_id"`
	Model          string `json:"model"`
	PermissionMode string `json:"permission_mode"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	WorkingDir     string `json:"working_dir"`

	Entries       []SerializedEntry `json:"entries"`
	CompressionLevel int             `json:"compression_level"`
	TotalInputTokens  int64          `json:"total_input_tokens"`
	TotalOutputTokens int64          `json:"total_output_tokens"`
	TotalCacheReadTokens int64       `json:"total_cache_read_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
}

// SerializedEntry represents a single conversation entry in serialized form.
// The Type field determines how Content is parsed on restore.
type SerializedEntry struct {
	Role       string          `json:"role"`
	Type       string          `json:"type"`
	Content    json.RawMessage `json:"content"`
	Summarized bool            `json:"summarized"`
}

// sessionsDir returns the directory for session files.
func sessionsDir(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "sessions")
}

// SaveConversation serializes the current conversation state to a JSON file.
// The file is written to {projectDir}/.claude/sessions/{sessionID}.json.
// Returns the path of the saved file, or an error.
func SaveConversation(projectDir, sessionID string, agent *AgentLoop) (string, error) {
	if projectDir == "" || sessionID == "" || agent == nil {
		return "", fmt.Errorf("projectDir, sessionID, and agent are required")
	}

	dir := sessionsDir(projectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create sessions directory: %w", err)
	}

	entries := agent.context.Entries()
	serializedEntries := make([]SerializedEntry, 0, len(entries))
	for _, e := range entries {
		se, err := serializeEntry(e)
		if err != nil {
			// Skip entries that fail to serialize rather than aborting the whole save
			agent.logDebug("[session-persist] skipping entry (role=%s): %v\n", e.role, err)
			continue
		}
		serializedEntries = append(serializedEntries, se)
	}

	snap := SerializedConversation{
		SessionID:      sessionID,
		Model:          agent.config.Model,
		PermissionMode: string(agent.config.PermissionMode),
		CreatedAt:      time.Now().Format(time.RFC3339),
		UpdatedAt:      time.Now().Format(time.RFC3339),
		WorkingDir:     projectDir,
		Entries:        serializedEntries,
		CompressionLevel: agent.context.compressionLevel,
		TotalInputTokens:  agent.totalInputTokens.Load(),
		TotalOutputTokens: agent.totalOutputTokens.Load(),
		TotalCacheReadTokens: agent.totalCacheReadTokens.Load(),
		TotalCacheCreationTokens: agent.totalCacheCreationTokens.Load(),
	}

	// If a file already exists, preserve the original CreatedAt
	existingPath := filepath.Join(dir, sessionID+".json")
	if existingData, err := os.ReadFile(existingPath); err == nil {
		var existing SerializedConversation
		if json.Unmarshal(existingData, &existing) == nil && existing.CreatedAt != "" {
			snap.CreatedAt = existing.CreatedAt
		}
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal conversation: %w", err)
	}

	path := filepath.Join(dir, sessionID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write session file: %w", err)
	}

	return path, nil
}

// LoadConversation reads a serialized conversation from a JSON file.
func LoadConversation(projectDir, sessionID string) (*SerializedConversation, error) {
	dir := sessionsDir(projectDir)
	path := filepath.Join(dir, sessionID+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session file not found: %w", err)
	}

	var snap SerializedConversation
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("failed to parse session file: %w", err)
	}

	return &snap, nil
}

// RestoreConversation restores a serialized conversation into the agent's context.
// It clears existing entries and replaces them with the deserialized ones.
func RestoreConversation(agent *AgentLoop, snap *SerializedConversation) error {
	if agent == nil || snap == nil {
		return fmt.Errorf("agent and snapshot are required")
	}

	entries := make([]conversationEntry, 0, len(snap.Entries))
	for _, se := range snap.Entries {
		entry, err := deserializeEntry(se)
		if err != nil {
			agent.logDebug("[session-persist] skipping entry (role=%s, type=%s): %v\n", se.Role, se.Type, err)
			continue
		}
		entries = append(entries, entry)
	}

	agent.context.ReplaceEntries(entries)
	agent.context.compressionLevel = snap.CompressionLevel

	// Restore token counters
	agent.totalInputTokens.Store(snap.TotalInputTokens)
	agent.totalOutputTokens.Store(snap.TotalOutputTokens)
	agent.totalCacheReadTokens.Store(snap.TotalCacheReadTokens)
	agent.totalCacheCreationTokens.Store(snap.TotalCacheCreationTokens)

	return nil
}

// ListSessions returns all saved sessions sorted by UpdatedAt (most recent first).
func ListSessions(projectDir string) ([]SerializedConversation, error) {
	dir := sessionsDir(projectDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []SerializedConversation
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var snap SerializedConversation
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		sessions = append(sessions, snap)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})

	return sessions, nil
}

// --- EntryContent serialization ---

func serializeEntry(e conversationEntry) (SerializedEntry, error) {
	se := SerializedEntry{
		Role:       e.role,
		Summarized: e.summarized,
	}

	switch c := e.content.(type) {
	case TextContent:
		se.Type = "text"
		data, err := json.Marshal(string(c))
		if err != nil {
			return se, err
		}
		se.Content = data

	case ToolUseContent:
		se.Type = "tool_use"
		// Convert anthropic blocks to JSON-safe format
		blocks := make([]json.RawMessage, 0, len(c))
		for _, b := range c {
			data, err := json.Marshal(b)
			if err != nil {
				continue
			}
			blocks = append(blocks, data)
		}
		data, err := json.Marshal(blocks)
		if err != nil {
			return se, err
		}
		se.Content = data

	case ToolResultContent:
		se.Type = "tool_result"
		blocks := make([]json.RawMessage, 0, len(c))
		for _, b := range c {
			data, err := json.Marshal(b)
			if err != nil {
				continue
			}
			blocks = append(blocks, data)
		}
		data, err := json.Marshal(blocks)
		if err != nil {
			return se, err
		}
		se.Content = data

	case CompactBoundaryContent:
		se.Type = "compact_boundary"
		data, err := json.Marshal(c)
		if err != nil {
			return se, err
		}
		se.Content = data

	case SummaryContent:
		se.Type = "summary"
		data, err := json.Marshal(string(c))
		if err != nil {
			return se, err
		}
		se.Content = data

	case CompressionInstructionContent:
		se.Type = "compression_instruction"
		data, err := json.Marshal(c)
		if err != nil {
			return se, err
		}
		se.Content = data

	case CompressedSummaryContent:
		se.Type = "compressed_summary"
		data, err := json.Marshal(c)
		if err != nil {
			return se, err
		}
		se.Content = data

	case AttachmentContent:
		se.Type = "attachment"
		data, err := json.Marshal(string(c))
		if err != nil {
			return se, err
		}
		se.Content = data

	case AntiReplayContent:
		se.Type = "anti_replay"
		data, err := json.Marshal(string(c))
		if err != nil {
			return se, err
		}
		se.Content = data

	case GoalContent:
		se.Type = "goal"
		data, err := json.Marshal(string(c))
		if err != nil {
			return se, err
		}
		se.Content = data

	default:
		se.Type = "unknown"
		data, err := json.Marshal(fmt.Sprintf("[unserializable content: %T]", c))
		if err != nil {
			return se, err
		}
		se.Content = data
	}

	return se, nil
}

func deserializeEntry(se SerializedEntry) (conversationEntry, error) {
	entry := conversationEntry{
		role:       se.Role,
		summarized: se.Summarized,
	}

	switch se.Type {
	case "text":
		var s string
		if err := json.Unmarshal(se.Content, &s); err != nil {
			return entry, err
		}
		entry.content = TextContent(s)

	case "tool_use":
		var blocks []json.RawMessage
		if err := json.Unmarshal(se.Content, &blocks); err != nil {
			return entry, err
		}
		// Reconstruct anthropic ContentBlockParamUnion from raw JSON
		var toolBlocks []anthropic.ContentBlockParamUnion
		for _, raw := range blocks {
			var b anthropic.ContentBlockParamUnion
			if err := json.Unmarshal(raw, &b); err != nil {
				continue
			}
			toolBlocks = append(toolBlocks, b)
		}
		if len(toolBlocks) > 0 {
			entry.content = ToolUseContent(toolBlocks)
		} else {
			entry.content = ToolUseContent{}
		}

	case "tool_result":
		var blocks []json.RawMessage
		if err := json.Unmarshal(se.Content, &blocks); err != nil {
			return entry, err
		}
		var resultBlocks []anthropic.ToolResultBlockParam
		for _, raw := range blocks {
			var b anthropic.ToolResultBlockParam
			if err := json.Unmarshal(raw, &b); err != nil {
				continue
			}
			resultBlocks = append(resultBlocks, b)
		}
		if len(resultBlocks) > 0 {
			entry.content = ToolResultContent(resultBlocks)
		} else {
			entry.content = ToolResultContent{}
		}

	case "compact_boundary":
		var c CompactBoundaryContent
		if err := json.Unmarshal(se.Content, &c); err != nil {
			return entry, err
		}
		entry.content = c

	case "summary":
		var s string
		if err := json.Unmarshal(se.Content, &s); err != nil {
			return entry, err
		}
		entry.content = SummaryContent(s)

	case "compression_instruction":
		var c CompressionInstructionContent
		if err := json.Unmarshal(se.Content, &c); err != nil {
			return entry, err
		}
		entry.content = c

	case "compressed_summary":
		var c CompressedSummaryContent
		if err := json.Unmarshal(se.Content, &c); err != nil {
			return entry, err
		}
		entry.content = c

	case "attachment":
		var s string
		if err := json.Unmarshal(se.Content, &s); err != nil {
			return entry, err
		}
		entry.content = AttachmentContent(s)

	case "anti_replay":
		var s string
		if err := json.Unmarshal(se.Content, &s); err != nil {
			return entry, err
		}
		entry.content = AntiReplayContent(s)

	case "goal":
		var s string
		if err := json.Unmarshal(se.Content, &s); err != nil {
			return entry, err
		}
		entry.content = GoalContent(s)

	default:
		// Unknown type — store as text placeholder
		entry.content = TextContent(fmt.Sprintf("[unknown content type: %s]", se.Type))
	}

	return entry, nil
}