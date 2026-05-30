package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// generateUUID creates a random UUID for compact boundary markers.
// Used by the transcript, session storage, and QueryEngine to reference
// specific compaction events.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- Tool Result Persistence (matching upstream toolResultStorage.ts) ---

const (
	// PERSISTED_OUTPUT_TAG is the XML tag used to wrap persisted output messages.
	PERSISTED_OUTPUT_TAG         = "<persisted-output>"
	PERSISTED_OUTPUT_CLOSING_TAG = "</persisted-output>"
	// PREVIEW_SIZE_BYTES is the preview size in bytes for the reference message.
	PREVIEW_SIZE_BYTES = 2000
	// TOOL_RESULT_CLEARED_MESSAGE is used when tool result content was cleared without persisting.
	TOOL_RESULT_CLEARED_MESSAGE = "[Old tool result content cleared]"
	// DEFAULT_MAX_RESULT_SIZE_CHARS is the default threshold before persistence kicks in.
	DEFAULT_MAX_RESULT_SIZE_CHARS = 8000
	// MAX_TOOL_RESULTS_PER_MESSAGE_CHARS is the per-message aggregate budget limit.
	MAX_TOOL_RESULTS_PER_MESSAGE_CHARS = 20000
	// SystemInjectedPrefix is prepended to auto-injected content (session memory,
	// file recovery, skill recovery) so ApplyPromptCaching can skip these messages
	// when placing cache breakpoints.
	SystemInjectedPrefix = "<!-- system-injected -->"
)

// buildCompressionPrompt generates the compression instruction text for inline
// cache-reusing compaction. The instruction is injected as a user message at
// the end of the conversation. Because the system prompt + tools + prior
// messages are already cached, only the instruction itself is new tokens.
// At higher compression levels, the instruction asks for shorter output.
func buildCompressionPrompt(level int) string {
	if level == 0 {
		return `═══════════════════════════════════════════════════════════════
CRITICAL: TASK CHANGE - MEMORY COMPRESSION MODE
═══════════════════════════════════════════════════════════════
The conversation above has ENDED. You are now in MEMORY COMPRESSION MODE.

CRITICAL INSTRUCTIONS - READ CAREFULLY:
1. This is NOT a continuation of the conversation
2. DO NOT respond to any requests in the conversation above
3. DO NOT call ANY tools or functions
4. DO NOT use tool_calls in your response
5. Your response MUST be PURE TEXT ONLY

YOUR ONLY TASK: Create a comprehensive summary of the conversation above.

REQUIRED RESPONSE FORMAT:
First output a <topics> line listing 3-6 key topic phrases (comma-separated, concise).
Then output the full summary wrapped in <summary> tags.

Example format:
<topics>Rails setup, database config, deploy pipeline, Tailwind CSS</topics>
<summary>
...full summary text...
</summary>`
	}
	if level == 1 {
		return `═══════════════════════════════════════════════════════════════
CRITICAL: TASK CHANGE - MEMORY COMPRESSION MODE [LEVEL 2]
═══════════════════════════════════════════════════════════════
The conversation above has ENDED. You are now in MEMORY COMPRESSION MODE.

DO NOT respond to requests. DO NOT call tools. PURE TEXT ONLY.

Create a CONCISE summary: key files, decisions, accomplishments only.

Format:
<topics>topic1, topic2, topic3</topics>
<summary>
...concise summary...
</summary>`
	}
	if level == 2 {
		return `═══════════════════════════════════════════════════════════════
CRITICAL: MEMORY COMPRESSION MODE [LEVEL 3]
═══════════════════════════════════════════════════════════════
DO NOT respond to requests. DO NOT call tools. PURE TEXT ONLY.

Create a MINIMAL summary: just project type, file counts, current status.

Format:
<topics>topic1, topic2</topics>
<summary>
...minimal summary...
</summary>`
	}
	return `═══════════════════════════════════════════════════════════════
MEMORY COMPRESSION MODE [LEVEL 4+]
═══════════════════════════════════════════════════════════════
DO NOT respond. DO NOT call tools. PURE TEXT ONLY.

One-line summary of current state and progress.

Format:
<topics>topic1</topics>
<summary>
...one line...
</summary>`
}

// PersistedToolResult holds information about a persisted tool result.
type PersistedToolResult struct {
	Filepath     string
	OriginalSize int
	IsJSON       bool
	Preview      string
	HasMore      bool
}

// ToolResultStore persists oversized tool results to disk so they can be
// re-read on demand after micro-compact clears them from context.
// Matching upstream's toolResultStorage.ts:
//   - Storage path: {projectDir}/{sessionId}/tool-results/{toolUseId}.{txt|json}
//   - XML tag: <persisted-output>...preview...</persisted-output>
//   - 'wx' flag for atomic writes (skip if exists)
//   - Preview truncated at newline boundary (2000 bytes)
type ToolResultStore struct {
	dir        string // session-specific tool results directory
	projectDir string
	sessionID  string
}

// NewToolResultStore creates a store rooted at {projectDir}/{sessionId}/tool-results/.
// If sessionID is empty, uses {projectDir}/tool-results/ as a fallback.
func NewToolResultStore(projectDir string, sessionID string) *ToolResultStore {
	var dir string
	if sessionID != "" {
		dir = filepath.Join(projectDir, sessionID, "tool-results")
	} else {
		dir = filepath.Join(projectDir, "tool-results")
	}
	_ = os.MkdirAll(dir, 0755) // create if not exists
	return &ToolResultStore{dir: dir, projectDir: projectDir, sessionID: sessionID}
}

// sanitizeToolID makes a toolUseID safe for use as a filename.
func sanitizeToolID(toolUseID string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, toolUseID)
}

// Persist saves a tool result to disk and returns metadata about the persisted file.
// Uses 'wx' flag for atomic writes — if the file already exists (from a prior turn),
// it is skipped (EEXIST is not an error). This matches upstream's idempotency guard.
func (s *ToolResultStore) Persist(toolUseID string, content string) *PersistedToolResult {
	if s.dir == "" || content == "" {
		return nil
	}

	safeID := sanitizeToolID(toolUseID)
	// Detect if content looks like JSON (starts with { or [)
	isJSON := len(content) > 0 && (content[0] == '{' || content[0] == '[')
	ext := "txt"
	if isJSON {
		ext = "json"
	}
	filename := safeID + "." + ext
	path := filepath.Join(s.dir, filename)

	// Use 'wx' flag — skip if file already exists (idempotent across turns)
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if !os.IsExist(err) {
			// Real error (not EEXIST) — return nil to signal failure
			return nil
		}
		// EEXIST: already persisted on a prior turn, fall through to generate preview
	} else {
		if _, writeErr := fd.Write([]byte(content)); writeErr != nil {
			fd.Close()
			return nil
		}
		if closeErr := fd.Close(); closeErr != nil {
			return nil
		}
	}

	preview, hasMore := generatePreview(content, PREVIEW_SIZE_BYTES)
	return &PersistedToolResult{
		Filepath:     path,
		OriginalSize: len(content),
		IsJSON:       isJSON,
		Preview:      preview,
		HasMore:      hasMore,
	}
}

// generatePreview truncates content at a newline boundary when possible,
// matching upstream's generatePreview().
func generatePreview(content string, maxBytes int) (preview string, hasMore bool) {
	if len(content) <= maxBytes {
		return content, false
	}
	truncated := content[:maxBytes]
	lastNewline := strings.LastIndex(truncated, "\n")
	// If we found a newline reasonably close to the limit, use it
	cutPoint := maxBytes
	if lastNewline > maxBytes/2 {
		cutPoint = lastNewline
	}
	return content[:cutPoint], true
}

// buildLargeToolResultMessage formats a persisted tool result into the
// <persisted-output> XML message that the model sees, matching upstream exactly.
func buildLargeToolResultMessage(result *PersistedToolResult) string {
	var sb strings.Builder
	sb.WriteString(PERSISTED_OUTPUT_TAG + "\n")
	sb.WriteString(fmt.Sprintf("Output too large (%s). Full output saved to: %s\n\n",
		formatFileSize(result.OriginalSize), result.Filepath))
	sb.WriteString(fmt.Sprintf("Preview (first %s):\n", formatFileSize(PREVIEW_SIZE_BYTES)))
	sb.WriteString(result.Preview)
	if result.HasMore {
		sb.WriteString("\n...\n")
	} else {
		sb.WriteString("\n")
	}
	sb.WriteString(PERSISTED_OUTPUT_CLOSING_TAG)
	return sb.String()
}

// formatFileSize returns a human-readable file size string.
func formatFileSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d bytes", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024.0)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024.0*1024.0))
}

// isToolResultContentEmpty returns true if content is empty or whitespace-only.
// Matching upstream's isToolResultContentEmpty().
func isToolResultContentEmpty(content string) bool {
	return strings.TrimSpace(content) == ""
}

// maybePersistToolResult checks if a tool result should be persisted based on
// size threshold. Returns the modified content string if persisted, or the
// original if not. Matching upstream's maybePersistLargeToolResult().
func (s *ToolResultStore) maybePersistToolResult(toolUseID string, toolName string, content string, threshold int) string {
	if isToolResultContentEmpty(content) {
		return fmt.Sprintf("(%s completed with no output)", toolName)
	}
	if len(content) <= threshold {
		return content
	}
	result := s.Persist(toolUseID, content)
	if result == nil {
		return content // persistence failed, return original
	}
	return buildLargeToolResultMessage(result)
}

// Read loads a persisted tool result from disk by its toolUseID.
func (s *ToolResultStore) Read(toolUseID string) (string, error) {
	if s.dir == "" {
		return "", fmt.Errorf("tool result store not configured")
	}
	// Try both .txt and .json extensions
	safeID := sanitizeToolID(toolUseID)
	for _, ext := range []string{".txt", ".json"} {
		path := filepath.Join(s.dir, safeID+ext)
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("tool result read error: %w", err)
		}
	}
	return "", fmt.Errorf("tool result not found on disk: %s", toolUseID)
}

// --- Content Replacement State (matching upstream toolResultStorage.ts) ---
//
// Tracks replacement state across turns so enforceToolResultBudget makes the
// same choices every time (preserves prompt cache prefix).
// Once seen, a result's fate is frozen for the conversation.

// ContentReplacementState tracks per-conversation-thread state for the aggregate
// tool result budget. Matching upstream's ContentReplacementState type.
//   - seenIds: results that have passed through the budget check (replaced or not).
//     Once seen, a result's fate is frozen.
//   - replacements: subset of seenIds that were persisted to disk and replaced
//     with previews, mapped to the exact preview string shown to the model.
type ContentReplacementState struct {
	mu           sync.RWMutex
	seenIds      map[string]bool
	replacements map[string]string // toolUseId -> exact replacement string
}

// NewContentReplacementState creates a fresh ContentReplacementState.
func NewContentReplacementState() *ContentReplacementState {
	return &ContentReplacementState{
		seenIds:      make(map[string]bool),
		replacements: make(map[string]string),
	}
}

// ContentReplacementRecord is a serializable record of one content-replacement
// decision, matching upstream's ContentReplacementRecord type.
type ContentReplacementRecord struct {
	Kind        string `json:"kind"` // always "tool-result"
	ToolUseID   string `json:"toolUseId"`
	Replacement string `json:"replacement"` // exact string the model saw
}

// toolResultCandidate represents a tool result block eligible for budget enforcement.
type toolResultCandidate struct {
	toolUseID   string
	content     string
	size        int
	replacement string // cached replacement string for mustReapply
}

