// Package main -- context compaction for long coding sessions.
// Self-contained, no external dependencies beyond the standard library.
//
// Provides:
//   - Token estimation (~4 chars per token heuristic)
//   - Context limit detection (configurable threshold, default 75%)
//   - Message grouping by API round (user+assistant pairs)
//   - Safe compression boundaries (tool_call/tool_result pairs kept together)
//   - Archive old messages to a history file
//   - Boundary markers inserted for omitted messages
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ─── Token estimation ────────────────────────────────────────────────────────

// CharsPerToken is the heuristic ratio of characters to tokens.
// Claude and similar models average ~3.5-4.5 chars/token; 4 is a good midpoint.
const CharsPerToken = 4

// EstimateTokens returns an approximate token count for the given text.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / CharsPerToken))
}

// EstimateContentTokens estimates tokens based on content type.
// Different content types have different chars/token ratios:
//   - Code: 3.5 chars/token (denser, more special tokens)
//   - Natural language: 4 chars/token (default)
//   - JSON/structured: 3 chars/token (lots of delimiters)
//   - tool_use blocks: 3 chars/token + 10 overhead
//   - tool_result blocks: 3 chars/token + 5 overhead
func EstimateContentTokens(text string, contentType string) int {
	charsPerToken := 4.0 // default: natural language
	switch contentType {
	case "code":
		charsPerToken = 3.5
	case "json":
		charsPerToken = 3.0
	case "tool_use":
		charsPerToken = 3.0
	case "tool_result":
		charsPerToken = 3.0
	}
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / charsPerToken))
}

// DetectContentType heuristically detects content type for token estimation.
func DetectContentType(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		// Likely JSON
		return "json"
	}
	// Check for code indicators
	codeIndicators := []string{"func ", "func(", "var ", "const ", "type ", "struct ", "impl ", "fn ", "class ", "def ", "import ", "package "}
	for _, ind := range codeIndicators {
		if strings.Contains(text, ind) {
			return "code"
		}
	}
	return "natural"
}

// ─── Constants ───────────────────────────────────────────────────────────────

const (
	// DefaultCompactionThreshold is the fraction of max context at which
	// compaction is recommended (75%).
	DefaultCompactionThreshold = 0.75

	// DefaultKeepRounds is the number of recent API rounds to always preserve.
	DefaultKeepRounds = 3

	// DefaultMaxContextTokens is the default max context window (Claude 200K).
	DefaultMaxContextTokens = 200_000

	// OmissionMarker is the format string for boundary markers.
	OmissionMarker = "<!-- %d earlier conversation rounds omitted to save context -->"

	// ArchiveExtension is the file extension for archived context files.
	ArchiveExtension = ".compact.archive.json"
)

// ─── Compaction message (internal, SDK-agnostic representation) ──────────────

// CompactionMessage is a lightweight, serializable representation of a
// conversation message used during compaction. It mirrors the essential
// fields without depending on any external SDK types.
type CompactionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolUseID string `json:"tool_use_id,omitempty"` // tool_call ID for pairing
	ToolName  string `json:"tool_name,omitempty"`   // name of the tool called
	Timestamp string `json:"timestamp,omitempty"`   // ISO-8601 timestamp
}

// ─── Compaction result ───────────────────────────────────────────────────────

// CompactionResult holds the outcome of a compaction operation.
type CompactionResult struct {
	Messages          []CompactionMessage `json:"messages"`
	OmittedCount      int                 `json:"omitted_count"`
	KeptCount         int                 `json:"kept_count"`
	TokensBefore      int                 `json:"tokens_before"`
	TokensAfter       int                 `json:"tokens_after"`
	TokensSaved       int                 `json:"tokens_saved"`
	CompactionRatio   float64             `json:"compaction_ratio"`
	ArchivePath       string              `json:"archive_path,omitempty"`
	CompactionTrigger string              `json:"compaction_trigger"`
}

// ─── Compaction config ───────────────────────────────────────────────────────

// CompactionConfig holds all tunable parameters.
type CompactionConfig struct {
	MaxContextTokens  int     // Maximum context window in tokens
	Threshold         float64 // Fraction of max context that triggers compaction (0.0-1.0)
	KeepRounds        int     // Number of recent rounds to always keep
	ArchiveDir        string  // Directory for archive files (empty = skip archiving)
	ClearPlaceholder  string  // Placeholder for cleared content (empty = hard omit)
	OmissionMarker    string  // Format string for boundary markers (empty = none)
}

// DefaultCompactionConfig returns sensible defaults.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		MaxContextTokens: DefaultMaxContextTokens,
		Threshold:        DefaultCompactionThreshold,
		KeepRounds:       DefaultKeepRounds,
		ArchiveDir:       "",
		ClearPlaceholder: "",
		OmissionMarker:   OmissionMarker,
	}
}

// ─── Round grouping ──────────────────────────────────────────────────────────

// apiRound represents one user+assistant exchange (may include tool calls).
type apiRound struct {
	indices    []int              // original message indices in this round
	messages   []CompactionMessage
	totalChars int
	isToolCall bool               // true if this round contains tool_call/tool_result
	toolPairID string             // tool_use_id for pairing tool_call with result
	ToolName   string             // name of the tool used in this round (if any)
}

// groupMessagesByRound groups a flat message list into API rounds.
// A round starts with a "user" message and includes all following
// "assistant" messages until the next "user" message.
// Tool call/result pairs are kept together within a round.
func groupMessagesByRound(messages []CompactionMessage) []apiRound {
	if len(messages) == 0 {
		return nil
	}

	var rounds []apiRound
	var current apiRound

	for i, msg := range messages {
		if msg.Role == "user" {
			// Flush previous round if it exists
			if len(current.messages) > 0 {
				rounds = append(rounds, current)
			}
			current = apiRound{
				indices:  []int{i},
				messages: []CompactionMessage{msg},
			}
			// Check if this user message contains tool results
			current.isToolCall = strings.Contains(msg.Content, "tool_result") ||
				strings.Contains(msg.Content, "tool_use_id")
			if id := extractToolResultID(msg); id != "" {
				current.toolPairID = id
			}
			if msg.ToolName != "" && current.ToolName == "" {
				current.ToolName = msg.ToolName
			}
		} else if msg.Role == "assistant" {
			current.indices = append(current.indices, i)
			current.messages = append(current.messages, msg)
			// Check if this assistant message contains tool calls
			if strings.Contains(msg.Content, "tool_use") || msg.ToolUseID != "" {
				current.isToolCall = true
				if msg.ToolUseID != "" {
					current.toolPairID = msg.ToolUseID
				}
				if msg.ToolName != "" && current.ToolName == "" {
					current.ToolName = msg.ToolName
				}
			}
		} else {
			// System messages get their own round
			if len(current.messages) > 0 {
				rounds = append(rounds, current)
			}
			current = apiRound{
				indices:  []int{i},
				messages: []CompactionMessage{msg},
			}
		}
		current.totalChars += len(msg.Content)
	}

	// Flush last round
	if len(current.messages) > 0 {
		rounds = append(rounds, current)
	}

	return rounds
}

// extractToolResultID tries to find a tool_use_id in a message's content.
func extractToolResultID(msg CompactionMessage) string {
	if msg.ToolUseID != "" {
		return msg.ToolUseID
	}
	// Look for tool_use_id in JSON-like content
	idx := strings.Index(msg.Content, `"tool_use_id"`)
	if idx == -1 {
		return ""
	}
	// Try to extract the value after the key
	rest := msg.Content[idx:]
	colonIdx := strings.Index(rest, ":")
	if colonIdx == -1 {
		return ""
	}
	rest = rest[colonIdx+1:]
	rest = strings.TrimSpace(rest)
	if len(rest) < 2 || rest[0] != '"' {
		return ""
	}
	end := strings.Index(rest[1:], `"`)
	if end == -1 {
		return ""
	}
	return rest[1 : 1+end]
}

// ─── Safe boundary detection ─────────────────────────────────────────────────

// findSafeCompactionBoundary finds the index in the rounds slice where we can
// safely split: before this index can be compacted, from this index onward
// should be kept. It ensures tool_call/tool_result pairs are not split.
//
// Parameters:
//   rounds: grouped API rounds
//   keepN: number of recent rounds to always keep
//
// Returns the round index where the "keep" region starts.
func findSafeCompactionBoundary(rounds []apiRound, keepN int) int {
	if len(rounds) <= keepN+1 {
		return 0 // Not enough rounds to compact
	}

	// Start from the beginning; we want to compact the oldest rounds
	// but ensure we don't split a tool_call/tool_result pair.
	cutPoint := len(rounds) - keepN

	// Walk forward from cutPoint to ensure we don't split a tool pair
	for cutPoint < len(rounds) {
		// Check if this round is part of an uncompleted tool pair
		if cutPoint > 0 && rounds[cutPoint].isToolCall {
			// Check if the previous round started a tool call that this round completes
			if rounds[cutPoint-1].isToolCall && rounds[cutPoint-1].toolPairID != "" {
				// This round completes a tool pair; move cutPoint back to include the pair
				if rounds[cutPoint].toolPairID == rounds[cutPoint-1].toolPairID {
					cutPoint--
					continue
				}
			}
		}
		break
	}

	// Ensure we never compact the system message (usually round 0)
	if cutPoint == 0 && len(rounds) > 1 {
		// Check if round 0 is a system message
		if len(rounds[0].messages) > 0 && rounds[0].messages[0].Role == "system" {
			cutPoint = 1
		}
	}

	return cutPoint
}

