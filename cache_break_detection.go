package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// CacheBreakTracker implements two-phase cache break detection matching upstream's
// promptCacheBreakDetection.ts. Phase 1 (pre-call) hashes system prompt, tool
// schemas, and parameters. Phase 2 (post-call) checks cache_read_tokens for
// >5% drops and explains why.
type CacheBreakTracker struct {
	prevSystemHash       uint64
	prevToolsHash        uint64
	prevCacheControlHash uint64
	prevSystemCharCount  int
	prevToolNames        []string
	prevPerToolHashes    map[string]uint64
	prevModel            string
	prevFastMode         bool
	prevCallCount          int
	prevCacheReadTokens  *int64
	pendingChanges       *PendingCacheChanges
	lastAssistantMsgTime time.Time
}

// PendingCacheChanges records what changed between calls for phase 2 explanation.
type PendingCacheChanges struct {
	SystemPromptChanged     bool
	ToolSchemasChanged      bool
	ModelChanged            bool
	FastModeChanged         bool
	CacheControlChanged     bool
	SystemCharDelta         int
	AddedToolCount          int
	RemovedToolCount        int
	AddedTools              []string
	RemovedTools            []string
	ChangedToolSchemas      []string
	PreviousModel           string
	NewModel                string
	PrevCacheControlHash    uint64
	NewCacheControlHash     uint64
}

// --- Helper functions ---

// Global tracker instance (package-level singleton).
var globalCacheBreakTracker = &CacheBreakTracker{}

// RecordPromptState is phase 1 (pre-call): hashes current state and detects changes.
// Returns pending changes (if any) for phase 2 to use in explanation.
func RecordPromptState(
	systemContent string,
	toolSchemas []map[string]any,
	toolNames []string,
	model string,
	fastMode bool,
) *PendingCacheChanges {
	// Compute hashes
	systemHash := fnvHashString(systemContent)
	toolsHash := fnvHashJSON(toolSchemas)

	// Compute cache control hash — in our implementation, we always place exactly
	// one cache_control marker. The position depends on skipCacheWrite.
	// For tracking, we hash the count of cache_control markers in the system prompt.
	cacheControlHash := fnvHashString(fmt.Sprintf("cache_control_count=%d", strings.Count(systemContent, "ephemeral")))

	// Compute per-tool hashes for change attribution
	perToolHashes := make(map[string]uint64, len(toolSchemas))
	for i, schema := range toolSchemas {
		if i < len(toolNames) {
			perToolHashes[toolNames[i]] = fnvHashJSON(schema)
		}
	}

	state := globalCacheBreakTracker
	prev := state.prevSystemHash

	if prev == 0 && state.prevCallCount == 0 {
		// First call — record baseline
		state.prevSystemHash = systemHash
		state.prevToolsHash = toolsHash
		state.prevCacheControlHash = cacheControlHash
		state.prevSystemCharCount = len(systemContent)
		state.prevToolNames = toolNames
		state.prevPerToolHashes = perToolHashes
		state.prevModel = model
		state.prevFastMode = fastMode
		state.prevCallCount = 1
		return nil
	}

	state.prevCallCount++

	// Detect changes
	changes := &PendingCacheChanges{
		SystemPromptChanged:  systemHash != state.prevSystemHash,
		ToolSchemasChanged:   toolsHash != state.prevToolsHash,
		ModelChanged:         model != state.prevModel,
		FastModeChanged:      fastMode != state.prevFastMode,
		CacheControlChanged:  cacheControlHash != state.prevCacheControlHash,
		SystemCharDelta:      len(systemContent) - state.prevSystemCharCount,
		PreviousModel:        state.prevModel,
		NewModel:             model,
		PrevCacheControlHash: state.prevCacheControlHash,
		NewCacheControlHash:  cacheControlHash,
	}

	// Compute tool set diffs if tools changed
	prevToolSet := make(map[string]bool)
	for _, name := range state.prevToolNames {
		prevToolSet[name] = true
	}
	newToolSet := make(map[string]bool)
	for _, name := range toolNames {
		newToolSet[name] = true
	}

	for _, name := range toolNames {
		if !prevToolSet[name] {
			changes.AddedTools = append(changes.AddedTools, name)
		}
	}
	for _, name := range state.prevToolNames {
		if !newToolSet[name] {
			changes.RemovedTools = append(changes.RemovedTools, name)
		}
	}
	changes.AddedToolCount = len(changes.AddedTools)
	changes.RemovedToolCount = len(changes.RemovedTools)

	// Check which individual tool schemas changed
	if changes.ToolSchemasChanged {
		for _, name := range toolNames {
			if prevToolSet[name] {
				newHash := perToolHashes[name]
				if oldHash, ok := state.prevPerToolHashes[name]; ok && newHash != oldHash {
					changes.ChangedToolSchemas = append(changes.ChangedToolSchemas, name)
				}
			}
		}
	}

	// Store pending changes if anything changed
	anythingChanged := changes.SystemPromptChanged ||
		changes.ToolSchemasChanged ||
		changes.ModelChanged ||
		changes.FastModeChanged ||
		changes.CacheControlChanged

	if anythingChanged {
		state.pendingChanges = changes
	} else {
		state.pendingChanges = nil
	}

	// Update baseline
	state.prevSystemHash = systemHash
	state.prevToolsHash = toolsHash
	state.prevCacheControlHash = cacheControlHash
	state.prevSystemCharCount = len(systemContent)
	state.prevToolNames = toolNames
	state.prevPerToolHashes = perToolHashes
	state.prevModel = model
	state.prevFastMode = fastMode

	return state.pendingChanges
}