// enforceToolResultBudget enforces the per-message budget on aggregate tool result
// size. Matching upstream's enforceToolResultBudget().
//
// For each user message whose tool_result blocks together exceed the per-message
// limit, the largest FRESH (never-before-seen) results are persisted to disk and
// replaced with <persisted-output> previews.
//
// State is tracked by tool_use_id in state. Once a result is seen its fate is
// frozen: previously-replaced results get the same replacement re-applied every
// turn (zero I/O, byte-identical), and previously-unreplaced results are never
// replaced later (would break prompt cache).
//
// Instead of mutating c.entries in-place (which breaks KV cache prefix),
// replacements are recorded in c.toolResultReplacements and applied during
// BuildMessages() serialization. This keeps original entries stable for cache.
//
// Returns the number of newly replaced results.
func (c *ConversationContext) enforceToolResultBudget(
	state *ContentReplacementState,
	store *ToolResultStore,
	limit int,
	skipToolNames map[string]bool,
) int {
	if state == nil || store == nil || limit <= 0 {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	// Ensure replacement map is initialized
	if c.toolResultReplacements == nil {
		c.toolResultReplacements = make(map[string]string)
	}

	// Build tool_use_id -> tool_name mapping from ToolUseContent entries
	toolNameMap := make(map[string]string)
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

	newlyReplaced := 0

	// Process each ToolResultContent entry (each represents one user message)
	for _, entry := range c.entries {
		results, ok := entry.content.(ToolResultContent)
		if !ok {
			continue
		}

		// Collect candidates from this message
		var candidates []toolResultCandidate
		for _, r := range results {
			// Extract text content
			contentText := ""
			for _, cb := range r.Content {
				if cb.OfText != nil {
					contentText += cb.OfText.Text
				}
			}
			if contentText == "" || strings.HasPrefix(contentText, PERSISTED_OUTPUT_TAG) {
				continue // skip empty or already-compacted
			}
			// Skip if already recorded as a replacement (from micro-compact or prior budget pass)
			if _, ok := c.toolResultReplacements[r.ToolUseID]; ok {
				continue
			}
			candidates = append(candidates, toolResultCandidate{
				toolUseID: r.ToolUseID,
				content:   contentText,
				size:      len(contentText),
			})
		}

		if len(candidates) == 0 {
			continue
		}

		// Partition by prior decision state
		var mustReapply []toolResultCandidate // previously replaced -> re-apply
		var frozen []toolResultCandidate      // previously seen and left unreplaced
		var fresh []toolResultCandidate       // never seen -> eligible

		for _, cand := range candidates {
			if repl, ok := state.replacements[cand.toolUseID]; ok {
				cand.replacement = repl // store for re-apply
				mustReapply = append(mustReapply, cand)
			} else if state.seenIds[cand.toolUseID] {
				frozen = append(frozen, cand)
			} else {
				fresh = append(fresh, cand)
			}
		}

		// Re-apply cached replacements via replacement map (zero I/O, byte-identical)
		// No entry mutation — BuildMessages() applies these during serialization
		for _, cand := range mustReapply {
			c.toolResultReplacements[cand.toolUseID] = cand.replacement
		}

		// Mark all non-fresh IDs as seen
		for _, cand := range mustReapply {
			state.seenIds[cand.toolUseID] = true
		}
		for _, cand := range frozen {
			state.seenIds[cand.toolUseID] = true
		}

		if len(fresh) == 0 {
			continue
		}

		// Skip tools in skipToolNames (e.g., Read with Infinity threshold)
		var eligible []toolResultCandidate
		for _, cand := range fresh {
			toolName := toolNameMap[cand.toolUseID]
			if skipToolNames[toolName] {
				state.seenIds[cand.toolUseID] = true // freeze without replacement
			} else {
				eligible = append(eligible, cand)
			}
		}

		if len(eligible) == 0 {
			continue
		}

		// Calculate total size: frozen + eligible
		frozenSize := 0
		for _, cand := range frozen {
			frozenSize += cand.size
		}
		freshSize := 0
		for _, cand := range eligible {
			freshSize += cand.size
		}

		// If total exceeds limit, select largest fresh results to replace
		if frozenSize+freshSize <= limit {
			// Under budget — mark all as seen (frozen) without replacement
			for _, cand := range eligible {
				state.seenIds[cand.toolUseID] = true
			}
			continue
		}

		// Sort eligible by size descending (replace largest first)
		sort.Slice(eligible, func(i, j int) bool {
			return eligible[i].size > eligible[j].size
		})

		// Select candidates to replace until under budget
		remaining := frozenSize + freshSize
		var selected []toolResultCandidate
		for _, cand := range eligible {
			if remaining <= limit {
				break
			}
			selected = append(selected, cand)
			remaining -= cand.size
		}

		// Mark non-selected as seen (frozen)
		selectedSet := make(map[string]bool, len(selected))
		for _, cand := range selected {
			selectedSet[cand.toolUseID] = true
		}
		for _, cand := range eligible {
			if !selectedSet[cand.toolUseID] {
				state.seenIds[cand.toolUseID] = true
			}
		}

		if len(selected) == 0 {
			continue
		}

		// Persist selected results and record replacements in map
		for _, cand := range selected {
			persisted := store.Persist(cand.toolUseID, cand.content)
			if persisted == nil {
				// Persistence failed — mark as seen but unreplaced (frozen)
				state.seenIds[cand.toolUseID] = true
				continue
			}
			replacement := buildLargeToolResultMessage(persisted)
			state.seenIds[cand.toolUseID] = true
			state.replacements[cand.toolUseID] = replacement
			newlyReplaced++

			// Record replacement in map instead of mutating entry
			c.toolResultReplacements[cand.toolUseID] = replacement
		}
	}

	return newlyReplaced
}

// applyToolResultBudget is the query-loop integration point for the aggregate budget.
// Gates on state (nil means feature disabled), applies enforcement.
// Returns true if any replacements were made.
func (c *ConversationContext) applyToolResultBudget(
	state *ContentReplacementState,
	store *ToolResultStore,
) bool {
	if state == nil || store == nil {
		return false
	}
	return c.enforceToolResultBudget(state, store, MAX_TOOL_RESULTS_PER_MESSAGE_CHARS, nil) > 0
}

// reconstructContentReplacementState rebuilds state from records loaded from
// the transcript, matching upstream's reconstructContentReplacementState().
func reconstructContentReplacementState(
	entries []conversationEntry,
	records []ContentReplacementRecord,
) *ContentReplacementState {
	state := NewContentReplacementState()

	// Collect all candidate tool_use_ids from entries
	candidateIDs := make(map[string]bool)
	for _, entry := range entries {
		results, ok := entry.content.(ToolResultContent)
		if !ok {
			continue
		}
		for _, r := range results {
			if r.ToolUseID != "" {
				candidateIDs[r.ToolUseID] = true
			}
		}
	}

	// Mark all candidates as seen
	for id := range candidateIDs {
		state.seenIds[id] = true
	}

	// Apply records for replacements
	for _, r := range records {
		if r.Kind == "tool-result" && candidateIDs[r.ToolUseID] {
			state.replacements[r.ToolUseID] = r.Replacement
		}
	}

	return state
}

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
	Trigger          CompactTrigger
	PreCompactTokens int
	// UUID uniquely identifies this compact boundary. Used by the transcript,
	// session storage, and QueryEngine to reference specific compaction events.
	UUID string
	// PreCompactDiscoveredTools carries loaded deferred tool schema names at
	// compact time. The summary doesn't preserve tool_reference blocks, so the
	// post-compact schema filter needs this to keep sending already-loaded
	// deferred tool schemas to the API. Matches upstream's compactMetadata.preCompactDiscoveredTools.
	PreCompactDiscoveredTools []string
	// PreservedSegment tracks the UUIDs of the first and last kept messages
	// for chain relinking after partial compaction. Matches upstream's
	// compactMetadata.preservedSegment (headUuid, anchorUuid, tailUuid).
	PreservedSegment *PreservedSegment
}

// PreservedSegment tracks UUIDs for message chain relinking after partial compaction.
// The loader uses this to patch head→anchor and anchor's-other-children→tail.
type PreservedSegment struct {
	HeadUUID   string // UUID of the first kept message
	AnchorUUID string // UUID of the message immediately before kept[0] in the desired chain
	TailUUID   string // UUID of the last kept message
}

func (CompactBoundaryContent) entryContent() {}

// SummaryContent represents a conversation summary inserted after compaction.
type SummaryContent string

func (SummaryContent) entryContent() {}

// CompressionInstructionContent represents an inline compression instruction
// injected into the conversation to trigger cache-reusing compaction.
// Inspired by openclacky's insert-then-compress pattern: instead of making
// a separate API call for compression, the instruction is appended as a
// user message, and the next API call reuses the prompt cache prefix.
type CompressionInstructionContent struct {
	Level int // compression level for progressive summarization
}

func (CompressionInstructionContent) entryContent() {}

// CompressedSummaryContent holds the parsed LLM response from an inline
// compression call. Inspired by openclacky's parse_compressed_result:
// wraps summary with chunk anchors, topics metadata, and previous-chunks index
// so the AI knows where to find archived conversation details.
type CompressedSummaryContent struct {
	Summary    string // the actual summary text
	Topics     string // comma-separated topic phrases extracted from <topics>
	ChunkPath  string // path to archived chunk file for this compaction
	TopicsOnly bool   // true if only topics were extracted (parsing fallback)
}

func (CompressedSummaryContent) entryContent() {}

// AttachmentContent represents post-compact recovery content (file/skill re-injection).
type AttachmentContent string

func (AttachmentContent) entryContent() {}

// AntiReplayContent contains post-compaction rules to prevent re-execution of completed tasks.
// Separated from the summary so it survives further compaction.
type AntiReplayContent string

func (AntiReplayContent) entryContent() {}

// GoalContent contains the structured goal block (pending/completed tasks, current work).
// Separated from the summary so it survives further compaction.
type GoalContent string

func (GoalContent) entryContent() {}

type fileState struct {
	epoch int
	mtime int64 // mtimeMs when read
}

// ToolStateTracker records what the agent has done across turns.
// It is injected into the system prompt before each API call so the agent
// knows what it has already read/searched without re-reading/re-searching.
//
// Compaction is the primary source of "short-term memory loss":
// when context is compacted, tool results (file content, grep output) are removed
// from the conversation history. The tracker must distinguish between items whose
// content is still in context (fresh) vs. items that were cleared (stale).
//
// We solve this with an epoch counter: every compaction increments the epoch.
// Items recorded with epoch == currentEpoch are fresh; items with lower epoch
// are stale (compaction cleared them). Post-compact recovery marks re-injected
// files as fresh by updating their epoch.
type ToolStateTracker struct {
	mu              sync.RWMutex
	compactionEpoch int                  // increments each time compaction runs; items with lower epoch are stale
	readFiles       map[string]fileState // absolute path -> (epoch, mtimeMs) when read
	searchQueries   map[string]int       // pattern -> epoch when search was run
	conclusions     []string             // key findings claimed by the agent
	// activeTask tracks what the agent is currently working on, derived from
	// the most recent user message and recent tool calls. This is injected
	// into the system prompt each turn to prevent "task drift" — the LLM
	// losing track of what it was doing and jumping back to old topics.
	activeTask string // brief description of current work
}

// NewToolStateTracker creates a tracker.
func NewToolStateTracker() *ToolStateTracker {
	return &ToolStateTracker{
		compactionEpoch: 0,
		readFiles:       make(map[string]fileState),
		searchQueries:   make(map[string]int),
		conclusions:     make([]string, 0),
	}
}

// RecordFileRead marks a file as read at the current epoch, recording the mtime.
func (t *ToolStateTracker) RecordFileRead(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	abs, _ := filepath.Abs(path)
	var mtimeMs int64
	if info, err := os.Stat(abs); err == nil {
		mtimeMs = info.ModTime().UnixMilli()
	}
	t.readFiles[abs] = fileState{epoch: t.compactionEpoch, mtime: mtimeMs}
}

// SetActiveTask records what the agent is currently working on, derived from
// the most recent user message. This is injected into the system prompt each
// turn to prevent "task drift" — the LLM losing track of its current task
// and jumping back to old topics.
func (t *ToolStateTracker) SetActiveTask(task string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.activeTask = task
}

// GetActiveTask returns the current active task description.
func (t *ToolStateTracker) GetActiveTask() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.activeTask
}

