package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileSnapshot stores the content of a file at a specific point in time.
type FileSnapshot struct {
	FilePath  string    `json:"file_path"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// SnapshotHistory maintains an append-only list of snapshots per file,
// enabling undo/rewind capability. It persists each snapshot as a JSON file
// under .claude/snapshots/ relative to the working directory.
type SnapshotHistory struct {
	mu        sync.RWMutex
	snapshots map[string][]FileSnapshot // keyed by absolute file path
	snapDir   string                    // root directory for snapshot storage
}

// NewSnapshotHistory creates a SnapshotHistory that stores data under baseDir/.claude/snapshots/.
func NewSnapshotHistory(baseDir string) *SnapshotHistory {
	snapDir := filepath.Join(baseDir, ".claude", "snapshots")
	return &SnapshotHistory{
		snapshots: make(map[string][]FileSnapshot),
		snapDir:   snapDir,
	}
}

// TakeSnapshot reads the current content of filePath and records a snapshot.
// It should be called before any file edit/write operation so that the
// previous state can be restored later.
func (h *SnapshotHistory) TakeSnapshot(filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("cannot resolve path %s: %w", filePath, err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot read %s: %w", absPath, err)
	}
	// If the file does not exist yet we still record a snapshot with
	// empty content so that RewindTo can delete it later.

	snap := FileSnapshot{
		FilePath:  absPath,
		Content:   string(content),
		Timestamp: time.Now(),
	}

	h.mu.Lock()
	h.snapshots[absPath] = append(h.snapshots[absPath], snap)
	h.mu.Unlock()

	return h.persist(snap)
}

// RewindTo restores filePath to the state captured in snapshot at index.
// Index 0 is the oldest snapshot. After restoring, the snapshot is not
// removed from the history so that multiple rewinds are possible.
func (h *SnapshotHistory) RewindTo(filePath string, index int) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("cannot resolve path %s: %w", filePath, err)
	}

	snaps := h.ListSnapshots(absPath)
	if index < 0 || index >= len(snaps) {
		return fmt.Errorf("snapshot index %d out of range for %s (have %d snapshots)", index, absPath, len(snaps))
	}

	snap := snaps[index]

	if snap.Content == "" {
		// Original snapshot captured a non-existent file; delete it.
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot delete %s: %w", absPath, err)
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return fmt.Errorf("cannot create directory for %s: %w", absPath, err)
	}
	if err := os.WriteFile(absPath, []byte(snap.Content), 0644); err != nil {
		return fmt.Errorf("cannot restore %s to snapshot %d: %w", absPath, index, err)
	}
	return nil
}

// ListSnapshots returns all snapshots for filePath, ordered oldest-first.
// It loads from the in-memory cache first and falls back to disk if needed.
func (h *SnapshotHistory) ListSnapshots(filePath string) []FileSnapshot {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil
	}

	h.mu.RLock()
	snaps := h.snapshots[absPath]
	h.mu.RUnlock()

	if snaps != nil {
		// Return a copy so the caller cannot mutate the slice header.
		out := make([]FileSnapshot, len(snaps))
		copy(out, snaps)
		return out
	}

	// Not in memory; try loading from disk.
	return h.loadFromDisk(absPath)
}

// SnapshotCount returns the number of snapshots for a file.
func (h *SnapshotHistory) SnapshotCount(filePath string) int {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return 0
	}
	return len(h.ListSnapshots(absPath))
}

// Clear removes all in-memory snapshots. Disk files are left untouched.
func (h *SnapshotHistory) Clear() {
	h.mu.Lock()
	h.snapshots = make(map[string][]FileSnapshot)
	h.mu.Unlock()
}

// persist writes a single snapshot to disk as a JSON file.
func (h *SnapshotHistory) persist(snap FileSnapshot) error {
	if h.snapDir == "" {
		return nil
	}
	if err := os.MkdirAll(h.snapDir, 0755); err != nil {
		return err
	}

	// Make the filename safe by replacing path separators with underscores.
	safeName := strings.NewReplacer(
		string(filepath.Separator), "_",
		":", "_",
	).Replace(snap.FilePath)

	ts := snap.Timestamp.Format("20060102T150405")
	filename := fmt.Sprintf("%s_%s.json", ts, safeName)

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(h.snapDir, filename), data, 0644)
}

// loadFromDisk reads all snapshot files for a given absolute path from disk.
func (h *SnapshotHistory) loadFromDisk(absPath string) []FileSnapshot {
	if h.snapDir == "" {
		return nil
	}

	safeName := strings.NewReplacer(
		string(filepath.Separator), "_",
		":", "_",
	).Replace(absPath)

	pattern := filepath.Join(h.snapDir, "*_"+safeName+".json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}

	var snaps []FileSnapshot
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var snap FileSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		snaps = append(snaps, snap)
	}

	// Sort by timestamp so the index is deterministic.
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].Timestamp.Before(snaps[j].Timestamp)
	})

	// Cache the result in memory.
	h.mu.Lock()
	if h.snapshots[absPath] == nil {
		h.snapshots[absPath] = snaps
	}
	h.mu.Unlock()

	return snaps
}
