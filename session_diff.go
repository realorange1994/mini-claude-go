package main

import (
	"fmt"
	"sync"
	"time"
)

// ─── Session Diff/Summary Tracking (MiMo-Code 3B) ─────────────────────────
//
// Tracks file changes per session with git diff computation.
// Enables "what changed in this session" visibility.
//
// MiMo-Code source: session/summary.ts (163 lines)

// SessionDiff tracks file changes in a session.
type SessionDiff struct {
	mu          sync.Mutex
	sessionID   string
	startTime   time.Time
	changes     []FileChange
	totalAdds   int
	totalDels   int
	filesChanged int
}

// FileChange represents a file change in a session.
type FileChange struct {
	Path      string
	Additions int
	Deletions int
	Timestamp time.Time
}

// SessionSummary holds session summary metadata.
type SessionSummary struct {
	SessionID    string `json:"session_id"`
	StartTime    string `json:"start_time"`
	FilesChanged int    `json:"files_changed"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Changes      []FileChange `json:"changes"`
}

// NewSessionDiff creates a new session diff tracker.
func NewSessionDiff(sessionID string) *SessionDiff {
	return &SessionDiff{
		sessionID: sessionID,
		startTime: time.Now(),
		changes:   make([]FileChange, 0),
	}
}

// RecordChange records a file change.
func (d *SessionDiff) RecordChange(path string, additions, deletions int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	change := FileChange{
		Path:      path,
		Additions: additions,
		Deletions: deletions,
		Timestamp: time.Now(),
	}

	d.changes = append(d.changes, change)
	d.totalAdds += additions
	d.totalDels += deletions
	d.filesChanged++
}

// GetSummary returns the session summary.
func (d *SessionDiff) GetSummary() SessionSummary {
	d.mu.Lock()
	defer d.mu.Unlock()

	changes := make([]FileChange, len(d.changes))
	copy(changes, d.changes)

	return SessionSummary{
		SessionID:    d.sessionID,
		StartTime:    d.startTime.Format(time.RFC3339),
		FilesChanged: d.filesChanged,
		Additions:    d.totalAdds,
		Deletions:    d.totalDels,
		Changes:      changes,
	}
}

// FormatDiff formats the session diff as a readable string.
func (d *SessionDiff) FormatDiff() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.changes) == 0 {
		return "No files changed in this session."
	}

	var sb string
	sb += fmt.Sprintf("## Session Changes (%d files)\n\n", d.filesChanged)
	sb += fmt.Sprintf("**+ %d additions, - %d deletions**\n\n", d.totalAdds, d.totalDels)

	for _, change := range d.changes {
		sb += fmt.Sprintf("- `%s`: +%d -%d\n", change.Path, change.Additions, change.Deletions)
	}

	return sb
}

// GetChangesSince returns changes since the given time.
func (d *SessionDiff) GetChangesSince(since time.Time) []FileChange {
	d.mu.Lock()
	defer d.mu.Unlock()

	var result []FileChange
	for _, change := range d.changes {
		if change.Timestamp.After(since) {
			result = append(result, change)
		}
	}
	return result
}

// Clear clears all tracked changes.
func (d *SessionDiff) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.changes = make([]FileChange, 0)
	d.totalAdds = 0
	d.totalDels = 0
	d.filesChanged = 0
}