// RecordSearch records a successful grep/glob search pattern at the current epoch.
func (t *ToolStateTracker) RecordSearch(pattern string, hadResults bool) {
	if !hadResults {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.searchQueries[pattern] = t.compactionEpoch
}

// RecordConclusion appends a key finding.
func (t *ToolStateTracker) RecordConclusion(conclusion string) {
	if conclusion == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conclusions = append(t.conclusions, conclusion)
}

// OnCompaction is called after context compaction runs. It advances the epoch,
// marking all previously tracked items as stale (their tool results are gone from context).
func (t *ToolStateTracker) OnCompaction() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.compactionEpoch++
}

// MarkFileFresh updates a file's epoch to current, marking its content as fresh
// (used after PostCompactRecovery re-injects file content into context).
func (t *ToolStateTracker) MarkFileFresh(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	abs, _ := filepath.Abs(path)
	var mtimeMs int64
	if info, err := os.Stat(abs); err == nil {
		mtimeMs = info.ModTime().UnixMilli()
	}
	t.readFiles[abs] = fileState{epoch: t.compactionEpoch, mtime: mtimeMs}
}

// ClearConclusions removes all recorded conclusions.
// Called after compaction when no files were recovered — the summary now captures
// all pre-compact knowledge, so stale conclusions should not be re-stated.
func (t *ToolStateTracker) ClearConclusions() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conclusions = t.conclusions[:0]
}

// GetConclusions returns a copy of the recorded conclusions.
func (t *ToolStateTracker) GetConclusions() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]string, len(t.conclusions))
	copy(result, t.conclusions)
	return result
}

// BuildSessionStateNote returns the text to inject into the system prompt.
// Items are split into "fresh" (content still in context) and "stale" (cleared by compaction).
func (t *ToolStateTracker) BuildSessionStateNote() string {
	t.mu.RLock()
	epoch := t.compactionEpoch
	var freshFiles, staleFiles []string
	for f, state := range t.readFiles {
		if state.epoch == epoch {
			freshFiles = append(freshFiles, f)
		} else {
			staleFiles = append(staleFiles, f)
		}
	}
	var freshSearches, staleSearches []string
	for q, e := range t.searchQueries {
		if e == epoch {
			freshSearches = append(freshSearches, q)
		} else {
			staleSearches = append(staleSearches, q)
		}
	}
	conclusions := make([]string, len(t.conclusions))
	copy(conclusions, t.conclusions)
	activeTask := t.activeTask
	t.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("## Session State\n")
	if activeTask != "" {
		sb.WriteString("Current task (STAY FOCUSED on this — do NOT jump to other topics):\n")
		sb.WriteString("  " + activeTask + "\n")
	}
	if len(freshFiles) > 0 {
		sb.WriteString("Files already read — content is in context (do NOT re-read):\n")
		for _, f := range freshFiles {
			sb.WriteString("  - " + f + "\n")
		}
	}
	if len(staleFiles) > 0 {
		sb.WriteString("Files read before compaction — content was cleared from context:\n")
		for _, f := range staleFiles {
			sb.WriteString("  - " + f + " (RE-READ if needed)\n")
		}
	}
	if len(freshSearches) > 0 {
		sb.WriteString("Search patterns already run — results in context (do NOT repeat):\n")
		for _, q := range freshSearches {
			sb.WriteString("  - " + q + "\n")
		}
	}
	if len(staleSearches) > 0 {
		sb.WriteString("Search patterns from before compaction — results were cleared:\n")
		for _, q := range staleSearches {
			sb.WriteString("  - " + q + " (RE-RUN if needed)\n")
		}
	}
	if len(conclusions) > 0 {
		sb.WriteString("Key findings from this session:\n")
		for _, c := range conclusions {
			sb.WriteString("  - " + c + "\n")
		}
	}
	if len(freshFiles) == 0 && len(staleFiles) == 0 && len(freshSearches) == 0 && len(staleSearches) == 0 && len(conclusions) == 0 {
		sb.WriteString("(no prior state)\n")
	}
	return sb.String()
}

// conversationEntry represents a single entry in the conversation history.
type conversationEntry struct {
	role       string // "user" or "assistant" (or "system" for boundary markers)
	content    EntryContent
	summarized bool // true if this entry was already included in a previous compaction summary
}

// ConversationContext manages the conversation message history and system prompt.
type ConversationContext struct {
	mu                  sync.RWMutex
	config              Config
	entries             []conversationEntry
	systemPrompt        string
	lastSummarizedIndex int       // index of last entry included in summary/compact (-1 = none)
	compactedEntryCount int       // entries already summarized by previous compaction (skip on next compact)
	lastAssistantTime   time.Time // timestamp of last assistant message added; used for time-based microcompact
	// pendingRedactedThinking holds opaque data blobs from redacted_thinking blocks
	// received in the most recent API response. These must be preserved and
	// re-submitted in subsequent assistant messages for context continuity.
	// Matching upstream's handling of redacted_thinking in normalizeMessagesForAPI().
	pendingRedactedThinking []string
	// toolResultStore persists oversized tool results to disk during micro-compact,
	// so they can be re-read on demand without re-executing tools.
	toolResultStore *ToolResultStore
	// contentReplacementState tracks replacement decisions across turns for
	// prompt cache stability. Matching upstream's ContentReplacementState.
	contentReplacementState *ContentReplacementState
	// apiTokenAnchor stores the exact input_tokens from the most recent API
	// response, along with the entry count at that point. This enables hybrid
	// token estimation: use the exact API count as anchor, then only estimate
	// the delta for entries added since. Matching upstream's tokenCountWithEstimation().
	apiTokenAnchor   int64 // exact input_tokens from last API response
	apiAnchorEntries int   // number of entries in context when anchor was recorded
	anchorEpoch      int   // epoch when anchor was recorded; stale if != compactionEpoch
	compactionEpoch  int   // increments each time compaction rewrites the message list
	// toolResultReplacements maps tool_use_id -> replacement text to apply during
	// BuildMessages() serialization. Populated by MicroCompactEntries() and
	// enforceToolResultBudget(). This keeps original entries stable for KV cache
	// prefix matching while the API still sees trimmed/placeholder content.
	toolResultReplacements map[string]string
	// clearedToolResults tracks tool_use_ids whose content was cleared by
	// MicroCompactEntries, so enforceToolResultBudget() can skip them.
	clearedToolResults map[string]bool
	// compressionLevel tracks how many times the conversation has been compressed.
	// Increments after each compaction, used for progressive summarization:
	// Level 1 = full detail, Level 2 = concise, Level 3 = minimal, Level 4+ = ultra-minimal.
	compressionLevel int
}

// NewConversationContext creates a new context.
func NewConversationContext(cfg Config) *ConversationContext {
	return &ConversationContext{
		config:                  cfg,
		toolResultReplacements:  make(map[string]string),
		clearedToolResults:      make(map[string]bool),
	}
}

// SetAPITokenAnchor records the exact input_tokens from an API response along
// with the current entry count. This enables hybrid token estimation in
// EstimatedTokens(): use the API anchor as a precise baseline, then only
// estimate the delta for entries added since. Matching upstream's
// tokenCountWithEstimation() which finds the last assistant message with
// usage data and uses that as the anchor point.
func (c *ConversationContext) SetAPITokenAnchor(inputTokens int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiTokenAnchor = inputTokens
	c.apiAnchorEntries = len(c.entries)
	c.anchorEpoch = c.compactionEpoch
}

// InvalidateAnchors clears the API token anchor after compaction rewrites the
// message list. Without this, the anchor's apiAnchorEntries index becomes
// numerically coincident but semantically invalid — it points to a different
// message than when it was recorded, causing EstimatedTokens() to compute a
// wrong delta and the cache optimizer to place cache_control markers at the
// wrong positions. The anchor will be recalculated from scratch on the next
// API call via SetAPITokenAnchor.
func (c *ConversationContext) InvalidateAnchors() {
	c.apiTokenAnchor = 0
	c.apiAnchorEntries = 0
	c.anchorEpoch = 0
}

// EstimatedTokens returns a hybrid token estimate using API anchor + incremental
// heuristic estimation. This matches upstream's tokenCountWithEstimation() approach:
//
//  1. If we have an API anchor (exact input_tokens from a prior API response),
//     use that as the baseline and only estimate the delta for entries added since.
//     This prevents cumulative drift that occurs when estimating ALL messages from
//     scratch using heuristics.
//
//  2. If no API anchor exists (early conversation), fall back to full heuristic
//     estimation with content-type-aware token counting and 4/3 safety margin.
//
// Only counts entries after the most recent compact boundary — entries before the
// boundary are not sent to the API (BuildMessages skips them), so counting them
// would inflate the estimate and cause false compaction triggers on resume.
// EntryCount returns the number of conversation entries in history.
// Used by the two-layer overflow recovery to calculate pull-back amounts.
func (c *ConversationContext) EntryCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *ConversationContext) EstimatedTokens() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Find the most recent compact boundary — same logic as BuildMessages()
	boundaryIdx := -1
	for i := len(c.entries) - 1; i >= 0; i-- {
		if _, ok := c.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}

	startIdx := 0
	if boundaryIdx >= 0 {
		startIdx = boundaryIdx
	}

	// Hybrid estimation: use API anchor if available and valid
	if c.apiTokenAnchor > 0 && c.apiAnchorEntries > 0 && c.anchorEpoch == c.compactionEpoch {
		// The anchor was recorded when there were apiAnchorEntries entries.
		// The epoch check ensures the anchor predates any compaction that
		// rewrote the message list — if compaction occurred after the anchor
		// was set, the index is numerically coincident but semantically wrong
		// (points to a different message), so we must fall through to full
		// estimation. The old range check is kept as a secondary guard.
		if c.apiAnchorEntries >= startIdx && c.apiAnchorEntries <= len(c.entries) {
			// Count how many entries exist after the anchor point
			deltaStart := c.apiAnchorEntries
			if deltaStart < startIdx {
				deltaStart = startIdx
			}
			deltaEstimate := estimateEntriesTokens(c.entries[deltaStart:])
			if deltaEstimate == 0 {
				return int(c.apiTokenAnchor)
			}
			// Apply 4/3 safety margin to the delta only
			deltaWithMargin := int(math.Ceil(float64(deltaEstimate) * 4.0 / 3.0))
			return int(c.apiTokenAnchor) + deltaWithMargin
		}
	}

	// Full heuristic estimation (no anchor or stale anchor)
	rawTotal := estimateEntriesTokens(c.entries[startIdx:])
	if rawTotal == 0 {
		return 0
	}
	// Apply 4/3 safety margin
	return int(math.Ceil(float64(rawTotal) * 4.0 / 3.0))
}

// EstimatedTokenRatio returns the estimated token count as a fraction of ctxMax.
// Returns 0 if ctxMax <= 0. Used for turn-start fold threshold checks.
func (c *ConversationContext) EstimatedTokenRatio(ctxMax int) float64 {
	if ctxMax <= 0 {
		return 0
	}
	return float64(c.EstimatedTokens()) / float64(ctxMax)
}

// EstimateRequestTokens estimates total request tokens including tools and system prompt.
// DeepSeek-Reasonix pattern: estimateTurnStart includes tools + fewshots overhead
// to make better pre-fold decisions.
func (c *ConversationContext) EstimateRequestTokens(toolTokenOverhead int, systemPromptTokens int) int {
	base := c.EstimatedTokens()
	return base + toolTokenOverhead + systemPromptTokens
}