// ─── Token counting for messages ─────────────────────────────────────────────

// messageTokens returns estimated token count for a single message.
func messageTokens(msg CompactionMessage) int {
	tokens := EstimateTokens(msg.Content)
	// Add overhead for role and metadata
	if msg.Role != "" {
		tokens += 2
	}
	if msg.ToolUseID != "" {
		tokens += 4
	}
	if msg.ToolName != "" {
		tokens += 2
	}
	return tokens
}

// roundTokens returns estimated token count for a round.
func roundTokens(round apiRound) int {
	total := 0
	for _, msg := range round.messages {
		total += messageTokens(msg)
	}
	return total
}

// totalTokens returns estimated token count for all messages.
func totalTokens(messages []CompactionMessage) int {
	total := 0
	for _, msg := range messages {
		total += messageTokens(msg)
	}
	return total
}

// ─── Compaction logic ────────────────────────────────────────────────────────

// NeedsCompaction checks whether the current message set is approaching
// the context limit and should be compacted.
func NeedsCompaction(messages []CompactionMessage, cfg CompactionConfig) bool {
	if cfg.MaxContextTokens <= 0 {
		return false
	}
	tokens := totalTokens(messages)
	fraction := float64(tokens) / float64(cfg.MaxContextTokens)
	return fraction >= cfg.Threshold
}

// ContextInfo returns human-readable info about current context usage.
func ContextInfo(messages []CompactionMessage, maxTokens int) string {
	tokens := totalTokens(messages)
	fraction := 0.0
	if maxTokens > 0 {
		fraction = float64(tokens) / float64(maxTokens)
	}
	return fmt.Sprintf("Context: %d tokens / %d max (%.1f%%), %d messages",
		tokens, maxTokens, fraction*100, len(messages))
}

// Compact performs context compaction on the given messages.
//
// It:
//  1. Groups messages into API rounds
//  2. Finds a safe compaction boundary
//  3. Archives old rounds if ArchiveDir is set
//  4. Replaces compacted rounds with an omission marker
//  5. Returns the compacted message list and stats
func Compact(messages []CompactionMessage, cfg CompactionConfig) (*CompactionResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to compact")
	}

	tokensBefore := totalTokens(messages)
	rounds := groupMessagesByRound(messages)
	if len(rounds) == 0 {
		return nil, fmt.Errorf("no rounds found after grouping")
	}

	cutPoint := findSafeCompactionBoundary(rounds, cfg.KeepRounds)
	if cutPoint == 0 {
		// Nothing to compact
		return &CompactionResult{
			Messages:          messages,
			OmittedCount:      0,
			KeptCount:         len(messages),
			TokensBefore:      tokensBefore,
			TokensAfter:       tokensBefore,
			TokensSaved:       0,
			CompactionRatio:   1.0,
			CompactionTrigger: "none_needed",
		}, nil
	}

	// Count omitted rounds/messages
	omittedRounds := rounds[:cutPoint]
	keptRounds := rounds[cutPoint:]
	omittedMsgCount := 0
	for _, r := range omittedRounds {
		omittedMsgCount += len(r.messages)
	}

	// Archive if configured
	archivePath := ""
	if cfg.ArchiveDir != "" && len(omittedRounds) > 0 {
		var err error
		archivePath, err = archiveRounds(cfg.ArchiveDir, omittedRounds)
		if err != nil {
			// Log but don't fail; archiving is best-effort
			archivePath = fmt.Sprintf("archive_error: %v", err)
		}
	}

	// Build compacted message list
	var result []CompactionMessage

	// Insert omission marker
	if cfg.OmissionMarker != "" && omittedMsgCount > 0 {
		result = append(result, CompactionMessage{
			Role:      "system",
			Content:   fmt.Sprintf(cfg.OmissionMarker, omittedMsgCount),
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	// Append kept rounds
	for _, r := range keptRounds {
		result = append(result, r.messages...)
	}

	tokensAfter := totalTokens(result)
	tokensSaved := tokensBefore - tokensAfter
	compactionRatio := 1.0
	if tokensBefore > 0 {
		compactionRatio = float64(tokensAfter) / float64(tokensBefore)
	}

	trigger := "token_threshold"
	if cfg.KeepRounds > 0 {
		trigger = fmt.Sprintf("keep_last_%d_rounds", cfg.KeepRounds)
	}

	return &CompactionResult{
		Messages:          result,
		OmittedCount:      omittedMsgCount,
		KeptCount:         len(result),
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
		TokensSaved:       tokensSaved,
		CompactionRatio:   compactionRatio,
		ArchivePath:       archivePath,
		CompactionTrigger: trigger,
	}, nil
}

// ─── Archiving ───────────────────────────────────────────────────────────────

// archiveRounds writes the omitted rounds to a JSON file in the archive dir.
// Returns the file path.
func archiveRounds(archiveDir string, rounds []apiRound) (string, error) {
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return "", fmt.Errorf("create archive dir: %w", err)
	}

	// Flatten rounds back to messages
	var messages []CompactionMessage
	for _, r := range rounds {
		messages = append(messages, r.messages...)
	}

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal archive: %w", err)
	}

	timestamp := time.Now().Format("20060102T150405")
	filename := fmt.Sprintf("context%s", ArchiveExtension)
	// Use timestamped file to avoid overwrites
	fullName := fmt.Sprintf("context-%s%s", timestamp, ArchiveExtension)
	path := filepath.Join(archiveDir, fullName)

	// Check if timestamped file already exists; fall back to non-timestamped
	if _, err := os.Stat(path); err == nil {
		path = filepath.Join(archiveDir, filename)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write archive: %w", err)
	}

	return path, nil
}

// LoadArchive reads a previously archived context file.
func LoadArchive(path string) ([]CompactionMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}

	var messages []CompactionMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("unmarshal archive: %w", err)
	}

	return messages, nil
}

// ListArchives returns archive files in the given directory, sorted by
// modification time (oldest first).
func ListArchives(archiveDir string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return nil, fmt.Errorf("read archive dir: %w", err)
	}

	var files []os.FileInfo
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ArchiveExtension) {
			info, err := e.Info()
			if err == nil {
				files = append(files, info)
			}
		}
	}

	// Sort by modification time
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	return files, nil
}

// ─── Selective compaction (per-round) ────────────────────────────────────────

// SelectiveCompactResult holds the result of selective compaction.
type SelectiveCompactResult struct {
	Rounds    []apiRound
	Compacted int
	Saved     int
}

// SelectiveCompact selectively compacts individual rounds that are marked
// as compactable (e.g., read-only tool results), while preserving rounds
// that contain write/exec tool calls.
//
// compactableTools is a set of tool names whose results can be compacted.
// If empty, all tool-call rounds are considered compactable.
func SelectiveCompact(rounds []apiRound, compactableTools map[string]bool, placeholder string) *SelectiveCompactResult {
	if len(rounds) == 0 {
		return &SelectiveCompactResult{Rounds: rounds}
	}
	if placeholder == "" {
		placeholder = "[content omitted to save context]"
	}

	result := &SelectiveCompactResult{
		Rounds: make([]apiRound, len(rounds)),
	}
	copy(result.Rounds, rounds)

	for i := range result.Rounds {
		r := &result.Rounds[i]
		if !r.isToolCall {
			continue
		}
		// Check if this round's tool is compactable
		if len(compactableTools) > 0 && r.ToolName != "" {
			if !compactableTools[r.ToolName] {
				continue // Non-compactable tool, preserve
			}
		}
		// Compact the content
		for j := range r.messages {
			if r.messages[j].Role != "system" {
				origTokens := EstimateTokens(r.messages[j].Content)
				r.messages[j].Content = placeholder
				result.Compacted++
				result.Saved += origTokens
			}
		}
	}

	return result
}

// ─── Micro-compaction (time-based content clearing) ──────────────────────────

// MicroCompactConfig holds micro-compaction settings.
type MicroCompactConfig struct {
	Enabled           bool
	GapThresholdMins  float64
	KeepRecent        int
	ClearPlaceholder  string
}

// DefaultMicroCompactConfig returns sensible defaults.
func DefaultMicroCompactConfig() MicroCompactConfig {
	return MicroCompactConfig{
		Enabled:          true,
		GapThresholdMins: 60.0,
		KeepRecent:       5,
		ClearPlaceholder: "[Old tool result content cleared]",
	}
}