// CheckResponseForCacheBreak is phase 2 (post-call): checks if cache broke and explains why.
// Returns (isCacheBreak bool, reason string).
func CheckResponseForCacheBreak(
	cacheReadTokens int64,
	cacheCreationTokens int64,
	timeSinceLastAssistantMsg *time.Duration,
	cacheDeletionsPending bool,
	compactionJustOccurred bool,
) (bool, string) {
	state := globalCacheBreakTracker

	// Skip first call — no previous value to compare
	if state.prevCacheReadTokens == nil {
		state.prevCacheReadTokens = &cacheReadTokens
		return false, ""
	}

	prevCacheRead := *state.prevCacheReadTokens
	state.prevCacheReadTokens = &cacheReadTokens

	// Cache deletions via cached microcompact intentionally reduce the cached prefix.
	// The drop is expected — reset baseline.
	if cacheDeletionsPending {
		// Expected drop, not a break
		state.pendingChanges = nil
		return false, "cache deletion (expected)"
	}

	// Compaction legitimately reduces message count.
	if compactionJustOccurred {
		state.pendingChanges = nil
		return false, "compaction (expected drop)"
	}

	// Detect cache break: cache read dropped >5% AND absolute drop > 2000 tokens.
	const minCacheMissTokens = 2000
	tokenDrop := prevCacheRead - cacheReadTokens

	if cacheReadTokens >= int64(float64(prevCacheRead)*0.95) || tokenDrop < minCacheMissTokens {
		state.pendingChanges = nil
		return false, ""
	}

	// Cache break detected — build explanation from pending changes.
	parts := buildCacheBreakExplanation(state.pendingChanges, timeSinceLastAssistantMsg, tokenDrop, prevCacheRead, cacheReadTokens)
	state.pendingChanges = nil

	return true, parts
}