// estimateEntriesTokens estimates token count for a slice of conversation entries
// using content-type-aware heuristic estimation. No safety margin is applied —
// the caller applies it if needed.
func estimateEntriesTokens(entries []conversationEntry) int {
	rawTotal := 0
	for _, entry := range entries {
		switch v := entry.content.(type) {
		case TextContent:
			ct := DetectContentType(string(v))
			rawTotal += EstimateContentTokens(string(v), ct)
		case ToolUseContent:
			for _, b := range v {
				if b.OfText != nil {
					rawTotal += EstimateContentTokens(b.OfText.Text, "code")
				}
				if b.OfToolUse != nil {
					rawTotal += 10 // tool_use overhead
					rawTotal += EstimateContentTokens(b.OfToolUse.Name, "code")
					if m, ok := b.OfToolUse.Input.(map[string]any); ok {
						if data, err := json.Marshal(m); err == nil {
							rawTotal += EstimateContentTokens(string(data), "json")
						}
					}
				}
			}
		case ToolResultContent:
			for _, r := range v {
				rawTotal += 8 // tool_result overhead
				for _, cb := range r.Content {
					if cb.OfText != nil {
						ct := DetectContentType(cb.OfText.Text)
						rawTotal += EstimateContentTokens(cb.OfText.Text, ct)
					}
				}
			}
		case CompactBoundaryContent:
			// Boundary markers are small, ignore for estimation
		case SummaryContent:
			rawTotal += EstimateContentTokens(string(v), "natural")
		case AntiReplayContent:
			rawTotal += EstimateContentTokens(string(v), "natural")
		case GoalContent:
			rawTotal += EstimateContentTokens(string(v), "natural")
		case CompressionInstructionContent:
			rawTotal += EstimateContentTokens(buildCompressionPrompt(v.Level), "natural")
		case CompressedSummaryContent:
			rawTotal += EstimateContentTokens(v.Summary, "natural")
		}
	}
	return rawTotal
}

// SetSystemPrompt sets the system prompt.
func (c *ConversationContext) SetSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.systemPrompt = prompt
}

// LatestUserMessage returns the text of the most recent user message
// (excluding tool_result messages). Used to derive the "active task"
// for task drift prevention.
func (c *ConversationContext) LatestUserMessage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := len(c.entries) - 1; i >= 0; i-- {
		entry := c.entries[i]
		if entry.role != "user" {
			continue
		}
		// Skip tool_result user messages
		if _, ok := entry.content.(ToolResultContent); ok {
			continue
		}
		// Extract text from TextContent or ToolUseContent
		switch v := entry.content.(type) {
		case TextContent:
			return string(v)
		case ToolUseContent:
			// Mixed content — extract text parts
			var textParts []string
			for _, block := range v {
				if block.OfText != nil {
					textParts = append(textParts, block.OfText.Text)
				}
			}
			if len(textParts) > 0 {
				return strings.Join(textParts, "\n")
			}
		}
	}
	return ""
}

// ShouldTimeBasedMicroCompact returns true when the time gap since the last
// assistant message exceeds gapMinutes. A gapMinutes of 0 means always fire
// (legacy count-based behavior for backward compatibility).
func (c *ConversationContext) ShouldTimeBasedMicroCompact(gapMinutes int) bool {
	if gapMinutes <= 0 {
		return true // disabled or legacy — fire every turn
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	gap := time.Since(c.lastAssistantTime)
	return gap >= time.Duration(gapMinutes)*time.Minute
}

// SystemPrompt returns the system prompt.
func (c *ConversationContext) SystemPrompt() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.systemPrompt
}

// SetRedactedThinkingData stores opaque data blobs from redacted_thinking blocks
// received in the most recent API response. These must be preserved and re-submitted
// in subsequent API requests for context continuity.
func (c *ConversationContext) SetRedactedThinkingData(data []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingRedactedThinking = data
}

// SetToolResultStore configures the disk persistence store for tool results.
// When set, MicroCompactEntries will persist cleared results to disk and
// replace them with <persisted-output> XML tags.
func (c *ConversationContext) SetToolResultStore(store *ToolResultStore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.toolResultStore = store
}

// SetContentReplacementState configures the state tracker for prompt cache
// stability. When set, enforceToolResultBudget will make consistent replacement
// decisions across turns, preserving the prompt cache prefix.
func (c *ConversationContext) SetContentReplacementState(state *ContentReplacementState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contentReplacementState = state
}

// SetToolResultStore configures the disk persistence store for tool results.

// AddUserMessage appends a user text message.
// Drops any trailing assistant tool_calls that have no matching tool_result,
// matching openclacky's drop_dangling_tool_calls! pattern. This prevents
// 400 errors when a previous turn was interrupted before tool results arrived.
func (c *ConversationContext) AddUserMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropDanglingToolCalls()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(content),
	})
	c.truncateIfNeeded()
}

// dropDanglingToolCalls removes trailing assistant entries that contain
// ToolUseContent with no subsequent ToolResultContent. This happens when
// the agent is interrupted mid-turn (e.g., user sends new message before
// tool results arrive). Without cleanup, the API rejects the conversation
// with a 400 error for unanswered tool_calls.
func (c *ConversationContext) dropDanglingToolCalls() {
	// Walk backwards from the end, dropping assistant entries that have
	// tool_use blocks with no matching tool_result after them.
	for len(c.entries) > 0 {
		last := c.entries[len(c.entries)-1]
		if last.role != "assistant" {
			break
		}
		blocks, ok := last.content.(ToolUseContent)
		if !ok {
			break // not a tool_use entry, stop
		}
		// Since this is the last entry, there can't be tool results after it.
		// If it has any tool_use blocks, they are dangling — drop it.
		hasToolCalls := false
		for _, b := range blocks {
			if b.OfToolUse != nil && b.OfToolUse.ID != "" {
				hasToolCalls = true
				break
			}
		}
		if hasToolCalls {
			c.entries = c.entries[:len(c.entries)-1]
		} else {
			break // assistant message without tool_calls, stop
		}
	}
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
	c.lastAssistantTime = time.Now()
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
	c.lastAssistantTime = time.Now()
	c.truncateIfNeeded()
}

// RemoveLastAssistantEntry removes the last assistant entry if it has tool_use content.
// This is used to undo AddAssistantToolCalls when streaming results fail to arrive.
func (c *ConversationContext) RemoveLastAssistantEntry() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) > 0 {
		lastIdx := len(c.entries) - 1
		if c.entries[lastIdx].role == "assistant" {
			c.entries = c.entries[:lastIdx]
		}
	}
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
// Compact boundaries are handled by skipping entries before the last boundary.
// This avoids the shared-array-overwrite bug that messages=messages[:0] causes
// when there are entries after the boundary (the first append after [:0] overwrites
// array[0], the system prompt, which subsequent appends then continue to overwrite).
func (c *ConversationContext) BuildMessages() []anthropic.MessageParam {
	c.mu.Lock()
	redactedData := make([]string, len(c.pendingRedactedThinking))
	copy(redactedData, c.pendingRedactedThinking)
	c.pendingRedactedThinking = nil // consume; only used once

	// Copy entries while holding the lock to avoid race conditions.
	// BuildMessages can be called from background goroutines (e.g. trySMCompact)
	// while other goroutines modify c.entries via AddMessage, AddToolResult, etc.
	entriesCopy := make([]conversationEntry, len(c.entries))
	copy(entriesCopy, c.entries)

	// Copy replacement map for cache-stable serialization.
	replacements := make(map[string]string, len(c.toolResultReplacements))
	for k, v := range c.toolResultReplacements {
		replacements[k] = v
	}
	c.mu.Unlock()

	// Find the last compact boundary. Entries at or after this point are preserved;
	// everything before is dropped. This is the key mechanism that makes compaction
	// actually reduce token usage — without this reset, old messages would still be
	// included and compaction would be a no-op.
	boundaryIdx := -1
	for i := len(entriesCopy) - 1; i >= 0; i-- {
		if _, ok := entriesCopy[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}

	startIdx := 0
	if boundaryIdx >= 0 {
		startIdx = boundaryIdx
	}

	messages := make([]anthropic.MessageParam, 0, len(entriesCopy)-startIdx)
	for i, entry := range entriesCopy[startIdx:] {
		msg := anthropic.MessageParam{Role: anthropic.MessageParamRole(entry.role)}

		// DEBUG: Log entries with empty/invalid roles or unknown content types.
		// This helps diagnose 2013 errors where tool_use entries are mysteriously missing.
		if entry.role == "" {
			fmt.Fprintf(os.Stderr, "[bm-debug] entry[%d] has EMPTY role, content type=%T\n", startIdx+i, entry.content)
		}

		switch v := entry.content.(type) {
		case TextContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: string(v)}},
			}
		case ToolUseContent:
			// Prepend redacted_thinking blocks to the first assistant tool_use message.
			// The API requires these opaque data blobs to be re-submitted for context
			// continuity when interleaved thinking is enabled.
			if len(redactedData) > 0 {
				blocks := make([]anthropic.ContentBlockParamUnion, 0, len(redactedData)+len(v))
				for _, data := range redactedData {
					blocks = append(blocks, anthropic.NewRedactedThinkingBlock(data))
				}
				blocks = append(blocks, v...)
				redactedData = nil // consume once
				msg.Content = blocks
			} else {
				msg.Content = v
			}
		case ToolResultContent:
			blocks := make([]anthropic.ContentBlockParamUnion, len(v))
			for i, r := range v {
				// Apply replacement if one exists for this tool_use_id.
				// This keeps original entries stable for KV cache while sending
				// trimmed/placeholder content to the API.
				if repl, ok := replacements[r.ToolUseID]; ok {
					replaced := anthropic.ToolResultBlockParam{
						ToolUseID: r.ToolUseID,
						Content: []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: repl}},
						},
						IsError: r.IsError,
					}
					blocks[i] = anthropic.ContentBlockParamUnion{OfToolResult: &replaced}
				} else {
					blocks[i] = anthropic.ContentBlockParamUnion{OfToolResult: &r}
				}
			}
			msg.Content = blocks
		case CompactBoundaryContent:
			// The boundary itself is not sent to the API — it serves as the
			// cutoff marker. The API doesn't understand compact boundaries.
			continue
		case SummaryContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + string(v)}},
			}
		case AttachmentContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + string(v)}},
			}
		case AntiReplayContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + string(v)}},
			}
		case GoalContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + string(v)}},
			}
		case CompressionInstructionContent:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + buildCompressionPrompt(v.Level)}},
			}
		case CompressedSummaryContent:
			// Render as user message with chunk anchor.
			// This matches openclacky's parse_compressed_result output:
			// system_injected prefix + summary + optional chunk path reference.
			text := SystemInjectedPrefix + v.Summary
			if v.ChunkPath != "" {
				text += fmt.Sprintf("\n\n📁 **Current chunk archived at:** `%s`\n_Use `file_reader` tool to recall details from this chunk._", v.ChunkPath)
			}
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: text}},
			}
		default:
			msg.Content = []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: ""}},
			}
		}

		// Skip entries with empty/invalid roles — these cause API 400 errors.
		// Can be created by compaction round-trips or session resume with
		// corrupted entries. Also skip entries that produced zero content blocks.
		if msg.Role != anthropic.MessageParamRoleUser && msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		if len(msg.Content) == 0 {
			continue
		}

		messages = append(messages, msg)
	}

	// Merge consecutive same-role messages (API requires strict alternation).
	// This handles cases where FixRoleAlternation couldn't merge due to
	// type mismatches (e.g., ToolResultContent + TextContent both user role).
	// The API allows a single user message to contain mixed text and tool_result blocks.
	// IMPORTANT: Never merge messages containing tool_use blocks with other
	// same-role messages — this can obscure the tool_use/tool_result boundary
	// that the API strictly validates.
	merged := make([]anthropic.MessageParam, 0, len(messages))
	for _, msg := range messages {
		if len(merged) > 0 && merged[len(merged)-1].Role == msg.Role {
			// Don't merge if either message contains tool_use blocks.
			// tool_use messages must remain separate to preserve the clear
			// pairing boundary that the API validates.
			if msgHasToolUseBlocks(merged[len(merged)-1]) || msgHasToolUseBlocks(msg) {
				merged = append(merged, msg)
				continue
			}
			merged[len(merged)-1].Content = append(merged[len(merged)-1].Content, msg.Content...)
		} else {
			merged = append(merged, msg)
		}
	}

	// Fix orphaned tool_results: when the compact boundary drops tool_use
	// entries that precede it, their matching tool_results in the kept tail
	// become orphaned. EnsureToolResultPairing would silently strip these,
	// and then insert "Tool execution was interrupted" for the missing results.
	// Instead, inject synthetic tool_use blocks for any tool_result whose
	// tool_use_id is not present in any assistant message's tool_use blocks.
	merged = fixOrphanedToolResults(merged)

	// Openclacky pattern: reasoning pad — if thinking-mode provider detected
	// (any assistant message has reasoning_content), ensure ALL assistant
	// ToolUseContent messages have a thinking block for structural consistency.
	if c.reasoningPadEnabled() {
		for i := range merged {
			if merged[i].Role != anthropic.MessageParamRoleAssistant {
				continue
			}
			hasThinking := false
			hasToolUse := false
			for _, block := range merged[i].Content {
				if block.OfThinking != nil || block.OfRedactedThinking != nil {
					hasThinking = true
				}
				if block.OfToolUse != nil {
					hasToolUse = true
				}
			}
			if hasToolUse && !hasThinking {
				// Prepend empty thinking block for structural consistency
				blocks := make([]anthropic.ContentBlockParamUnion, 1, len(merged[i].Content)+1)
				blocks[0] = anthropic.NewThinkingBlock("", "")
				blocks = append(blocks, merged[i].Content...)
				merged[i].Content = blocks
			}
		}
	}

	return merged
}