// MicroCompact clears content of old compactable tool-result messages
// that are older than the gap threshold.
func MicroCompact(messages []CompactionMessage, cfg MicroCompactConfig) []CompactionMessage {
	if !cfg.Enabled || len(messages) == 0 {
		return messages
	}

	now := time.Now()
	result := make([]CompactionMessage, len(messages))
	copy(result, messages)

	// Count recent compactable messages (iterate in reverse)
	recentCount := 0
	for i := len(result) - 1; i >= 0; i-- {
		msg := &result[i]
		if !isCompactableToolResult(msg) {
			continue
		}
		if recentCount < cfg.KeepRecent {
			recentCount++
			continue
		}
		// Check time gap
		ts := parseTimestamp(msg.Timestamp)
		if !ts.IsZero() && now.Sub(ts).Minutes() >= cfg.GapThresholdMins {
			msg.Content = cfg.ClearPlaceholder
		}
	}

	return result
}

// isCompactableToolResult returns true if the message looks like a
// tool result that could be compacted (heuristic based on content).
func isCompactableToolResult(msg *CompactionMessage) bool {
	if msg.Role != "user" {
		return false
	}
	content := msg.Content
	// Heuristic: tool results typically contain tool_use_id or tool_result markers
	return strings.Contains(content, "tool_result") ||
		strings.Contains(content, "tool_use_id") ||
		strings.Contains(content, "output")
}

// parseTimestamp parses an ISO-8601 timestamp string.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try without timezone
	t, err = time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		return t
	}
	// Try date only
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t
	}
	return time.Time{}
}

// ─── Smart compaction (turn-based) ──────────────────────────────────────────

// SmartCompactResult holds the result of smart (turn-based) compaction.
type SmartCompactResult struct {
	Messages     []CompactionMessage
	KeptTurns    int
	CollapsedTurns int
}

// SmartCompact groups messages into conversational turns, keeps the first
// N and last M turns, and collapses the middle turns with a marker.
func SmartCompact(messages []CompactionMessage, keepFirst int, keepLast int) *SmartCompactResult {
	if len(messages) == 0 {
		return &SmartCompactResult{}
	}
	if keepFirst <= 0 {
		keepFirst = 2
	}
	if keepLast <= 0 {
		keepLast = 2
	}

	// Extract system message if present
	var systemMsgs []CompactionMessage
	var conversational []CompactionMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemMsgs = append(systemMsgs, m)
		} else {
			conversational = append(conversational, m)
		}
	}

	// Group into turns -- each turn captures all messages in one user+assistant exchange,
	// including tool_result (user role) and tool_use (assistant role) messages.
	type turn struct {
		messages []CompactionMessage
	}
	var turns []turn
	var cur turn
	for _, m := range conversational {
		if m.Role == "user" && len(cur.messages) > 0 {
			// New user message starts a new turn (unless previous was also user i.e. tool_result)
			lastRole := cur.messages[len(cur.messages)-1].Role
			if lastRole == "assistant" {
				turns = append(turns, cur)
				cur = turn{}
			}
		}
		cur.messages = append(cur.messages, m)
	}
	if len(cur.messages) > 0 {
		turns = append(turns, cur)
	}

	totalTurns := len(turns)
	collapsed := 0
	if totalTurns <= keepFirst+keepLast {
		// Keep all turns
		result := make([]CompactionMessage, 0, len(systemMsgs)+len(conversational))
		result = append(result, systemMsgs...)
		result = append(result, conversational...)
		return &SmartCompactResult{
			Messages:     result,
			KeptTurns:    totalTurns,
			CollapsedTurns: 0,
		}
	}

	collapsed = totalTurns - keepFirst - keepLast

	// Build result
	var result []CompactionMessage
	result = append(result, systemMsgs...)

	// First N turns
	for i := 0; i < keepFirst && i < len(turns); i++ {
		result = append(result, turns[i].messages...)
	}

	// Omission marker
	if collapsed > 0 {
		result = append(result, CompactionMessage{
			Role:    "system",
			Content: fmt.Sprintf(OmissionMarker, collapsed),
		})
	}

	// Last M turns
	for i := totalTurns - keepLast; i < totalTurns; i++ {
		if i < 0 {
			continue
		}
		result = append(result, turns[i].messages...)
	}

	return &SmartCompactResult{
		Messages:     result,
		KeptTurns:    keepFirst + keepLast,
		CollapsedTurns: collapsed,
	}
}

// ─── Summary ─────────────────────────────────────────────────────────────────

// Summary returns a human-readable summary of the compaction result.
func (r *CompactionResult) Summary() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Compaction: %d messages -> %d messages ",
		r.OmittedCount+r.KeptCount, r.KeptCount))
	b.WriteString(fmt.Sprintf("(%d tokens -> %d tokens, saved %d) ",
		r.TokensBefore, r.TokensAfter, r.TokensSaved))
	b.WriteString(fmt.Sprintf("ratio: %.1f%%", r.CompactionRatio*100))
	if r.ArchivePath != "" {
		b.WriteString(fmt.Sprintf(" | archived: %s", r.ArchivePath))
	}
	return b.String()
}

// ─── Conversion between conversationEntry and CompactionMessage ──────────────

// serializeContentBlockParamUnion serializes []anthropic.ContentBlockParamUnion to a JSON string.
// Returns the JSON content and extracted tool name/ID if present.
func serializeContentBlocks(blocks []anthropic.ContentBlockParamUnion) (content string, toolUseID string, toolName string) {
	data, err := json.Marshal(blocks)
	if err != nil {
		content = "{}"
		return
	}
	content = string(data)
	// Extract tool info
	for _, b := range blocks {
		if b.OfToolUse != nil {
			toolUseID = b.OfToolUse.ID
			toolName = b.OfToolUse.Name
			break
		}
	}
	return
}