// buildCacheBreakExplanation constructs a human-readable reason for the cache break.
func buildCacheBreakExplanation(
	changes *PendingCacheChanges,
	timeSinceLastAssistantMsg *time.Duration,
	tokenDrop int64,
	prevCacheRead int64,
	cacheReadTokens int64,
) string {
	var parts []string

	if changes != nil {
		if changes.ModelChanged {
			parts = append(parts, fmt.Sprintf("model changed (%s → %s)", changes.PreviousModel, changes.NewModel))
		}
		if changes.SystemPromptChanged {
			charInfo := ""
			if changes.SystemCharDelta != 0 {
				if changes.SystemCharDelta > 0 {
					charInfo = fmt.Sprintf(" (+%d chars)", changes.SystemCharDelta)
				} else {
					charInfo = fmt.Sprintf(" (%d chars)", changes.SystemCharDelta)
				}
			}
			parts = append(parts, fmt.Sprintf("system prompt changed%s", charInfo))
		}
		if changes.ToolSchemasChanged {
			toolInfo := ""
			if changes.AddedToolCount > 0 || changes.RemovedToolCount > 0 {
				toolInfo = fmt.Sprintf(" (+%d/-%d tools)", changes.AddedToolCount, changes.RemovedToolCount)
			} else if len(changes.ChangedToolSchemas) > 0 {
				toolInfo = fmt.Sprintf(" (schemas: %s)", strings.Join(changes.ChangedToolSchemas, ", "))
			} else {
				toolInfo = " (tool prompt/schema changed, same tool set)"
			}
			parts = append(parts, fmt.Sprintf("tools changed%s", toolInfo))
		}
		if changes.FastModeChanged {
			parts = append(parts, "fast mode toggled")
		}
		if changes.CacheControlChanged {
			parts = append(parts, "cache_control metadata changed (scope or TTL)")
		}
	}

	if len(parts) == 0 {
		// No client-side changes detected — likely server-side
		if timeSinceLastAssistantMsg != nil {
			if *timeSinceLastAssistantMsg > time.Hour {
				parts = append(parts, "possible 1h TTL expiry (prompt unchanged)")
			} else if *timeSinceLastAssistantMsg > 5*time.Minute {
				parts = append(parts, "possible 5min TTL expiry (prompt unchanged)")
			} else {
				parts = append(parts, fmt.Sprintf("likely server-side eviction (prompt unchanged, %.0f min gap)", timeSinceLastAssistantMsg.Minutes()))
			}
		} else {
			parts = append(parts, "unknown cause (prompt unchanged)")
		}
	}

	return fmt.Sprintf(
		"CACHE BREAK: %s [drop: %d → %d (%d tokens)]",
		strings.Join(parts, ", "),
		prevCacheRead,
		cacheReadTokens,
		tokenDrop,
	)
}

// ResetCacheBreakTracker resets the tracker (e.g., after /clear or new session).
func ResetCacheBreakTracker() {
	globalCacheBreakTracker = &CacheBreakTracker{}
}

// NotifyCacheDeletion flags that cache_edits deletions were sent.
// The next API response will have lower cache_read_tokens — that's expected.
func NotifyCacheDeletion() {
	// This would set a flag that CheckResponseForCacheBreak checks.
	// For simplicity, the caller passes cacheDeletionsPending=true.
}

// UpdateLastAssistantMsgTime records when the last assistant message was received.
// Used for TTL detection in phase 2.
func UpdateLastAssistantMsgTime() {
	globalCacheBreakTracker.lastAssistantMsgTime = time.Now()
}

// TimeSinceLastAssistantMsg returns the duration since the last assistant message.
func TimeSinceLastAssistantMsg() *time.Duration {
	t := globalCacheBreakTracker.lastAssistantMsgTime
	if t.IsZero() {
		return nil
	}
	d := time.Since(t)
	return &d
}

// --- Helper functions ---

func fnvHashString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func fnvHashJSON(v any) uint64 {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64()
}

// CacheBreakStateSnapshot is a serializable snapshot for logging.
type CacheBreakStateSnapshot struct {
	SystemHash       uint64   `json:"system_hash"`
	ToolsHash        uint64   `json:"tools_hash"`
	CacheControlHash uint64   `json:"cache_control_hash"`
	ToolNames        []string `json:"tool_names"`
	Model            string   `json:"model"`
	FastMode         bool     `json:"fast_mode"`
	CallCount        int      `json:"call_count"`
	CacheReadTokens  *int64   `json:"cache_read_tokens,omitempty"`
}

// GetCacheBreakSnapshot returns the current state for debugging.
func GetCacheBreakSnapshot() CacheBreakStateSnapshot {
	state := globalCacheBreakTracker
	return CacheBreakStateSnapshot{
		SystemHash:       state.prevSystemHash,
		ToolsHash:        state.prevToolsHash,
		CacheControlHash: state.prevCacheControlHash,
		Model:            state.prevModel,
		FastMode:         state.prevFastMode,
		CallCount:        state.prevCallCount,
		CacheReadTokens:  state.prevCacheReadTokens,
	}
}