// InjectTimeContext adds the current date/time as a user message with the
// system-injected prefix. This replaces the time injection that was previously
// inside the system prompt. By keeping it as a separate injected message, the
// system prompt remains fully static and cacheable, and the time message can
// be skipped for cache breakpoint placement.
func (c *ConversationContext) InjectTimeContext() {
	now := time.Now()
	currentTime := now.Format("2006-01-02 15:04:05")
	_, offset := now.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	timezone := fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)

	timeMsg := fmt.Sprintf("%s[Current time: %s (%s)]", SystemInjectedPrefix, currentTime, timezone)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(timeMsg),
	})
}

// InjectTodoReminder adds the current todo list as a user message with the
// system-injected prefix. This replaces the previous approach of appending
// the todo reminder to the system prompt, which changed the system prompt
// every turn and broke prompt caching. By injecting as a separate message
// with SystemInjectedPrefix, the system prompt stays fully static and
// cacheable, and the todo message is skipped for cache breakpoint placement.
func (c *ConversationContext) InjectTodoReminder(reminder string) {
	if reminder == "" {
		return
	}
	msg := fmt.Sprintf("%s%s\n\n## Important\nUse TodoWrite tool to keep the above task list up to date as you work.", SystemInjectedPrefix, reminder)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(msg),
	})
}

// InjectIdleReminder adds a TodoWrite idle nudge as a user message with the
// system-injected prefix. Used when the model hasn't used TodoWrite for a while
// and has no task list.
func (c *ConversationContext) InjectIdleReminder(idleMsg string) {
	if idleMsg == "" {
		return
	}
	msg := fmt.Sprintf("%s%s", SystemInjectedPrefix, idleMsg)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(msg),
	})
}


// InjectBudgetHint adds a proactive budget management hint as a user message
// with the system-injected prefix.
func (c *ConversationContext) InjectBudgetHint(hint string) {
	if hint == "" {
		return
	}
	msg := fmt.Sprintf("%s%s", SystemInjectedPrefix, hint)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(msg),
	})
}

// InjectSessionState adds session state (tracked files, search patterns, etc.)
// as a user message with the system-injected prefix. This replaces the previous
// approach of appending to the system prompt, which changed the system prompt
// every turn and broke prompt caching.
func (c *ConversationContext) InjectSessionState(state string) {
	if state == "" {
		return
	}
	msg := fmt.Sprintf("%s%s", SystemInjectedPrefix, state)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: TextContent(msg),
	})
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
		// Keep ALL entries and let FixRoleAlternation handle same-role merging.
		// Previously, same-role entries were silently dropped here, causing content loss
		// and the "re-executes historical instructions" bug (important user instructions
		// in Summary/Attachment entries were permanently dropped).
		c.entries = append(first, recent...)

		// After truncation, fix role alternation.
		// Do NOT call ValidateToolPairing here — it fires from AddAssistantToolCalls()
		// BEFORE AddToolResults() has been called, causing Pass 3 to see "missing"
		// tool_results (which are merely pending) and insert error placeholders.
		// ValidateToolPairing is called by BuildMessages() just before the API request,
		// when all tool results are present.
		c.FixRoleAlternation()
	}
}

// TruncateHistory drops older messages to recover from context overflow.
// Keeps the first entry (initial user message) and the last 10 entries.
// Compact-boundary-aware: if compaction has occurred, preserves from the
// boundary through recent entries instead of discarding the summary.
func (c *ConversationContext) TruncateHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) <= 12 {
		return
	}
	keep := 10
	c.entries = c.truncateWithBoundary(1, keep)
	c.ValidateToolPairing()
	c.FixRoleAlternation()
}

// AggressiveTruncateHistory drops more aggressively - keeps only first and last 5.
// Compact-boundary-aware.
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
	c.entries = c.truncateWithBoundary(1, keep)
	c.ValidateToolPairing()
	c.FixRoleAlternation()
}

// PullBackFromTail removes the last k entries from history and returns them.
// This is the core mechanism for two-layer context overflow recovery:
//
//	Layer 1 (standard, K=1): pop 1 message → preserves prompt cache checkpoint #A
//	Layer 2 (aggressive, K=half): pop ~half the history → sacrifices cache but
//	  guarantees the compression call fits.
//
// The pulled-back entries are returned so they can be re-appended after
// compression completes. k is clamped to never remove the first entry
// (system/initial user message).
//
// Matches openclacky's pull_back_from_tail mechanism in
// message_compressor_helper.rb:175-186.
func (c *ConversationContext) PullBackFromTail(k int) []conversationEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	if k <= 0 || len(c.entries) <= 1 {
		return nil
	}

	// Clamp: never pop more than entries-1 (preserve at least the first entry)
	if k >= len(c.entries) {
		k = len(c.entries) - 1
	}

	pulled := make([]conversationEntry, k)
	copy(pulled, c.entries[len(c.entries)-k:])
	c.entries = c.entries[:len(c.entries)-k]

	return pulled
}

// ReAppendEntries re-appends previously pulled-back entries to the end of history.
// Used after compression to restore the most recent task progress that was
// temporarily removed for cache-preserving compaction.
func (c *ConversationContext) ReAppendEntries(entries []conversationEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, entries...)
}

// MinimumHistory drops to bare minimum - only first user message and last 2 entries.
// Compact-boundary-aware.
func (c *ConversationContext) MinimumHistory() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) <= 3 {
		return
	}
	// Keep more entries to preserve tool_use/tool_result pairing.
	// Previous value of 2 was too aggressive and broke pairing, causing 2013.
	// Keep 6 entries: enough to preserve at least one complete tool pair.
	c.entries = c.truncateWithBoundary(1, 6)
	c.ValidateToolPairing()
	c.FixRoleAlternation()
}

// truncateWithBoundary performs a naive truncation but preserves the compaction
// boundary marker and summary if one exists. After compaction, entries look like:
//
//	[0] initial-user, [1] CompactBoundary, [2] Summary, [3..n] attachments+recent
//
// Naive truncation (entries[:1] + recent) would discard entries[1] and [2],
// causing the agent to lose all compressed memory. This function finds the
// boundary and preserves everything from the boundary onwards.
func (c *ConversationContext) truncateWithBoundary(headKeep int, tailKeep int) []conversationEntry {
	// Find the most recent CompactBoundaryContent
	boundaryIdx := -1
	for i := len(c.entries) - 1; i >= 0; i-- {
		if _, ok := c.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdx = i
			break
		}
	}

	if boundaryIdx >= 0 {
		// After compaction: keep from the boundary through recent entries.
		// This preserves the boundary marker, summary, attachments, and
		// recent messages. Don't discard the summary — it's the only memory
		// of what happened before.
		tailStart := len(c.entries) - tailKeep
		if tailStart <= boundaryIdx {
			// Recent portion overlaps with or starts before the boundary —
			// just keep everything from boundary onwards.
			return c.entries[boundaryIdx:]
		}
		// Keep from boundary through end
		return c.entries[boundaryIdx:]
	}

	// No boundary — use naive truncation
	first := c.entries[:headKeep]
	recent := c.entries[len(c.entries)-tailKeep:]
	return append(first, recent...)
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
		case AntiReplayContent:
			msgs = append(msgs, CompactionMessage{
				Role:      entry.role,
				Content:   string(v),
				Timestamp: time.Now().Format(time.RFC3339),
			})
		case GoalContent:
			msgs = append(msgs, CompactionMessage{
				Role:      entry.role,
				Content:   string(v),
				Timestamp: time.Now().Format(time.RFC3339),
			})
		case AttachmentContent:
			msgs = append(msgs, CompactionMessage{
				Role:      entry.role,
				Content:   string(v),
				Timestamp: time.Now().Format(time.RFC3339),
			})
		case CompressionInstructionContent:
			msgs = append(msgs, CompactionMessage{
				Role:      "user",
				Content:   buildCompressionPrompt(v.Level),
				Timestamp: time.Now().Format(time.RFC3339),
			})
		case CompressedSummaryContent:
			msgs = append(msgs, CompactionMessage{
				Role:      "user",
				Content:   v.Summary,
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}
	}

	return msgs, toolNames
}