// serializeToolResultBlockParam serializes []anthropic.ToolResultBlockParam to a JSON string.
func serializeToolResultBlocks(results []anthropic.ToolResultBlockParam) (content string, toolUseID string, toolName string) {
	data, err := json.Marshal(results)
	if err != nil {
		content = "{}"
		return
	}
	content = string(data)
	// Extract tool use ID and name
	for _, r := range results {
		if r.ToolUseID != "" {
			toolUseID = r.ToolUseID
			// Try to extract tool name from content
			for _, c := range r.Content {
				if c.OfText != nil && strings.Contains(c.OfText.Text, toolUseID) {
					// Tool name not available in result, leave empty
				}
			}
			break
		}
	}
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

// flattenRounds converts []apiRound back to a flat []CompactionMessage list.
func flattenRounds(rounds []apiRound) []CompactionMessage {
	var msgs []CompactionMessage
	for _, r := range rounds {
		msgs = append(msgs, r.messages...)
	}
	return msgs
}

// defaultCompactableTools returns tool names whose output can be safely
// cleared during selective compaction. Read-only tools are compactable;
// write/exec tools must be preserved.
func defaultCompactableTools() map[string]bool {
	return map[string]bool{
		"read_file":  true,
		"glob":       true,
		"grep":       true,
		"list_dir":   true,
		"web_fetch":  true,
		"web_search": true,
	}
}

// messageRoundParam represents an API round built from MessageParam structs.
type messageRoundParam struct {
	role  string // "system", "user", or "assistant" (of first msg)
	msgs  []anthropic.MessageParam
}

// groupMessageParamsByRound groups []anthropic.MessageParam into API rounds.
// Same structure as groupMessagesByRound but operates on MessageParam types.
func groupMessageParamsByRound(messages []anthropic.MessageParam) []messageRoundParam {
	if len(messages) == 0 {
		return nil
	}

	var rounds []messageRoundParam
	var current messageRoundParam
	firstMsg := true

	for _, msg := range messages {
		role := string(msg.Role)
		if role == "user" || firstMsg {
			if len(current.msgs) > 0 {
				rounds = append(rounds, current)
			}
			current = messageRoundParam{role: role, msgs: []anthropic.MessageParam{msg}}
			firstMsg = false
		} else {
			current.msgs = append(current.msgs, msg)
		}
	}
	if len(current.msgs) > 0 {
		rounds = append(rounds, current)
	}
	return rounds
}

// ─── LLM-Driven Compaction ───────────────────────────────────────────────────

// CompactTrigger indicates what triggered a compaction.
type CompactTrigger int

const (
	CompactTriggerAuto CompactTrigger = iota
	CompactTriggerManual
	CompactTriggerSMCompact // SM-compact: uses session memory as summary, no LLM call
)

func (t CompactTrigger) String() string {
	switch t {
	case CompactTriggerAuto:
		return "auto"
	case CompactTriggerManual:
		return "manual"
	case CompactTriggerSMCompact:
		return "sm-compact"
	default:
		return "unknown"
	}
}

// compaction prompts
const compactSystemPrompt = "You are a helpful AI assistant tasked with summarizing conversations."

// NO_TOOLS_PREAMBLE is an aggressive no-tools preamble placed BEFORE the main
// prompt to prevent the model from wasting a turn attempting tool calls.
// On Sonnet 4.6+ adaptive-thinking models, the model sometimes attempts a tool
// call despite weaker trailer instructions. With maxTurns: 1, a denied tool call
// means no text output. Putting this FIRST prevents the wasted turn.
const noToolsPreamble = `CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

`

// NO_TOOLS_TRAILER reinforces the no-tools constraint after the main prompt.
const noToolsTrailer = "\n\nREMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task."

// detailedAnalysisInstructionBase is the analysis instruction for full compaction.
const detailedAnalysisInstructionBase = `Before providing your final summary, wrap your analysis in <analysis> tags to organize your thoughts and ensure you've covered all necessary points. In your analysis process:

1. Chronologically analyze each message and section of the conversation. For each section thoroughly identify:
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Specific details like:
     - file names
     - full code snippets
     - function signatures
     - file edits
   - Errors that you ran into and how you fixed them
   - Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
2. Double-check for technical accuracy and completeness, addressing each required element thoroughly.`

// detailedAnalysisInstructionPartial is the analysis instruction for partial compaction.
const detailedAnalysisInstructionPartial = `Before providing your final summary, wrap your analysis in <analysis> tags to organize your thoughts and ensure you've covered all necessary points. In your analysis process:

1. Analyze the recent messages chronologically. For each section thoroughly identify:
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Specific details like:
     - file names
     - full code snippets
     - function signatures
     - file edits
   - Errors that you ran into and how you fixed them
   - Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
2. Double-check for technical accuracy and completeness, addressing each required element thoroughly.`



// ContextWindowTracker tracks token usage against model-specific context windows.
type ContextWindowTracker struct {
	modelMaxTokens       int
	autoCompactThreshold float64
	autoCompactBuffer    int
}

// NewContextWindowTracker creates a tracker for the given model.
func NewContextWindowTracker(model string, threshold float64, buffer int) *ContextWindowTracker {
	return &ContextWindowTracker{
		modelMaxTokens:       modelContextWindow(model),
		autoCompactThreshold: threshold,
		autoCompactBuffer:    buffer,
	}
}

// modelContextWindow returns the context window size for a model.
func modelContextWindow(model string) int {
	// Default to 200K for all Anthropic models
	return 200_000
}

// EffectiveWindow returns the usable context window minus output reserve.
func (t *ContextWindowTracker) EffectiveWindow() int {
	return t.modelMaxTokens - 20_000 // reserve 20K for output
}

// CompactThreshold returns the token count at which compaction should trigger.
func (t *ContextWindowTracker) CompactThreshold() int {
	effective := t.EffectiveWindow()
	threshold := int(float64(effective) * t.autoCompactThreshold)
	buf := effective - t.autoCompactBuffer
	if threshold < buf {
		return threshold
	}
	return buf
}

// ShouldCompact checks if the current message count exceeds the compaction threshold.
func (t *ContextWindowTracker) ShouldCompact(messages []anthropic.MessageParam) bool {
	tokens := estimateMessageParamsTokens(messages)
	return tokens >= t.CompactThreshold()
}

// estimateMessageParamsTokens estimates tokens for API message params.
func estimateMessageParamsTokens(messages []anthropic.MessageParam) int {
	total := 0
	for _, msg := range messages {
		// Role overhead
		total += 3
		for _, block := range msg.Content {
			if block.OfText != nil {
				total += EstimateTokens(block.OfText.Text)
			}
			if block.OfToolUse != nil {
				total += 10 // overhead
				total += EstimateTokens(block.OfToolUse.Name)
				if data, err := json.Marshal(block.OfToolUse.Input); err == nil {
					total += EstimateTokens(string(data))
				}
			}
			if block.OfToolResult != nil {
				total += 8 // overhead
				for _, c := range block.OfToolResult.Content {
					if c.OfText != nil {
						total += EstimateTokens(c.OfText.Text)
					}
				}
			}
		}
	}
	return total
}

// CompactionResultLLM holds the result of LLM-driven compaction.
type CompactionResultLLM struct {
	BoundaryText      string // system-role compact boundary marker
	SummaryText       string // user-role summary
	PreCompactTokens  int
	PostCompactTokens int
}

// Compactor handles context compaction with LLM-based and fallback strategies.
type Compactor struct {
	mu                    sync.Mutex
	maxTokens             int
	compactThreshold      float64
	compactBuffer         int
	llmCompactFailedCount int
	maxLLMCompactFailures int
	disabled              bool     // permanently disable LLM-driven auto-compact after too many failures; other compaction paths (SM-compact, PartialCompact, CompactContext fallback) remain available
	lastSummary           string   // for iterative summary updates
	lastCompactSavings    []float64 // track savings ratio for anti-thrashing
	postCompactTokens     int      // token count after last compaction, for cooldown
}

// NewCompactor creates a new compactor with default settings.
func NewCompactor() *Compactor {
	return &Compactor{
		maxTokens:             200_000,
		compactThreshold:      0.75,
		compactBuffer:         13_000,
		llmCompactFailedCount: 0,
		maxLLMCompactFailures: 3,
	}
}

// SetPostCompactTokens updates the post-compact token count for cooldown.
// Called by tryCompaction() after injecting boundary + summary, using the
// actual token count of rebuilt messages (summary + tail), not just the
// LLM-estimated summary-only count.
func (c *Compactor) SetPostCompactTokens(tokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.postCompactTokens = tokens
}

// ShouldCompact checks if compaction is needed based on token count and cooldown.
func (c *Compactor) ShouldCompact(messages []anthropic.MessageParam) bool {
	tokens := estimateMessageParamsTokens(messages)
	threshold := int(float64(c.maxTokens) * c.compactThreshold)
	if tokens < threshold {
		return false
	}
	// Cooldown: skip if tokens haven't grown 25% since last compaction
	if c.postCompactTokens > 0 {
		cooldownThreshold := c.postCompactTokens + c.postCompactTokens/4
		if tokens < cooldownThreshold {
			return false
		}
	}
	return true
}

// Compact performs LLM-driven compaction, falling back to truncation on failure.
// Returns true if compaction was performed, false if not needed or fallback used.
// If LLM compaction succeeds, returns the summary text to inject.
// If LLM compaction fails (too many failures), the caller should use existing truncation.
func (c *Compactor) Compact(
	messages []anthropic.MessageParam,
	model string,
	apiKey string,
	baseURL string,
) (summary string, performed bool) {
	c.mu.Lock()
	if c.disabled {
		c.mu.Unlock()
		return "", false
	}
	if c.llmCompactFailedCount >= c.maxLLMCompactFailures {
		c.disabled = true
		c.mu.Unlock()
		return "", false
	}
	c.mu.Unlock()

	if !c.ShouldCompact(messages) {
		return "", false
	}

	// Anti-thrashing: skip if last 2 compactions each saved <10%
	c.mu.Lock()
	if len(c.lastCompactSavings) >= 2 {
		if c.lastCompactSavings[len(c.lastCompactSavings)-1] < 0.10 &&
			c.lastCompactSavings[len(c.lastCompactSavings)-2] < 0.10 {
			c.mu.Unlock()
			return "", false
		}
	}
	c.mu.Unlock()

	result, err := compactConversationLLM(messages, model, apiKey, baseURL, c.lastSummary)
	if err != nil {
		c.mu.Lock()
		c.llmCompactFailedCount++
		if c.llmCompactFailedCount >= c.maxLLMCompactFailures {
			c.disabled = true
			fmt.Fprintf(os.Stderr, "\n[Compaction] LLM auto-compact disabled after %d consecutive failures; other paths (SM-compact, PartialCompact, truncation fallback) remain available\n", c.maxLLMCompactFailures)
		} else {
			fmt.Fprintf(os.Stderr, "\n[Compaction] LLM compaction failed (%d/%d): %v\n", c.llmCompactFailedCount, c.maxLLMCompactFailures, err)
		}
		c.mu.Unlock()
		return "", false
	}

	// Reset failure count on success, update lastSummary, and record savings
	c.mu.Lock()
	c.llmCompactFailedCount = 0
	c.lastSummary = result.SummaryText
	if result.PreCompactTokens > 0 {
		savingsRatio := float64(result.PreCompactTokens-result.PostCompactTokens) / float64(result.PreCompactTokens)
		c.lastCompactSavings = append(c.lastCompactSavings, savingsRatio)
		if len(c.lastCompactSavings) > 2 {
			c.lastCompactSavings = c.lastCompactSavings[len(c.lastCompactSavings)-2:]
		}
	}
	// Set cooldown: record post-compact token count to prevent immediate re-compaction
	c.postCompactTokens = result.PostCompactTokens
	c.mu.Unlock()

	fmt.Fprintf(os.Stderr, "\n[Compaction] auto: %d messages -> 2 (summary), ~%d tokens saved\n",
		len(messages), result.PreCompactTokens-result.PostCompactTokens)

	return result.SummaryText, true
}

// compactConversationLLM calls the LLM API to generate a conversation summary.
// Supports iterative summary updates when a previous summary exists.
// Includes a PTL (prompt-too-long) retry loop: if the compact API call itself
// exceeds the context limit, progressively drop oldest API-round groups and
// retry, up to MAX_PTL_RETRIES times.
const MAX_PTL_RETRIES = 3

func compactConversationLLM(messages []anthropic.MessageParam, model string, apiKey string, baseURL string, previousSummary string) (*CompactionResultLLM, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to compact")
	}

	// PTL retry loop: try compaction, and if the API itself rejects due to
	// prompt-too-long, progressively drop the oldest rounds and retry.
	var lastErr error
	for attempt := 0; attempt <= MAX_PTL_RETRIES; attempt++ {
		result, err := doCompactLLMCall(messages, model, apiKey, baseURL, previousSummary)
		if err == nil {
			return result, nil
		}

		lastErr = err
		errMsg := err.Error()

		if !isContextLengthError(errMsg) {
			// Non-PTL error (auth, timeout, etc.) — bail out
			return nil, err
		}

		// Prompt-too-long: try to drop oldest rounds
		actual, maxTokens, hasGap := parsePromptTooLongTokenGap(errMsg)
		var dropFraction float64
		if hasGap && actual > 0 {
			// Drop just enough rounds to cover the token gap
			needed := actual - maxTokens
			total := estimateMessageParamsTokens(messages)
			if total > 0 {
				dropFraction = float64(needed) / float64(total)
				if dropFraction < 0.20 {
					dropFraction = 0.20 // minimum 20% to make progress
				}
			}
		} else {
			// Gap unparseable: drop 20% fallback
			dropFraction = 0.20
		}

		// Count drops for this attempt
		dropCount := int(float64(len(messages)) * dropFraction)
		if dropCount < 1 {
			dropCount = 1
		}
		if dropCount > len(messages)/2 {
			dropCount = len(messages) / 2 // never drop more than half
		}

		// Group messages by rounds and drop the oldest
		rounds := groupMessageParamsByRound(messages)
		if len(rounds) <= 3 {
			// Too few rounds to drop — give up
			break
		}

		// Find how many rounds to drop (skip system message at round 0)
		dropRounds := 0
		droppedMsgs := 0
		for i := range rounds {
			if i == 0 && len(rounds) > 1 && rounds[0].role == "system" {
				continue // skip system message round
			}
			dropRounds++
			droppedMsgs += len(rounds[i].msgs)
			if droppedMsgs >= dropCount {
				break
			}
		}
		if dropRounds == 0 {
			dropRounds = 1
		}

		// Calculate actual drop offset (skip system round if present)
		startIdx := 0
		if len(rounds) > 1 && rounds[0].role == "system" {
			startIdx = 1
			// Ensure we don't drop all non-system rounds
			if dropRounds >= len(rounds)-2 {
				dropRounds = len(rounds) - 2
			}
		}
		if dropRounds <= 0 || startIdx+dropRounds >= len(rounds)-1 {
			break
		}

		// Build flattened message list without dropped rounds
		var kept []anthropic.MessageParam
		for i, r := range rounds {
			if i >= startIdx && i < startIdx+dropRounds {
				continue // dropped
			}
			kept = append(kept, r.msgs...)
		}
		if len(kept) < 2 {
			break // not enough messages left
		}

		messages = kept
	}

	return nil, fmt.Errorf("compact API error: prompt too long after %d retries, %w", MAX_PTL_RETRIES, lastErr)
}

