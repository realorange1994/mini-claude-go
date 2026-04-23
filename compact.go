// Package main - context compaction for long coding sessions.
//
// Provides:
//   - Token estimation (~4 chars per token heuristic)
//   - Context limit detection (configurable threshold, default 75%)
//   - Message grouping by API round (user+assistant pairs)
//   - Safe compression boundaries (tool_call/tool_result pairs kept together)
//   - Archive old messages to a history file
//   - Boundary markers inserted for omitted messages
//   - Conversion between SDK types and CompactionMessage
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// --- Token estimation --------------------------------------------------------

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

// --- Constants ---------------------------------------------------------------

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

// --- Compaction message (internal, SDK-agnostic representation) --------------

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

// --- Compaction result -------------------------------------------------------

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

// --- Compaction config -------------------------------------------------------

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

// --- Context usage tracker ---------------------------------------------------

// ContextUsage tracks token usage over time for compaction decisions.
type ContextUsage struct {
	mu             sync.Mutex
	history        []usageSnapshot
	maxContextSize int
}

type usageSnapshot struct {
	Timestamp time.Time
	Tokens    int
	Model     string
}

// NewContextUsage creates a new usage tracker.
func NewContextUsage(maxContextSize int) *ContextUsage {
	return &ContextUsage{maxContextSize: maxContextSize}
}

// Record logs a usage snapshot.
func (u *ContextUsage) Record(tokens int, model string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.history = append(u.history, usageSnapshot{
		Timestamp: time.Now(),
		Tokens:    tokens,
		Model:     model,
	})
	// Keep only the last 100 snapshots
	if len(u.history) > 100 {
		u.history = u.history[len(u.history)-100:]
	}
}

// UsageFraction returns the current context usage as a fraction of max.
func (u *ContextUsage) UsageFraction(tokens int) float64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.maxContextSize <= 0 {
		return 0
	}
	return float64(tokens) / float64(u.maxContextSize)
}

// --- Round grouping ----------------------------------------------------------

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

// --- Safe boundary detection -------------------------------------------------

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

// --- Token counting for messages ---------------------------------------------

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

// --- Compaction logic --------------------------------------------------------

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

// --- Archiving ---------------------------------------------------------------

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

	// Sort by modification time (simple insertion sort, small N expected)
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].ModTime().Before(files[j-1].ModTime()); j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}

	return files, nil
}

// --- Selective compaction (per-round) ----------------------------------------

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

// --- Micro-compaction (time-based content clearing) --------------------------

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

// --- Smart compaction (turn-based) ------------------------------------------

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

	// Group into turns - each turn captures all messages in one user+assistant exchange,
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

// --- Summary -----------------------------------------------------------------

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

// serializeContentBlocks serializes []anthropic.ContentBlockParamUnion to a JSON string.
// Returns the JSON content and extracted tool name/ID if present.
func serializeContentBlocks(blocks []anthropic.ContentBlockParamUnion) (content string, toolUseID string, toolName string) {
	data, err := json.Marshal(blocks)
	if err != nil {
		content = "{}"
		return
	}
	content = string(data)
	for _, b := range blocks {
		if b.OfToolUse != nil {
			toolUseID = b.OfToolUse.ID
			toolName = b.OfToolUse.Name
			break
		}
	}
	return
}

// serializeToolResultBlocks serializes []anthropic.ToolResultBlockParam to a JSON string.
func serializeToolResultBlocks(results []anthropic.ToolResultBlockParam) (content string, toolUseID string, toolName string) {
	data, err := json.Marshal(results)
	if err != nil {
		content = "{}"
		return
	}
	content = string(data)
	for _, r := range results {
		if r.ToolUseID != "" {
			toolUseID = r.ToolUseID
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
