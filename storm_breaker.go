package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
)

// StormBreaker detects and suppresses repeat-loop tool call storms.
// This matches DeepSeek-Reasonix's StormBreaker pattern: when the LLM
// calls the same tool with identical arguments multiple times in a row
// (e.g., reading the same file 3+ times), subsequent calls are suppressed.
//
// Key design decisions:
//   - Mutating calls (edit_file, write_file) clear prior read-only entries
//     from the window so post-edit verification reads are not falsely flagged
//   - 3 identical mutating calls in a row still triggers suppression (genuine loop)
//   - Cheap state-inspection tools are exempt from the check
type StormBreaker struct {
	mu        sync.Mutex
	recent    []stormEntry
	window    int // max entries in window (default 20)
	threshold int // repeat count to trigger suppression (default 3)
}

type stormEntry struct {
	name     string
	argsHash string // first 8 chars of SHA-256 of arguments JSON
	mutating bool
}

// stormExempt tools that bypass storm detection (cheap state-inspection tools).
var stormExempt = map[string]bool{
	"list_dir":    true,
	"glob":        true,
	"tool_search": true,
}

// mutatingTools are tools that modify state (writes, deletes, etc.)
var mutatingTools = map[string]bool{
	"edit_file":  true,
	"multi_edit": true,
	"write_file": true,
	"fileops":    true,
	"exec":       true,
}

// NewStormBreaker creates a storm breaker with default settings.
func NewStormBreaker() *StormBreaker {
	return &StormBreaker{
		window:    20,
		threshold: 3,
	}
}

// Inspect checks if a tool call is a repeat storm. Returns a reason string
// if the call should be suppressed, or empty string if allowed.
func (b *StormBreaker) Inspect(toolName string, input map[string]any) string {
	// Exempt cheap state-inspection tools
	if stormExempt[toolName] {
		return ""
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	argsHash := hashArgs(input)
	mutating := mutatingTools[toolName]

	if mutating {
		// Drop prior read-only entries — the file/shell state just changed,
		// so a verify-read after this should start with a clean slate.
		// Keep mutator entries: 3 identical edits in a row is still a storm.
		newRecent := make([]stormEntry, 0, len(b.recent))
		for _, e := range b.recent {
			if !e.mutating {
				continue
			}
			newRecent = append(newRecent, e)
		}
		b.recent = newRecent
	}

	// Count identical calls in window
	count := 0
	for _, e := range b.recent {
		if e.name == toolName && e.argsHash == argsHash {
			count++
		}
	}

	// If we've seen this call threshold-1 times already, suppress
	if count >= b.threshold-1 {
		return fmt.Sprintf("Storm breaker: %s was called with identical arguments %d times in a row. This appears to be a repeat loop. Try a different approach.", toolName, count+1)
	}

	// Record this call
	b.recent = append(b.recent, stormEntry{
		name:     toolName,
		argsHash: argsHash,
		mutating: mutating,
	})

	// Trim window
	for len(b.recent) > b.window {
		b.recent = b.recent[1:]
	}

	return ""
}

// Reset clears the storm breaker state. Called at turn boundaries.
func (b *StormBreaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recent = nil
}

// hashArgs computes a short hash of the tool arguments.
func hashArgs(input map[string]any) string {
	data, _ := json.Marshal(input)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:4])
}