// doCompactLLMCall performs a single attempt at the LLM compaction API call.
func doCompactLLMCall(messages []anthropic.MessageParam, model string, apiKey string, baseURL string, previousSummary string) (*CompactionResultLLM, error) {
	preTokens := estimateMessageParamsTokens(messages)

	// Apply 3-pass pre-pruning before sending to LLM
	tailBudget := preTokens / 4 // 25% of current tokens as tail budget
	pruned := pruneToolResults(messages, tailBudget)

	// Strip base64 image data and image URLs to save tokens during compaction
	pruned = stripImages(pruned)

	// Redact sensitive information
	for i := range pruned {
		for j := range pruned[i].Content {
			if pruned[i].Content[j].OfText != nil {
				pruned[i].Content[j].OfText.Text = redactSensitiveText(pruned[i].Content[j].OfText.Text)
			}
			if pruned[i].Content[j].OfToolResult != nil {
				for k := range pruned[i].Content[j].OfToolResult.Content {
					if pruned[i].Content[j].OfToolResult.Content[k].OfText != nil {
						pruned[i].Content[j].OfToolResult.Content[k].OfText.Text = redactSensitiveText(pruned[i].Content[j].OfToolResult.Content[k].OfText.Text)
					}
				}
			}
		}
	}

	// Choose prompt based on whether we have a previous summary.
	// All prompts are wrapped with NO_TOOLS_PREAMBLE + NO_TOOLS_TRAILER
	// to prevent the model from wasting a turn on tool calls.
	var userPrompt string
	if previousSummary != "" {
		userPrompt = noToolsPreamble + strings.Replace(iterativeCompactUserPrompt, "{previous_summary}", previousSummary, 1) + noToolsTrailer
	} else {
		userPrompt = noToolsPreamble + structuredCompactUserPrompt + noToolsTrailer
	}

	// Append the summary prompt as the final user message
	finalMsgs := make([]anthropic.MessageParam, len(pruned)+1)
	copy(finalMsgs, pruned)
	finalMsgs[len(pruned)] = anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: userPrompt}},
		},
	}

	opts := []option.RequestOption{
		option.WithHeader("Authorization", "Bearer "+apiKey),
		option.WithHeader("anthropic-version", "2023-06-01"),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 20000,
		System: []anthropic.TextBlockParam{
			{Text: compactSystemPrompt},
		},
		Messages: finalMsgs,
	}, option.WithHTTPClient(&http.Client{
		Timeout: 60 * time.Second,
	}))
	if err != nil {
		return nil, fmt.Errorf("compact API error: %w", err)
	}

	// Extract summary text
	var summaryText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			summaryText += block.Text
		}
	}
	if summaryText == "" {
		return nil, fmt.Errorf("no summary text in response")
	}
	summaryText = extractSummaryFromCompactOutput(summaryText)

	boundaryText := fmt.Sprintf("[Previous conversation summary (%d tokens compressed)]", preTokens)
	// Match upstream's getCompactUserSummaryMessage: wrap the summary with
	// session continuation header and a "continue without asking questions"
	// instruction to prevent the model from re-executing historical tasks.
	summaryContent := fmt.Sprintf(
		"This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n"+
			"%s\n\n%s\n\n"+
			"Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened.",
		boundaryText, summaryText,
	)

	postTokens := EstimateTokens(boundaryText) + EstimateTokens(summaryText) + 6 // role overhead

	return &CompactionResultLLM{
		BoundaryText:      boundaryText,
		SummaryText:       summaryContent,
		PreCompactTokens:  preTokens,
		PostCompactTokens: postTokens,
	}, nil
}

// ─── Content-type-aware token estimation for API messages ────────────────────

// EstimateMessageTokensSmart estimates tokens for a message using content-type
// detection for more accurate per-block estimation.
func EstimateMessageTokensSmart(msg anthropic.MessageParam) int {
	total := 4 // role overhead per message
	for _, block := range msg.Content {
		if block.OfText != nil {
			contentType := DetectContentType(block.OfText.Text)
			total += EstimateContentTokens(block.OfText.Text, contentType)
		}
		if block.OfToolUse != nil {
			total += 10 // tool_use overhead
			total += EstimateContentTokens(block.OfToolUse.Name, "code")
			if data, err := json.Marshal(block.OfToolUse.Input); err == nil {
				total += EstimateContentTokens(string(data), "json")
			}
		}
		if block.OfToolResult != nil {
			total += 5 // tool_result overhead
			for _, c := range block.OfToolResult.Content {
				if c.OfText != nil {
					contentType := DetectContentType(c.OfText.Text)
					total += EstimateContentTokens(c.OfText.Text, contentType)
				}
			}
		}
	}
	return total
}

// ─── Three-pass pre-pruning ──────────────────────────────────────────────────

// PruneToolCallInfo stores info about a tool call for pre-pruning.
type PruneToolCallInfo struct {
	ToolName string
	Args     any
}

// PruneToolCallIndex maps tool_use_id -> PruneToolCallInfo
type PruneToolCallIndex map[string]PruneToolCallInfo

// buildToolCallIndex extracts tool call info from assistant messages.
func buildToolCallIndex(messages []anthropic.MessageParam) PruneToolCallIndex {
	index := make(PruneToolCallIndex)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolUse != nil {
				index[block.OfToolUse.ID] = PruneToolCallInfo{
					ToolName: block.OfToolUse.Name,
					Args:     block.OfToolUse.Input,
				}
			}
		}
	}
	return index
}