// compactionMessagesToEntries converts compacted messages back to conversation entries.
// CRITICAL: When JSON deserialization fails for tool_use/tool_result blocks, the
// original pairing metadata (ToolUseID, ToolName) must be preserved by constructing
// proper ToolUseContent/ToolResultContent instead of silently degrading to TextContent.
// Silently degrading to TextContent breaks tool_use/tool_result pairing, causing
// API error 2013 or the model losing awareness of tool calls.
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
			// Deserialization failed — preserve the pairing by constructing a
			// minimal ToolUseContent with the tool_use_id and name from the
			// CompactionMessage. Never degrade to TextContent; that breaks
			// tool_use/tool_result pairing permanently.
			if msg.ToolUseID != "" || msg.ToolName != "" {
				blocks := []anthropic.ContentBlockParamUnion{
					{OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    msg.ToolUseID,
						Name:  msg.ToolName,
						Input: map[string]any{},
					}},
				}
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: ToolUseContent(blocks),
				})
				if msg.ToolName != "" {
					toolNames[key] = msg.ToolName
				}
			} else {
				// Last resort: content looks like tool_use JSON but has no
				// extractable metadata. Keep as text rather than dropping entirely.
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: TextContent(msg.Content),
				})
			}
		} else if isToolResultJSON(msg.Content) {
			// Reconstruct tool result blocks
			if results, err := deserializeToolResultBlocks(msg.Content); err == nil {
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: ToolResultContent(results),
				})
				continue
			}
			// Deserialization failed — preserve the tool_use_id pairing by
			// constructing a minimal ToolResultContent. The tool_use_id is
			// stored in msg.ToolUseID from the serialization step.
			if msg.ToolUseID != "" {
				results := []anthropic.ToolResultBlockParam{
					{ToolUseID: msg.ToolUseID, Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "[tool result content recovered from compacted message]"}},
					}},
				}
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: ToolResultContent(results),
				})
			} else {
				// Last resort: no pairing info available.
				entries = append(entries, conversationEntry{
					role:    msg.Role,
					content: TextContent(msg.Content),
				})
			}
		} else if msg.Role == "assistant" && msg.ToolUseID != "" {
			// This is a tool_use message whose content was cleared by
			// SelectiveCompact (or whose JSON round-trip failed but metadata
			// was preserved). Reconstruct proper ToolUseContent instead of
			// degrading to TextContent, which would break tool pairing.
			blocks := []anthropic.ContentBlockParamUnion{
				{OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    msg.ToolUseID,
					Name:  msg.ToolName,
					Input: map[string]any{},
				}},
			}
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: ToolUseContent(blocks),
			})
			if msg.ToolName != "" {
				toolNames[key] = msg.ToolName
			}
		} else if msg.Role == "user" && msg.ToolUseID != "" {
			// This is a tool_result message whose content was cleared by
			// SelectiveCompact (or whose JSON round-trip failed but tool_use_id
			// was preserved). Reconstruct proper ToolResultContent to maintain
			// the tool_use/tool_result pairing.
			results := []anthropic.ToolResultBlockParam{
				{ToolUseID: msg.ToolUseID, Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: "[tool result compacted]"}},
				}},
			}
			entries = append(entries, conversationEntry{
				role:    msg.Role,
				content: ToolResultContent(results),
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
// LastCompactBoundaryUUID returns the UUID of the most recently added compact
// boundary. Returns "" if no boundary exists.
func (c *ConversationContext) LastCompactBoundaryUUID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := len(c.entries) - 1; i >= 0; i-- {
		if bc, ok := c.entries[i].content.(CompactBoundaryContent); ok {
			return bc.UUID
		}
	}
	return ""
}

func (c *ConversationContext) AddCompactBoundary(trigger CompactTrigger, preCompactTokens int, opts ...func(*CompactBoundaryContent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	content := CompactBoundaryContent{
		Trigger:          trigger,
		PreCompactTokens: preCompactTokens,
		UUID:             generateUUID(),
	}
	// Apply optional configuration functions (for preCompactDiscoveredTools, preservedSegment)
	for _, opt := range opts {
		opt(&content)
	}
	// Cap old compact boundary entries to prevent unbounded accumulation
	// across many compaction cycles. Keep only the 3 most recent.
	c.capCompactBoundaries(3)
	c.entries = append(c.entries, conversationEntry{
		role:    "system",
		content: content,
	})
}

// capCompactBoundaries removes old compact boundary entries, keeping only the
// most recent maxCount boundaries. Also removes their associated SummaryContent,
// AntiReplayContent, and related meta entries. Must be called with c.mu held.
func (c *ConversationContext) capCompactBoundaries(maxCount int) {
	if maxCount <= 0 {
		maxCount = 3
	}
	// Find all compact boundary indices
	var boundaryIdxs []int
	for i := 0; i < len(c.entries); i++ {
		if _, ok := c.entries[i].content.(CompactBoundaryContent); ok {
			boundaryIdxs = append(boundaryIdxs, i)
		}
	}
	// If we have too many, remove the oldest ones
	excess := len(boundaryIdxs) - maxCount
	if excess <= 0 {
		return
	}
	// Determine which indices to remove (the oldest 'excess' boundaries)
	// and their associated entries (summary, anti-replay, etc.)
	remove := make(map[int]bool)
	for i := 0; i < excess; i++ {
		bIdx := boundaryIdxs[i]
		remove[bIdx] = true
		// Also remove entries immediately after the boundary until we hit
		// a non-meta entry (SummaryContent, AntiReplayContent, etc.)
		for j := bIdx + 1; j < len(c.entries); j++ {
			switch c.entries[j].content.(type) {
			case SummaryContent, AntiReplayContent, CompressedSummaryContent:
				remove[j] = true
			default:
				break
			}
		}
	}
	// Build new entries slice without removed indices
	if len(remove) > 0 {
		newEntries := make([]conversationEntry, 0, len(c.entries)-len(remove))
		for i, e := range c.entries {
			if !remove[i] {
				newEntries = append(newEntries, e)
			}
		}
		c.entries = newEntries
	}
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

// CompressionLevel returns the current compression level (0 = never compressed).
func (c *ConversationContext) CompressionLevel() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.compressionLevel
}

// NextCompressionLevel increments and returns the next compression level.
func (c *ConversationContext) NextCompressionLevel() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.compressionLevel++
	return c.compressionLevel
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

// AddAntiReplayRules injects post-compaction rules to prevent re-execution of
// completed tasks. Separated from the summary so it survives further compaction.
func (c *ConversationContext) AddAntiReplayRules() {
	rules := "## Rules After Compaction\n" +
		"1. DO NOT re-execute any task listed in \"Completed Work\" — those are done.\n" +
		"2. Start from the first item in \"Pending Tasks\" that you have not yet completed.\n" +
		"3. Do NOT ask the user what to work on — you already know.\n"
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: AntiReplayContent(rules),
	})
}

// AddGoalBlock injects a structured goal block (pending/completed tasks, current work).
// Separated from the summary so it survives further compaction.
func (c *ConversationContext) AddGoalBlock(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: GoalContent(content),
	})
}

// AddCompressionInstruction injects an inline compression instruction into the
// conversation as a user message. This enables the insert-then-compress pattern
// from openclacky: instead of a separate API call for summarization, the
// instruction is injected and the next LLM call reuses the prompt cache prefix
// (system prompt + tools + prior messages are already cached, only the
// instruction text is new).
func (c *ConversationContext) AddCompressionInstruction(level int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role:    "user",
		content: CompressionInstructionContent{Level: level},
	})
}

// AddCompressedSummary injects a parsed compaction result as a CompressedSummaryContent
// entry. This is the output side of the insert-then-compress pattern: after the
// LLM responds to the compression instruction, we parse <topics> + <summary>,
// then inject this structured entry so it survives further compaction.
// Matches openclacky's parse_compressed_result: role "user", compressed_summary: true.
func (c *ConversationContext) AddCompressedSummary(summary, topics, chunkPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, conversationEntry{
		role: "user",
		content: CompressedSummaryContent{
			Summary:   summary,
			Topics:    topics,
			ChunkPath: chunkPath,
		},
	})
}

// HasCompressionInstruction returns true if the context currently contains
// a CompressionInstructionContent entry that hasn't been processed yet.
func (c *ConversationContext) HasCompressionInstruction() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := len(c.entries) - 1; i >= 0; i-- {
		if _, ok := c.entries[i].content.(CompressionInstructionContent); ok {
			return true
		}
	}
	return false
}