// pruneToolResults performs 3-pass pre-pruning on messages before LLM compaction.
// Returns a modified copy of messages with:
//
//	Pass 1: Deduplicate identical tool results (FNV-1a hash)
//	Pass 2: Summarize old tool results beyond tail protection
//	Pass 3: Truncate large tool_call arguments
func pruneToolResults(messages []anthropic.MessageParam, tailBudget int) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}

	result := make([]anthropic.MessageParam, len(messages))
	copy(result, messages)

	// Pass 1: Deduplicate tool results
	result = dedupToolResults(result)

	// Pass 2: Summarize old tool results
	index := buildToolCallIndex(result)
	result = summarizeOldToolResults(result, index, tailBudget)

	// Pass 3: Truncate large tool_call arguments
	result = truncateLargeToolArgs(result, 2000) // 2000 char limit per arg

	return result
}

// dedupToolResults replaces duplicate tool results with a reference marker.
func dedupToolResults(messages []anthropic.MessageParam) []anthropic.MessageParam {
	seen := make(map[string]string) // hash -> first tool_use_id

	for i := range messages {
		for j := range messages[i].Content {
			block := &messages[i].Content[j]
			if block.OfToolResult == nil {
				continue
			}
			// Hash the content
			contentHash := hashToolResultContent(block.OfToolResult)
			toolUseID := block.OfToolResult.ToolUseID

			if firstID, exists := seen[contentHash]; exists && firstID != toolUseID {
				// Duplicate - replace with reference
				block.OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: fmt.Sprintf("[duplicate result, see tool_use_id %s]", firstID)}},
				}
			} else {
				seen[contentHash] = toolUseID
			}
		}
	}
	return messages
}

// hashToolResultContent creates a simple FNV-1a hash of tool result content.
func hashToolResultContent(result *anthropic.ToolResultBlockParam) string {
	var sb strings.Builder
	for _, c := range result.Content {
		if c.OfText != nil {
			sb.WriteString(c.OfText.Text)
		}
	}
	// Simple FNV-1a hash
	h := uint32(2166136261)
	for _, b := range sb.String() {
		h ^= uint32(b)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

// summarizeOldToolResults replaces old tool results with one-line summaries.
func summarizeOldToolResults(messages []anthropic.MessageParam, index PruneToolCallIndex, tailBudget int) []anthropic.MessageParam {
	// Find tail cut point by token budget
	cutPoint := findTailCutByTokens(messages, tailBudget)

	for i := 0; i < cutPoint && i < len(messages); i++ {
		for j := range messages[i].Content {
			block := &messages[i].Content[j]
			if block.OfToolResult == nil {
				continue
			}
			toolUseID := block.OfToolResult.ToolUseID
			info, exists := index[toolUseID]
			if !exists {
				continue
			}
			// Generate one-line summary
			summary := generateToolResultSummary(info.ToolName, block.OfToolResult)
			block.OfToolResult.Content = []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: summary}},
			}
		}
	}
	return messages
}

// generateToolResultSummary creates a one-line summary for a tool result.
func generateToolResultSummary(toolName string, result *anthropic.ToolResultBlockParam) string {
	// Extract content text
	var contentText string
	for _, c := range result.Content {
		if c.OfText != nil {
			contentText += c.OfText.Text
		}
	}

	lines := strings.Count(contentText, "\n") + 1
	isErr := result.IsError.Valid() && result.IsError.Value

	status := "ok"
	if isErr {
		status = "error"
	}

	// Truncate content for summary
	preview := contentText
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	preview = strings.ReplaceAll(preview, "\n", " ")

	return fmt.Sprintf("[%s] -> %s, %d lines output", toolName, status, lines)
}

// truncateLargeToolArgs truncates large arguments in tool_use blocks.
func truncateLargeToolArgs(messages []anthropic.MessageParam, maxArgChars int) []anthropic.MessageParam {
	for i := range messages {
		for j := range messages[i].Content {
			block := &messages[i].Content[j]
			if block.OfToolUse == nil {
				continue
			}
			// Truncate large input values
			inputMap, ok := block.OfToolUse.Input.(map[string]any)
			if !ok {
				continue
			}
			modified := false
			newMap := make(map[string]any, len(inputMap))
			for k, v := range inputMap {
				if s, ok := v.(string); ok && len(s) > maxArgChars {
					newMap[k] = s[:maxArgChars] + "...[truncated]"
					modified = true
				} else {
					newMap[k] = v
				}
			}
			if modified {
				block.OfToolUse.Input = newMap
			}
		}
	}
	return messages
}

// ─── Token-budget tail protection ────────────────────────────────────────────

// findTailCutByTokens walks backward from the end of messages accumulating
// tokens until the tail budget is reached. Returns the index where the
// "keep" region starts. Ensures:
//   - At least 3 messages are always protected
//   - The most recent user message is always in the tail
//   - Tool_call/result pairs are not split
func findTailCutByTokens(messages []anthropic.MessageParam, tailTokenBudget int) int {
	if len(messages) <= 3 {
		return 0
	}

	accumulated := 0
	cutPoint := len(messages)

	// Walk backward
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := estimateSingleMessageTokens(messages[i])
		accumulated += msgTokens

		if accumulated >= tailTokenBudget && i <= len(messages)-3 {
			cutPoint = i
			break
		}
	}

	// Ensure at least 3 messages in tail
	if cutPoint > len(messages)-3 {
		cutPoint = len(messages) - 3
	}

	// Ensure most recent user message is in tail
	for i := len(messages) - 1; i >= cutPoint; i-- {
		if messages[i].Role == anthropic.MessageParamRoleUser {
			// Check this isn't a tool_result user message at the boundary
			break
		}
	}

	// Align boundary: don't split tool_call/result pairs
	// If the message at cutPoint is a tool_result, move back to include the tool_call
	for cutPoint > 0 {
		hasToolResult := false
		for _, block := range messages[cutPoint].Content {
			if block.OfToolResult != nil {
				hasToolResult = true
				break
			}
		}
		if hasToolResult {
			// This is a tool_result; check if previous message has the matching tool_use
			cutPoint--
			continue
		}
		break
	}

	return cutPoint
}

// estimateSingleMessageTokens estimates tokens for a single MessageParam.
func estimateSingleMessageTokens(msg anthropic.MessageParam) int {
	total := 4 // role overhead
	for _, block := range msg.Content {
		if block.OfText != nil {
			total += EstimateTokens(block.OfText.Text)
		}
		if block.OfToolUse != nil {
			total += 10 // tool_use overhead
			total += EstimateTokens(block.OfToolUse.Name)
			if data, err := json.Marshal(block.OfToolUse.Input); err == nil {
				total += EstimateTokens(string(data))
			}
		}
		if block.OfToolResult != nil {
			total += 5 // tool_result overhead
			for _, c := range block.OfToolResult.Content {
				if c.OfText != nil {
					total += EstimateTokens(c.OfText.Text)
				}
			}
		}
	}
	return total
}

// ─── Iterative summary updates ──────────────────────────────────────────────

const structuredCompactUserPrompt = `Your task is to create a detailed summary of the conversation so far, paying close attention to the user's explicit requests and your previous actions.
This summary should be thorough in capturing technical details, code patterns, and architectural decisions that would be essential for continuing development work without losing context.

Before providing your final summary, wrap your analysis in <analysis> tags to organize your thoughts and ensure you've covered all necessary points. In your analysis process:

1. Chronologically analyze each message and section of the conversation. For each section thoroughly identify:
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Specific details like:
     - file names
     - full code snippets
     - function signatures
     - file edits
   - Errors that you ran into and how you fixed them
   - Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
2. Double-check for technical accuracy and completeness, addressing each required element thoroughly.

Your summary should include the following sections:

1. Primary Request and Intent: Capture all of the user's explicit requests and intents in detail
2. Key Technical Concepts: List all important technical concepts, technologies, and frameworks discussed.
3. Files and Code Sections: Enumerate specific files and code sections examined, modified, or created. Pay special attention to the most recent messages and include full code snippets where applicable and include a summary of why this file read or edit is important.
4. Errors and fixes: List all errors that you ran into, and how you fixed them. Pay special attention to specific user feedback that you received, especially if the user told you to do something differently.
5. Problem Solving: Document problems solved and any ongoing troubleshooting efforts.
6. All user messages: List ALL user messages that are not tool results. These are critical for understanding the users' feedback and changing intent.
7. Pending Tasks: Outline any pending tasks that you have explicitly been asked to work on.
8. Current Work: Describe in detail precisely what was being worked on immediately before this summary request, paying special attention to the most recent messages from both user and assistant. Include file names and code snippets where applicable.
9. Optional Next Step: List the next step that you will take that is related to the most recent work you were doing. IMPORTANT: ensure that this step is DIRECTLY in line with the user's most recent explicit requests, and the task you were working on immediately before this summary request. If your last task was concluded, then only list next steps if they are explicitly in line with the users request. Do not start on tangential requests or really old requests that were already completed without confirming with the user first.
                       If there is a next step, include direct quotes from the most recent conversation showing exactly what task you were working on and where you left off. This should be verbatim to ensure there's no drift in task interpretation.

Here's an example of how your output should be structured:

<example>
<analysis>
[Your thought process, ensuring all points are covered thoroughly and accurately]
</analysis>

<summary>
1. Primary Request and Intent:
   [Detailed description]

2. Key Technical Concepts:
   - [Concept 1]
   - [Concept 2]
   - [...]

3. Files and Code Sections:
   - [File Name 1]
      - [Summary of why this file is important]
      - [Summary of the changes made to this file, if any]
      - [Important Code Snippet]
   - [File Name 2]
      - [Important Code Snippet]
   - [...]

4. Errors and fixes:
    - [Detailed description of error 1]:
      - [How you fixed the error]
      - [User feedback on the error if any]
    - [...]

5. Problem Solving:
   [Description of solved problems and ongoing troubleshooting]

6. All user messages:
    - [Detailed non tool use user message]
    - [...]

7. Pending Tasks:
   - [Task 1]
   - [Task 2]
   - [...]

8. Current Work:
   [Precise description of current work]

9. Optional Next Step:
   [Optional Next step to take]

</summary>
</example>

Please provide your summary based on the conversation so far, following this structure and ensuring precision and thoroughness in your response.

There may be additional summarization instructions provided in the included context. If so, remember to follow these instructions when creating the above summary. Examples of instructions include:
<example>
## Compact Instructions
When summarizing the conversation focus on typescript code changes and also remember the mistakes you made and how you fixed them.
</example>

<example>
# Summary instructions
When you are using compact - please focus on test output and code changes. Include file reads verbatim.
</example>
`

// extractSummaryFromCompactOutput strips the <analysis> block and extracts
// the <summary> block from the LLM's compaction response.
// If no <summary> tags are found, returns the full text as-is.
func extractSummaryFromCompactOutput(text string) string {
	// Strip <analysis>...</analysis> entirely
	analysisRe := regexp.MustCompile(`(?s)<analysis>.*?</analysis>`)
	text = analysisRe.ReplaceAllString(text, "")

	// Extract content from <summary>...</summary>
	summaryRe := regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)
	matches := summaryRe.FindStringSubmatch(text)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	// Fallback: if no <summary> tags, return cleaned text
	return strings.TrimSpace(text)
}

const iterativeCompactUserPrompt = `Below is the previous summary followed by new conversation messages. Update the summary by:
- Merging new information into existing fields
- Updating progress on tasks mentioned in the previous summary
- Adding new files, errors, or decisions that appeared in the new messages
- Removing information that is no longer relevant
- Preserving all user messages (add new ones, keep existing ones)
- Preserving code snippets, function signatures, and file edits from the previous summary (do NOT summarize them away -- keep them verbatim)

Previous Summary:
{previous_summary}

Write your analysis in <analysis> tags, then the updated summary in <summary> tags with the same 9-field structure.`

// ─── Partial Compaction (Directional) ────────────────────────────────────────

// PartialCompactDirection indicates which part of the conversation to summarize.
type PartialCompactDirection string

const (
	// PartialCompactUpTo summarizes everything UP TO the pivot index,
	// keeping recent context intact (suffix-preserving).
	PartialCompactUpTo PartialCompactDirection = "up_to"

	// PartialCompactFrom summarizes everything FROM the pivot index forward,
	// keeping early context intact (prefix-preserving).
	PartialCompactFrom PartialCompactDirection = "from"
)

// PartialCompactResult holds the outcome of a partial compaction operation.
type PartialCompactResult struct {
	Summary           string // generated summary of the compacted region
	Direction         PartialCompactDirection
	PivotIndex        int    // index where the split occurred
	MessagesKept      int    // number of messages preserved
	MessagesSummarized int  // number of messages that were summarized
	TokensBefore      int
	TokensAfter       int
	TokensSaved       int
}

// PartialCompact performs directional partial compaction on conversation entries.
//
//   - "up_to": Summarize entries 0..pivotIndex, keep entries pivotIndex..end.
//     This preserves recent context. Use when early conversation is less relevant.
//   - "from": Summarize entries pivotIndex..end (keeping the last N entries),
//     keep entries 0..pivotIndex. This preserves early context (goals, decisions).
//     Use when the end of conversation has redundant tool output.
//
// Both directions preserve tool_use/tool_result pairing integrity by adjusting
// the pivot to avoid splitting pairs.
func (c *ConversationContext) PartialCompact(
	direction PartialCompactDirection,
	pivotIndex int,
	keepTail int, // for "from" direction: number of recent entries to always keep
) (*PartialCompactResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := c.entries
	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries to partially compact")
	}

	// Clamp pivotIndex to valid range
	if pivotIndex < 0 {
		pivotIndex = 0
	}
	if pivotIndex > len(entries) {
		pivotIndex = len(entries)
	}
	if keepTail <= 0 {
		keepTail = 3
	}

	var summarizeEntries []conversationEntry
	var keepEntries []conversationEntry

	switch direction {
	case PartialCompactUpTo:
		// Summarize 0..pivotIndex, keep pivotIndex..end
		// Adjust pivot to avoid splitting tool pairs
		adjustedPivot := adjustPivotForToolPairs(entries, pivotIndex, "up_to")
		if adjustedPivot <= 1 {
			return nil, fmt.Errorf("not enough messages to summarize before pivot")
		}
		summarizeEntries = entries[:adjustedPivot]
		keepEntries = entries[adjustedPivot:]

	case PartialCompactFrom:
		// Summarize pivotIndex..(end-keepTail), keep 0..pivotIndex + last keepTail
		adjustedPivot := adjustPivotForToolPairs(entries, pivotIndex, "from")
		tailStart := len(entries) - keepTail
		if tailStart < 0 {
			tailStart = 0
		}
		if adjustedPivot >= tailStart {
			return nil, fmt.Errorf("pivot too close to end; not enough to summarize")
		}
		// Entries to summarize: pivotIndex..tailStart
		summarizeEntries = entries[adjustedPivot:tailStart]
		if len(summarizeEntries) == 0 {
			return nil, fmt.Errorf("no messages to summarize in from direction")
		}
		// Entries to keep: 0..pivotIndex + tailStart..end
		keepEntries = make([]conversationEntry, 0, adjustedPivot+keepTail)
		keepEntries = append(keepEntries, entries[:adjustedPivot]...)
		keepEntries = append(keepEntries, entries[tailStart:]...)

	default:
		return nil, fmt.Errorf("unknown partial compact direction: %s", direction)
	}

	// Calculate token counts
	tokensBefore := c.estimateEntriesTokens(summarizeEntries)
	if tokensBefore == 0 {
		return nil, fmt.Errorf("no tokens to save in entries to summarize")
	}

	// Generate summary by converting entries to text
	summaryText := entriesToSummaryText(summarizeEntries)

	tokensAfter := EstimateTokens(summaryText)
	tokensSaved := tokensBefore - tokensAfter

	// Build replacement: boundary marker + summary + kept entries
	var newEntries []conversationEntry

	// Insert compact boundary
	newEntries = append(newEntries, conversationEntry{
		role: "system",
		content: CompactBoundaryContent{
			Trigger:          CompactTriggerAuto,
			PreCompactTokens: tokensBefore,
		},
	})

	// Insert summary as user message
	newEntries = append(newEntries, conversationEntry{
		role:    "user",
		content: SummaryContent(fmt.Sprintf(
			"This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n"+
				"[partial-compact: %s, %d tokens compressed]\n\n%s\n\n"+
				"Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened.",
			direction, tokensBefore, summaryText)),
	})

	// Append kept entries
	newEntries = append(newEntries, keepEntries...)

	// Replace context entries
	c.entries = newEntries
	c.ValidateToolPairing()
	c.FixRoleAlternation()

	fmt.Fprintf(os.Stderr, "\n[partial-compact: %s] %d entries summarized, %d kept, ~%d tokens saved\n",
		direction, len(summarizeEntries), len(keepEntries), tokensSaved)

	return &PartialCompactResult{
		Summary:           summaryText,
		Direction:         direction,
		PivotIndex:        pivotIndex,
		MessagesKept:      len(keepEntries),
		MessagesSummarized: len(summarizeEntries),
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
		TokensSaved:       tokensSaved,
	}, nil
}