// KeepRecentMessages preserves the most recent conversation entries verbatim
// after compaction, keeping their original structure (including ToolUseContent
// and ToolResultContent). This matches upstream's messagesToKeep mechanism
// (sessionMemoryCompact.ts calculateMessagesToKeepIndex + adjustIndexToPreserveAPIInvariants).
//
// Unlike AddHistorySnip which converts entries to plain text (losing tool structure),
// this method keeps entries as-is so the model can see actual tool_use/tool_result pairs,
// preventing re-execution of commands it already ran.
//
// The method also adjusts the kept range backwards to include any assistant messages
// whose tool_use blocks are referenced by tool_results in the kept range, ensuring
// tool_use/tool_result pairing is never broken (matching upstream's
// adjustIndexToPreserveAPIInvariants).
func (c *ConversationContext) KeepRecentMessages(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count <= 0 {
		count = 8 // default: ~4 tool pairs (~2 turns)
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

	// Collect up to 'count' entries BEFORE the boundary (pre-compact messages)
	// Walk backwards from the boundary, collecting non-meta entries.
	var keptEntries []conversationEntry
	for i := boundaryIdx - 1; i >= 0 && len(keptEntries) < count; i-- {
		entry := c.entries[i]
		switch entry.content.(type) {
		case CompactBoundaryContent, SummaryContent, AttachmentContent, CompressionInstructionContent, AntiReplayContent, GoalContent, CompressedSummaryContent:
			continue // skip meta entries from previous compactions
		default:
			keptEntries = append([]conversationEntry{entry}, keptEntries...)
		}
	}

	if len(keptEntries) == 0 {
		return
	}

	// Adjust backwards to preserve tool_use/tool_result pairing.
	// If any kept entry contains ToolResultContent, collect its tool_use_ids,
	// then walk further backwards to find the assistant messages with matching
	// ToolUseContent blocks. This prevents orphaned tool_results that would
	// cause API error 2013.
	keptEntries = adjustForToolPairing(c.entries[:boundaryIdx], keptEntries)

	// Truncate oversized tool results in the kept tail.
	// Matching openclacky's truncate_tool_result(): cap individual tool results
	// at 2000 chars to prevent one huge output from dominating the tail context.
	truncateKeptToolResults(keptEntries, 2000)

	// Append the kept entries after the boundary+summary as preserved messages
	c.entries = append(c.entries, keptEntries...)
}

// KeepRecentMessagesAdaptive calculates how many recent messages to keep using
// token budgets (matching upstream's calculateMessagesToKeepIndex). It ensures:
//   - At least minTokens tokens are kept (enough context for recovery)
//   - At least minTextMsgs text messages are kept (so the model can see recent text)
//   - At most maxTokens tokens are kept (prevent tail from consuming context)
func (c *ConversationContext) KeepRecentMessagesAdaptive(minTokens, minTextMsgs, maxTokens int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if minTokens <= 0 {
		minTokens = 1000 // ~3 text blocks of assistant output
	}
	if minTextMsgs <= 0 {
		minTextMsgs = 4
	}
	if maxTokens <= 0 {
		maxTokens = 10_000
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

	// Walk backward from boundary, collecting entries until token/min constraints are met.
	// Entries marked as summarized (already included in a previous compaction summary)
	// are skipped — this matches upstream's incremental SM-compact behavior using
	// lastSummarizedMessageId to avoid redundant re-summarization.
	var keptEntries []conversationEntry
	keptStartIdx := -1 // lowest index in c.entries that was kept
	accumTokens := 0
	textMsgCount := 0

	for i := boundaryIdx - 1; i >= 0; i-- {
		entry := c.entries[i]
		if entry.summarized {
			continue // already included in a previous compaction summary
		}
		switch entry.content.(type) {
		case CompactBoundaryContent, SummaryContent, AttachmentContent, CompressionInstructionContent, CompressedSummaryContent:
			continue // skip meta entries
		}

		toks := entryEstimatedTokens(&entry)
		if accumTokens > 0 || textMsgCount > 0 {
			// Already collecting: prepend this entry
			keptEntries = append([]conversationEntry{entry}, keptEntries...)
			accumTokens += toks
			if isTextEntry(&entry) {
				textMsgCount++
			}
		} else {
			// First entry to consider
			keptEntries = append([]conversationEntry{entry}, keptEntries...)
			accumTokens = toks
			if isTextEntry(&entry) {
				textMsgCount++
			}
		}
		keptStartIdx = i

		// Check if we've met all constraints
		if accumTokens >= minTokens && textMsgCount >= minTextMsgs {
			break // met minimum requirements
		}
		// If we've exceeded max tokens, stop collecting
		if accumTokens >= maxTokens {
			break
		}
	}

	if len(keptEntries) == 0 {
		return
	}

	// Adjust backwards to preserve tool_use/tool_result pairing.
	keptEntries = adjustForToolPairing(c.entries[:boundaryIdx], keptEntries)

	// Truncate oversized tool results in the kept tail.
	truncateKeptToolResults(keptEntries, 2000)

	// Update lastSummarizedIndex: the lowest index of kept entries.
	// Entries before this index were summarized/compacted.
	if keptStartIdx >= 0 {
		c.lastSummarizedIndex = keptStartIdx
	}

	// Update compactedEntryCount: how many entries before the boundary were
	// already summarized (for incremental compaction on next round).
	compactedCount := 0
	for i := 0; i < boundaryIdx; i++ {
		switch c.entries[i].content.(type) {
		case CompactBoundaryContent, SummaryContent, AttachmentContent, CompressionInstructionContent, CompressedSummaryContent:
		default:
			compactedCount++
		}
	}
	c.compactedEntryCount = compactedCount

	// Mark kept entries as summarized so they won't be re-summarized on next compaction.
	for i := range keptEntries {
		keptEntries[i].summarized = true
	}

	// Append the kept entries after the boundary+summary as preserved messages
	c.entries = append(c.entries, keptEntries...)
}

// entryEstimatedTokens returns a rough token estimate for a single entry.
func entryEstimatedTokens(entry *conversationEntry) int {
	switch v := entry.content.(type) {
	case TextContent:
		return EstimateTokens(string(v))
	case ToolUseContent:
		toks := 0
		for _, b := range v {
			if b.OfText != nil {
				toks += EstimateTokens(b.OfText.Text)
			}
			if b.OfToolUse != nil {
				toks += len(b.OfToolUse.ID)/4 + len(b.OfToolUse.Name)/4
				if m, ok := b.OfToolUse.Input.(map[string]any); ok {
					for k, val := range m {
						toks += len(k)/4 + len(fmt.Sprintf("%v", val))/4
					}
				}
			}
		}
		return toks
	case ToolResultContent:
		toks := 0
		for _, r := range v {
			for _, c := range r.Content {
				if c.OfText != nil {
					toks += EstimateTokens(c.OfText.Text)
				}
			}
		}
		return toks
	default:
		return 10 // fallback for meta entries
	}
}

// isTextEntry returns true if the entry is a text message (not a tool call/result).
func isTextEntry(entry *conversationEntry) bool {
	switch entry.content.(type) {
	case TextContent, SummaryContent:
		return true
	default:
		return false
	}
}

// truncateKeptToolResults caps individual tool result content at maxChars characters.
// Matching openclacky's truncate_tool_result(): when a tool result in the kept tail
// exceeds maxChars, its content is truncated to the first maxChars chars.
// This prevents one enormous tool output from dominating the preserved context.
func truncateKeptToolResults(entries []conversationEntry, maxChars int) {
	const truncationNotice = "\n[...content truncated...]"
	for i := range entries {
		results, ok := entries[i].content.(ToolResultContent)
		if !ok {
			continue
		}
		for j := range results {
			totalChars := 0
			for _, cb := range results[j].Content {
				if cb.OfText != nil {
					totalChars += len(cb.OfText.Text)
				}
			}
			if totalChars <= maxChars {
				continue
			}
			// Truncate across all text blocks until we've removed enough chars
			remaining := maxChars
			for k := range results[j].Content {
				if cb := results[j].Content[k].OfText; cb != nil {
					if len(cb.Text) > remaining {
						cb.Text = cb.Text[:remaining] + truncationNotice
						remaining = 0
						break
					}
					remaining -= len(cb.Text)
				}
			}
		}
	}
}

// adjustForToolPairing walks backwards from the kept range to include assistant
// messages whose tool_use blocks are referenced by tool_results in the kept range.
// This matches upstream's adjustIndexToPreserveAPIInvariants.
func adjustForToolPairing(preBoundary []conversationEntry, kept []conversationEntry) []conversationEntry {
	// Collect all tool_use_ids from ToolResultContent in the kept range
	var neededToolUseIDs []string
	for _, entry := range kept {
		if results, ok := entry.content.(ToolResultContent); ok {
			for _, r := range results {
				if r.ToolUseID != "" {
					neededToolUseIDs = append(neededToolUseIDs, r.ToolUseID)
				}
			}
		}
	}

	if len(neededToolUseIDs) == 0 {
		return kept
	}

	// Check which tool_use_ids are already present in the kept range
	alreadyPresent := make(map[string]bool)
	for _, entry := range kept {
		if blocks, ok := entry.content.(ToolUseContent); ok {
			for _, b := range blocks {
				if b.OfToolUse != nil && b.OfToolUse.ID != "" {
					alreadyPresent[b.OfToolUse.ID] = true
				}
			}
		}
	}

	// Only need to find tool_uses that are NOT already in the kept range
	var missingIDs []string
	for _, id := range neededToolUseIDs {
		if !alreadyPresent[id] {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) == 0 {
		return kept
	}

	// Walk backwards through pre-boundary entries to find assistant messages
	// containing the missing tool_use blocks
	missingSet := make(map[string]bool, len(missingIDs))
	for _, id := range missingIDs {
		missingSet[id] = true
	}

	var additionalEntries []conversationEntry
	for i := len(preBoundary) - 1; i >= 0 && len(missingSet) > 0; i-- {
		entry := preBoundary[i]
		if entry.role != "assistant" {
			continue
		}
		if blocks, ok := entry.content.(ToolUseContent); ok {
			hasMatch := false
			for _, b := range blocks {
				if b.OfToolUse != nil && missingSet[b.OfToolUse.ID] {
					missingSet[b.OfToolUse.ID] = false // mark as found
					hasMatch = true
				}
			}
			if hasMatch {
				additionalEntries = append([]conversationEntry{entry}, additionalEntries...)
			}
		}
	}

	// Prepend additional entries to the kept range
	if len(additionalEntries) > 0 {
		return append(additionalEntries, kept...)
	}
	return kept
}

// compactableToolNames is defined in compact.go (shared across package main).

// MicroCompactEntries clears content of old tool results beyond the keepRecent
// window. Operates directly on conversation entries (no serialization round-trip).
// Returns the number of tool result entries that were cleared.
// ToolUseID is preserved in cleared results to maintain pairing validity.
//
// Improvements:
//  1. Dedup: skips tool results already cleared to the placeholder string.
//  2. Whitelist: only clears results from compactable tools (read/exec/edit/grep/glob/web/write).
//  3. Size threshold: only clears results whose text content >= minCharCount chars.
//     This preserves small useful results (error messages, short outputs) while still
//     freeing context from large file reads and command outputs. Matches upstream's
//     approach of only clearing results that actually save significant tokens.
func (c *ConversationContext) MicroCompactEntries(keepRecent int, placeholder string, minCharCount int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if keepRecent <= 0 {
		keepRecent = 5
	}
	if placeholder == "" {
		placeholder = "[Old tool result content cleared]"
	}
	if minCharCount <= 0 {
		minCharCount = 2000 // default: only clear results >= 2000 chars
	}

	// Reset replacement tracking at the start of each micro-compact pass.
	// BuildMessages() will apply replacements during serialization.
	c.toolResultReplacements = make(map[string]string)
	c.clearedToolResults = make(map[string]bool)

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

	// Pass 2: Iterate backwards, recording replacements for eligible tool results.
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

		// Preserve error results — they contain important debugging info
		hasError := false
		for _, r := range results {
			if r.IsError.Valid() && r.IsError.Value {
				hasError = true
				break
			}
		}
		if hasError {
			continue
		}

		// Check each block: is it already cleared? is it a compactable tool? is it large enough?
		allCleared := true
		hasCompactable := false
		for _, r := range results {
			// Check if already cleared (tracked by ID instead of content inspection)
			if c.clearedToolResults[r.ToolUseID] {
				continue
			}
			allCleared = false

			// Check if this tool is compactable AND its content is large enough to justify clearing.
			if toolName, ok := toolNameMap[r.ToolUseID]; ok && compactableToolNames[toolName] {
				totalChars := 0
				for _, cb := range r.Content {
					if cb.OfText != nil {
						totalChars += len(cb.OfText.Text)
					}
				}
				if totalChars >= minCharCount {
					hasCompactable = true
				}
			}
		}

		// Skip if all blocks are already cleared, or none are compactable
		if allCleared || !hasCompactable {
			continue
		}

		// Record replacements for compactable tool results that are large enough
		for _, r := range results {
			toolName, ok := toolNameMap[r.ToolUseID]
			if !ok || !compactableToolNames[toolName] {
				continue
			}
			totalChars := 0
			for _, cb := range r.Content {
				if cb.OfText != nil {
					totalChars += len(cb.OfText.Text)
				}
			}
			if totalChars < minCharCount {
				continue // Too small to justify clearing — preserve the result
			}

			// Already replaced in this pass
			if c.clearedToolResults[r.ToolUseID] {
				continue
			}

			// Persist the content to disk before clearing from context.
			// The model can re-read the result on demand without re-executing.
			contentText := ""
			for _, cb := range r.Content {
				if cb.OfText != nil {
					contentText += cb.OfText.Text
				}
			}
			diskRef := ""
			if c.toolResultStore != nil && contentText != "" {
				result := c.toolResultStore.Persist(r.ToolUseID, contentText)
				if result != nil {
					diskRef = buildLargeToolResultMessage(result)
				}
			}
			blockText := diskRef
			if blockText == "" {
				blockText = placeholder
			}

			// Record replacement instead of mutating entry
			c.toolResultReplacements[r.ToolUseID] = blockText
			c.clearedToolResults[r.ToolUseID] = true
		}
		if len(c.clearedToolResults) > cleared {
			cleared++
		}
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

// fixOrphanedToolResults fixes tool_result entries whose matching tool_use was
// dropped by the compact boundary in BuildMessages. Without this, orphaned
// tool_results are silently stripped by EnsureToolResultPairing, and synthetic
// "Tool execution was interrupted" errors are inserted instead, causing the LLM
// to believe tools are broken.
//
// For each orphaned tool_result, we inject a synthetic tool_use block into the
// preceding assistant message (using inferred tool name from result content).
// This preserves the real tool result while satisfying EnsureToolResultPairing.
func fixOrphanedToolResults(messages []anthropic.MessageParam) []anthropic.MessageParam {
	// Step 1: Collect all tool_use IDs from assistant messages
	allToolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role != anthropic.MessageParamRoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.OfToolUse != nil && block.OfToolUse.ID != "" {
				allToolUseIDs[block.OfToolUse.ID] = true
			}
		}
	}

	// Step 2: Find orphaned tool_results and pair them with their preceding
	// assistant message by injecting synthetic tool_use blocks
	result := make([]anthropic.MessageParam, len(messages))
	copy(result, messages)

	for i, msg := range result {
		if msg.Role != anthropic.MessageParamRoleUser {
			continue
		}
		// Find orphaned tool_results in this user message
		var orphaned []anthropic.ToolResultBlockParam
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				id := block.OfToolResult.ToolUseID
				if id != "" && !allToolUseIDs[id] {
					orphaned = append(orphaned, *block.OfToolResult)
					allToolUseIDs[id] = true // mark as handled so we don't double-inject
				}
			}
		}
		if len(orphaned) == 0 {
			continue
		}

		// Inject synthetic tool_use into preceding assistant message
		if i > 0 && result[i-1].Role == anthropic.MessageParamRoleAssistant {
			synthBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(orphaned))
			for _, o := range orphaned {
				toolName := inferToolNameFromResult(o)
				synthBlocks = append(synthBlocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    o.ToolUseID,
						Name:  toolName,
						Input: map[string]any{},
					},
				})
			}
			// Prepend synthetic tool_use blocks to the existing content
			newContent := make([]anthropic.ContentBlockParamUnion, 0, len(synthBlocks)+len(result[i-1].Content))
			newContent = append(newContent, synthBlocks...)
			newContent = append(newContent, result[i-1].Content...)
			result[i-1].Content = newContent
		}
	}

	return result
}