// adjustPivotForToolPairs adjusts the pivot index to avoid splitting
// tool_use/tool_result pairs. For "up_to", if the pivot lands on a
// tool_result, move it back to include the matching tool_use. For "from",
// if the pivot lands mid-pair, move it forward to complete the pair.
func adjustPivotForToolPairs(entries []conversationEntry, pivot int, direction PartialCompactDirection) int {
	if pivot <= 0 || pivot >= len(entries) {
		return pivot
	}

	// Build tool_use_id map
	toolUseIDs := make(map[string]int) // tool_use_id -> index of ToolUseContent
	for i, entry := range entries {
		if blocks, ok := entry.content.(ToolUseContent); ok {
			for _, b := range blocks {
				if b.OfToolUse != nil && b.OfToolUse.ID != "" {
					toolUseIDs[b.OfToolUse.ID] = i
				}
			}
		}
	}

	if direction == PartialCompactUpTo {
		// If pivot lands on a tool_result, check if its tool_use is before pivot
		// If so, move pivot back to include the tool_use
		for i := pivot; i < len(entries); i++ {
			if results, ok := entries[i].content.(ToolResultContent); ok {
				for _, r := range results {
					if useIdx, ok := toolUseIDs[r.ToolUseID]; ok && useIdx < pivot {
						// tool_use is in summarize region, tool_result would be in keep region
						// Move pivot back to include tool_use (it's already in summarize)
						// No adjustment needed since tool_use is already summarized
					}
				}
			}
		}
		// Check if entry just before pivot is a tool_use whose result is after pivot
		// In that case, move pivot forward to include the result too
		for i := pivot; i < len(entries); i++ {
			if results, ok := entries[i].content.(ToolResultContent); ok {
				for _, r := range results {
					if useIdx, ok := toolUseIDs[r.ToolUseID]; ok && useIdx == pivot-1 {
						// tool_use at pivot-1, tool_result at i (in keep region)
						// Move pivot forward to include this result
						pivot = i + 1
						break
					}
				}
			}
		}
	} else {
		// "from" direction: tool_use in keep region, tool_result in summarize region
		// Move pivot back to include tool_use in summarize too
		for i := pivot - 1; i >= 0; i-- {
			if blocks, ok := entries[i].content.(ToolUseContent); ok {
				for _, b := range blocks {
					if b.OfToolUse != nil {
						// Check if any result in summarize region references this tool_use
						for j := pivot; j < len(entries); j++ {
							if results, ok := entries[j].content.(ToolResultContent); ok {
								for _, r := range results {
									if r.ToolUseID == b.OfToolUse.ID {
										// tool_use at i, tool_result in summarize region
										// Move pivot back to include tool_use
										pivot = i
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return pivot
}

// estimateEntriesTokens estimates token count for a slice of entries.
func (c *ConversationContext) estimateEntriesTokens(entries []conversationEntry) int {
	totalChars := 0
	for _, entry := range entries {
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
				for _, cb := range r.Content {
					if cb.OfText != nil {
						totalChars += len(cb.OfText.Text)
					}
				}
			}
		case SummaryContent:
			totalChars += len(v)
		}
	}
	if totalChars < 4 {
		return 0
	}
	return totalChars / 4
}

// entriesToSummaryText converts entries to a readable summary string.
// For text entries, includes the content (truncated if long).
// For tool entries, includes a one-line description.
func entriesToSummaryText(entries []conversationEntry) string {
	var sb strings.Builder
	turnCount := 0
	toolCallCount := 0
	filesMentioned := make(map[string]bool)

	for _, entry := range entries {
		switch v := entry.content.(type) {
		case TextContent:
			text := string(v)
			if entry.role == "user" {
				turnCount++
				// Include user message content (truncated)
				preview := text
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				sb.WriteString(fmt.Sprintf("User: %s\n", preview))
			} else if entry.role == "assistant" {
				preview := text
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				sb.WriteString(fmt.Sprintf("Assistant: %s\n", preview))
			}
		case ToolUseContent:
			for _, b := range v {
				if b.OfToolUse != nil {
					toolCallCount++
					name := b.OfToolUse.Name
					// Extract file paths from tool arguments
					if m, ok := b.OfToolUse.Input.(map[string]any); ok {
						if path, ok := m["path"].(string); ok {
							filesMentioned[path] = true
						}
					}
					sb.WriteString(fmt.Sprintf("[tool call: %s]\n", name))
				}
			}
		case ToolResultContent:
			for _, r := range v {
				for _, cb := range r.Content {
					if cb.OfText != nil {
						text := cb.OfText.Text
						// Extract key info from tool result
						lines := strings.Count(text, "\n")
						preview := text
						if len(preview) > 100 {
							preview = preview[:100] + "..."
						}
						sb.WriteString(fmt.Sprintf("[tool result: %d lines] %s\n", lines+1, preview))
					}
				}
			}
		}
	}

	// Append summary statistics
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Summary of %d conversation turns with %d tool calls.\n", turnCount, toolCallCount))
	if len(filesMentioned) > 0 {
		files := make([]string, 0, len(filesMentioned))
		for f := range filesMentioned {
			files = append(files, f)
		}
		summary.WriteString(fmt.Sprintf("Files mentioned: %s\n", strings.Join(files, ", ")))
	}
	summary.WriteString("---\n")
	summary.WriteString(sb.String())
	return summary.String()
}

// ─── Reactive Compaction ─────────────────────────────────────────────────────

// ReactiveCompactResult holds the result of a reactive compaction.
type ReactiveCompactResult struct {
	Triggered        bool
	PreTokens        int
	PreviousTokens   int
	TokenDelta       int
	CompactionMethod string // "sm-compact", "partial-compact", or "llm-compact"
}

// CheckReactiveCompact checks if a token spike warrants proactive compaction.
// Returns a non-nil result if compaction should be triggered.
//
// A "token spike" is when the token count has increased significantly
// (delta > threshold) compared to the previous turn. This catches situations
// where a large file read or search result suddenly inflates the context.
func CheckReactiveCompact(currentTokens, previousTokens, threshold int) *ReactiveCompactResult {
	if threshold <= 0 {
		threshold = 5000 // default threshold
	}

	delta := currentTokens - previousTokens
	if delta <= 0 || delta < threshold {
		return nil // No spike detected
	}

	return &ReactiveCompactResult{
		Triggered:      true,
		PreTokens:      currentTokens,
		PreviousTokens: previousTokens,
		TokenDelta:     delta,
	}
}

// ─── Sensitive info redaction ────────────────────────────────────────────────

// Precompiled regex patterns for sensitive info redaction (H-03: avoid compiling per call).
var redactPatterns []struct {
	quotedRe   *regexp.Regexp
	unquotedRe *regexp.Regexp
}

func init() {
	patterns := []string{
		`api_key`, `apikey`, `api-key`,
		`password`, `passwd`, `pass`,
		`secret`, `token`, `credential`,
		`auth`, `private_key`, `access_key`,
	}
	redactPatterns = make([]struct {
		quotedRe   *regexp.Regexp
		unquotedRe *regexp.Regexp
	}, len(patterns))
	for i, pattern := range patterns {
		redactPatterns[i].quotedRe = regexp.MustCompile(
			fmt.Sprintf(`(?i)(%s)\s*[:=]\s*["']([^"']{4,})["']`, pattern))
		redactPatterns[i].unquotedRe = regexp.MustCompile(
			fmt.Sprintf(`(?i)(%s)\s*[:=]\s*(\S{8,})`, pattern))
	}
}

// redactSensitiveText replaces sensitive values in text before sending to LLM.
func redactSensitiveText(text string) string {
	result := text
	for _, p := range redactPatterns {
		result = p.quotedRe.ReplaceAllString(result, `$1: "[REDACTED]"`)
		result = p.unquotedRe.ReplaceAllString(result, `$1: [REDACTED]`)
	}
	return result
}

// stripImages removes base64-encoded image data and image URLs from messages
// before sending to the compaction LLM, saving tokens for irrelevant image content.
func stripImages(messages []anthropic.MessageParam) []anthropic.MessageParam {
	// Pre-compile regexes for base64 images and image URLs
	base64Re := regexp.MustCompile(`data:image/[a-zA-Z0-9+.-]+;base64,[A-Za-z0-9+/=]{10,}`)
	urlRe := regexp.MustCompile(`https?://\S+\.(?:png|jpg|jpeg|gif|webp|svg|bmp|tiff)\b`)

	const placeholder = "[image content stripped]"

	result := make([]anthropic.MessageParam, 0, len(messages))
	for _, msg := range messages {
		mutMsg := msg // copy
		newContent := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Content))
		for _, block := range msg.Content {
			mutBlock := block
			if mutBlock.OfText != nil {
				text := mutBlock.OfText.Text
				text = base64Re.ReplaceAllString(text, placeholder)
				text = urlRe.ReplaceAllString(text, placeholder)
				mutBlock.OfText.Text = text
			}
			if mutBlock.OfToolResult != nil {
				newToolContent := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(mutBlock.OfToolResult.Content))
				for _, tc := range mutBlock.OfToolResult.Content {
					mutTc := tc
					if mutTc.OfText != nil {
						text := mutTc.OfText.Text
						text = base64Re.ReplaceAllString(text, placeholder)
						text = urlRe.ReplaceAllString(text, placeholder)
						mutTc.OfText.Text = text
					}
					newToolContent = append(newToolContent, mutTc)
				}
				mutBlock.OfToolResult.Content = newToolContent
			}
			newContent = append(newContent, mutBlock)
		}
		mutMsg.Content = newContent
		result = append(result, mutMsg)
	}
	return result
}