// inferToolNameFromResult attempts to infer a tool name from an orphaned tool_result.
// Since the original tool_use is missing, we use content heuristics to provide
// a meaningful placeholder that preserves conversation context.
func inferToolNameFromResult(r anthropic.ToolResultBlockParam) string {
	// Check the content for clues about what tool was used
	for _, c := range r.Content {
		if c.OfText != nil {
			text := c.OfText.Text
			// Heuristics based on common tool output patterns
			if strings.Contains(text, "lines") && strings.Contains(text, "───") {
				return "read_file"
			}
			if strings.Contains(text, "$ ") || strings.Contains(text, "> ") {
				return "bash"
			}
			if strings.Contains(text, "commit") || strings.Contains(text, "branch") {
				return "git"
			}
			if strings.Contains(text, "Found") && strings.Contains(text, "match") {
				return "grep"
			}
			if strings.Contains(text, "wrote") || strings.Contains(text, "modified") {
				return "edit_file"
			}
			if strings.Contains(text, "directory") || strings.Contains(text, "files") {
				return "list_directory"
			}
		}
	}
	return "unknown_tool"
}

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

	// Pass 2: Backfill orphaned tool_results with synthetic tool_use blocks.
	// When resuming a session, a tool execution may have completed before the
	// session was interrupted, leaving a tool_result without a matching tool_use.
	// Instead of discarding the result (losing context), we create a synthetic
	// tool_use block with a descriptive placeholder and keep the original result.
	resultIDs := make(map[string]bool)
	// Track orphan results per entry index
	type orphanResult struct {
		r anthropic.ToolResultBlockParam
	}
	orphanByIndex := make(map[int][]orphanResult)

	for i, entry := range c.entries {
		if entry.role == "user" {
			if results, ok := entry.content.(ToolResultContent); ok {
				var valid []anthropic.ToolResultBlockParam
				for _, r := range results {
					if callIDs[r.ToolUseID] {
						valid = append(valid, r)
						resultIDs[r.ToolUseID] = true
					} else {
						orphanByIndex[i] = append(orphanByIndex[i], orphanResult{r})
					}
				}
				// For mixed case (valid + orphans): keep ALL results, not just valid.
				// The orphan injection step below will create synthetic tool_use
				// for orphans, so the final entry needs all results to match.
				// Previously we were dropping orphans here, causing 2013 errors.
				_ = valid // placeholder for clarity - we don't modify entry
			}
		}
	}

	// Build final entries: inject synthetic tool_use before orphaned tool_results
	if len(orphanByIndex) > 0 {
		finalEntries := make([]conversationEntry, 0, len(c.entries)+len(orphanByIndex))
		for i, entry := range c.entries {
			if orphans, hasOrphans := orphanByIndex[i]; hasOrphans {
				// Create synthetic tool_use blocks for all orphaned results
				var synthBlocks []anthropic.ContentBlockParamUnion
				for _, o := range orphans {
					toolName := inferToolNameFromResult(o.r)
					synthBlocks = append(synthBlocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    o.r.ToolUseID,
							Name:  toolName,
							Input: map[string]any{},
						},
					})
				}
				finalEntries = append(finalEntries, conversationEntry{
					role:    "assistant",
					content: ToolUseContent(synthBlocks),
				})
				// Then append the original entry with all orphaned results
			}
			finalEntries = append(finalEntries, entry)
		}
		c.entries = finalEntries
	}

	// Pass 3: Insert synthetic tool_results for tool_use blocks without matching results.
	// After compaction, a tool_use block may survive in the kept tail while its
	// tool_result was in the summarized portion. The API requires every tool_use
	// to have a corresponding tool_result — without one, it returns error 2013.
	// Insert a synthetic error result right after the assistant message containing
	// the unpaired tool_use. This matches upstream's ensureToolResultPairing.
	var missingIDs []string
	for id := range callIDs {
		if !resultIDs[id] {
			missingIDs = append(missingIDs, id)
		}
	}
	if len(missingIDs) > 0 {
		missingSet := make(map[string]bool, len(missingIDs))
		for _, id := range missingIDs {
			missingSet[id] = true
		}
		placeholder := "[Tool result missing due to internal error]"
		var newEntries []conversationEntry
		for _, entry := range c.entries {
			newEntries = append(newEntries, entry)
			if entry.role == "assistant" {
				if blocks, ok := entry.content.(ToolUseContent); ok {
					var synthResults []anthropic.ToolResultBlockParam
					for _, b := range blocks {
						if b.OfToolUse != nil && missingSet[b.OfToolUse.ID] {
							synthResults = append(synthResults, anthropic.ToolResultBlockParam{
								ToolUseID: b.OfToolUse.ID,
								Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: placeholder}}},
								IsError:   anthropic.Bool(true),
							})
							delete(missingSet, b.OfToolUse.ID)
						}
					}
					if len(synthResults) > 0 {
						newEntries = append(newEntries, conversationEntry{
							role:    "user",
							content: ToolResultContent(synthResults),
						})
						// If the next entry is also a user-role message (e.g., another
						// tool_result), FixRoleAlternation will merge them later.
					}
				}
			}
		}
		c.entries = newEntries
	}
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
				// Merge same-role consecutive messages.
				// For user-role: also merge compact-generated types (SummaryContent,
				// AttachmentContent) with TextContent to avoid multiple consecutive
				// user-role messages, which the Anthropic API rejects as error 2013.
				// Note: ToolResultContent is NOT merged with TextContent - doing so
				// would destroy the tool_use/tool_result pairing.
				wasMerged := false
				switch a := last.content.(type) {
				case TextContent:
					if b, ok := entry.content.(TextContent); ok {
						last.content = TextContent(string(a) + "\n" + string(b))
						wasMerged = true
					} else if entry.role == "user" {
						switch entry.content.(type) {
						case SummaryContent, AttachmentContent, AntiReplayContent, GoalContent:
							last.content = TextContent(string(a) + "\n" + entryContentToText(entry.content))
							wasMerged = true
						}
					}
				case SummaryContent:
					if entry.role == "user" {
						switch entry.content.(type) {
						case SummaryContent, TextContent, AttachmentContent, AntiReplayContent, GoalContent:
							last.content = TextContent(string(a) + "\n" + entryContentToText(entry.content))
							wasMerged = true
						}
					}
				case AttachmentContent:
					if entry.role == "user" {
						switch entry.content.(type) {
						case SummaryContent, TextContent, AttachmentContent, AntiReplayContent, GoalContent:
							last.content = TextContent(string(a) + "\n" + entryContentToText(entry.content))
							wasMerged = true
						}
					}
				case AntiReplayContent:
					if entry.role == "user" {
						switch entry.content.(type) {
						case SummaryContent, TextContent, AttachmentContent, AntiReplayContent, GoalContent:
							last.content = TextContent(string(a) + "\n" + entryContentToText(entry.content))
							wasMerged = true
						}
					}
				case GoalContent:
					if entry.role == "user" {
						switch entry.content.(type) {
						case SummaryContent, TextContent, AttachmentContent, AntiReplayContent, GoalContent:
							last.content = TextContent(string(a) + "\n" + entryContentToText(entry.content))
							wasMerged = true
						}
					}
				case ToolUseContent:
					if b, ok := entry.content.(ToolUseContent); ok {
						last.content = append(a, b...)
						wasMerged = true
					}
				case ToolResultContent:
					if b, ok := entry.content.(ToolResultContent); ok {
						last.content = append(a, b...)
						wasMerged = true
					}
				}
				if wasMerged {
					continue
				}
			}
		}
		merged = append(merged, entry)
	}
	c.entries = merged
}

// entryContentToText serializes any EntryContent to a plain text string.
// Used by FixRoleAlternation to handle type-mismatched same-role entries.
// TurnInterruptionKind represents how a session was interrupted on resume.
// Matching upstream's TurnInterruptionState (conversationRecovery.ts:139-142).
type TurnInterruptionKind string

const (
	// TurnInterruptedNone means the session completed normally or has no
	// indication of interruption.
	TurnInterruptedNone TurnInterruptionKind = "none"
	// TurnInterruptedPrompt means the user's prompt was never acted upon.
	// The assistant never started responding, so the prompt is still pending.
	TurnInterruptedPrompt TurnInterruptionKind = "interrupted_prompt"
	// TurnInterruptedTurn means the assistant was in the middle of responding
	// when interrupted. The model started a turn but didn't finish it.
	TurnInterruptedTurn TurnInterruptionKind = "interrupted_turn"
)

// TurnInterruptionState holds the result of interruption detection.
type TurnInterruptionState struct {
	Kind       TurnInterruptionKind
	PromptText string // the user prompt text if interrupted_prompt
}

// DetectTurnInterruption analyzes the conversation entries to determine if the
// session was interrupted mid-turn. This matches upstream's detectTurnInterruption
// (conversationRecovery.ts:272-333).
//
// Returns:
//   - TurnInterruptedNone: session completed normally or empty
//   - TurnInterruptedPrompt: user prompt never received a response
//   - TurnInterruptedTurn: assistant was mid-response when interrupted
func (c *ConversationContext) DetectTurnInterruption() TurnInterruptionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return detectTurnInterruptionLocked(c.entries)
}

// detectTurnInterruptionLocked performs the actual detection. Must be called with lock held.
func detectTurnInterruptionLocked(entries []conversationEntry) TurnInterruptionState {
	if len(entries) == 0 {
		return TurnInterruptionState{Kind: TurnInterruptedNone}
	}

	// Find the last turn-relevant entry, skipping system and compact boundary entries.
	// These are bookkeeping artifacts that should not mask a genuine interruption.
	lastIdx := -1
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.role != "system" {
			// Also skip compact boundary markers stored as system entries with
			// CompactBoundaryContent
			if _, ok := e.content.(CompactBoundaryContent); ok {
				continue
			}
			lastIdx = i
			break
		}
	}
	if lastIdx == -1 {
		return TurnInterruptionState{Kind: TurnInterruptedNone}
	}

	last := entries[lastIdx]

	// Assistant entry: check if it's a tool_use-only entry without matching results.
	// In Go's transcript recording, assistant tool_use entries are recorded when
	// the model requests tools. If no tool_result follows, the turn was interrupted.
	if last.role == "assistant" {
		switch last.content.(type) {
		case ToolUseContent:
			// Assistant requested tools but results were never received — interrupted turn
			return TurnInterruptionState{Kind: TurnInterruptedTurn}
		default:
			// Text response — turn most likely completed normally
			return TurnInterruptionState{Kind: TurnInterruptedNone}
		}
	}

	// User entry: the assistant never responded to this prompt.
	if last.role == "user" {
		// Check if it's a meta/system-generated message (summary, compact)
		switch last.content.(type) {
		case SummaryContent:
			// Compact summary — not a user prompt, session completed normally
			return TurnInterruptionState{Kind: TurnInterruptedNone}
		case AttachmentContent:
			// Attachments are part of the user turn but the assistant never responded
			return TurnInterruptionState{Kind: TurnInterruptedTurn}
		case ToolResultContent:
			// Tool result without a matching tool_use in subsequent entries means
			// the tool was called but no follow-up text was generated.
			// Check if this is a terminal tool result (brief mode / send message).
			// Since we can't easily look back for the tool_use here (entries may
			// have been flushed), treat as interrupted_turn conservatively.
			// The upstream checks for SendUserMessage/BriefTool terminal results.
			return TurnInterruptionState{Kind: TurnInterruptedTurn}
		default:
			// Plain text user prompt — assistant hadn't started responding
			text := ""
			if tc, ok := last.content.(TextContent); ok {
				text = string(tc)
			}
			return TurnInterruptionState{Kind: TurnInterruptedPrompt, PromptText: text}
		}
	}

	// Tool use entry: assistant requested tools but results were never received.
	if last.role == "assistant" {
		if _, ok := last.content.(ToolUseContent); ok {
			return TurnInterruptionState{Kind: TurnInterruptedTurn}
		}
	}

	return TurnInterruptionState{Kind: TurnInterruptedNone}
}

// ApplyTurnInterruptionResume handles the resume logic after detecting an interruption.
// For interrupted_turn: injects "Continue from where you left off." user message.
// For interrupted_prompt: no injection needed, the prompt is already there.
// This matches upstream's conversationRecovery.ts:206-245.
func (c *ConversationContext) ApplyTurnInterruptionResume(state TurnInterruptionState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch state.Kind {
	case TurnInterruptedTurn:
		// Mid-turn interruption: inject a synthetic continuation message.
		// This transforms interrupted_turn into interrupted_prompt by appending
		// a synthetic continuation message, matching upstream's design.
		c.entries = append(c.entries, conversationEntry{
			role:    "user",
			content: TextContent("Continue from where you left off."),
		})
	case TurnInterruptedPrompt:
		// Prompt was never acted upon — no injection needed, the existing
		// user prompt will be sent when the agent loop runs.
		// However, append a synthetic assistant sentinel so the conversation
		// is API-valid if no action is taken (matching upstream:231-245).
		c.entries = append(c.entries, conversationEntry{
			role:    "assistant",
			content: TextContent(NO_RESPONSE_REQUESTED),
		})
	}
}

// NO_RESPONSE_REQUESTED is a synthetic assistant message placeholder used to
// make conversations API-valid when resuming from interrupted prompts.
const NO_RESPONSE_REQUESTED = "[Response requested]"

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
	case AntiReplayContent:
		return string(v)
	case GoalContent:
		return string(v)
	case CompressionInstructionContent:
		return buildCompressionPrompt(v.Level)
	case CompressedSummaryContent:
		return v.Summary
	default:
		return ""
	}
}

// msgHasToolResultBlocks returns true if the message contains tool_result blocks.
func msgHasToolResultBlocks(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return true
		}
	}
	return false
}

// msgHasToolUseBlocks returns true if the message contains tool_use blocks.
func msgHasToolUseBlocks(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			return true
		}
	}
	return false
}
