package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileSnapshot stores the content of a file at a specific point in time.
type FileSnapshot struct {
	FilePath    string    `json:"file_path"`
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description,omitempty"` // e.g. "before edit_file", "after write_file"
	Checksum    string    `json:"checksum"`              // FNV-1a hash of content for dedup
	Deleted     bool      `json:"deleted,omitempty"`     // tombstone: file was deleted
}

// SnapshotHistory maintains an append-only list of snapshots per file,
// enabling undo/rewind capability. It persists each snapshot as a JSON file
// under .claude/snapshots/ relative to the working directory.
type SnapshotHistory struct {
	mu           sync.RWMutex
	snapshots    map[string][]FileSnapshot // keyed by absolute file path
	snapDir      string                    // root directory for snapshot storage
	maxSnapshots int                       // max snapshots per file (default 50)
	maxAge       time.Duration             // max age before trimming (default 7 days)
}

const (
	defaultMaxSnapshots = 50
	defaultMaxAge       = 7 * 24 * time.Hour
	defaultMinKeep      = 5
)

// NewSnapshotHistory creates a SnapshotHistory that stores data under baseDir/.claude/snapshots/.
func NewSnapshotHistory(baseDir string) *SnapshotHistory {
	snapDir := filepath.Join(baseDir, ".claude", "snapshots")
	return &SnapshotHistory{
		snapshots:    make(map[string][]FileSnapshot),
		snapDir:      snapDir,
		maxSnapshots: defaultMaxSnapshots,
		maxAge:       defaultMaxAge,
	}
}

// computeChecksum returns an FNV-1a hash of the content.
func computeChecksum(content string) string {
	h := fnv.New128a()
	h.Write([]byte(content))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TakeSnapshot reads the current content of filePath and records a snapshot.
// It should be called before any file edit/write operation so that the
// previous state can be restored later.
func (h *SnapshotHistory) TakeSnapshot(filePath string) error {
	return h.TakeSnapshotWithDesc(filePath, "")
}

// TakeSnapshotWithDesc reads the current content of filePath and records a snapshot
// with an optional description (e.g. "before edit_file", "after write_file").
// If the content is identical to the last snapshot (same checksum), it is skipped.
func (h *SnapshotHistory) TakeSnapshotWithDesc(filePath, description string) error {
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

	checksum := computeChecksum(string(content))

	// Dedup: skip if content is identical to the last snapshot
	h.mu.RLock()
	existing := h.snapshots[absPath]
	h.mu.RUnlock()
	if len(existing) > 0 {
		last := existing[len(existing)-1]
		if last.Checksum == checksum && !last.Deleted {
			// Content unchanged, skip duplicate snapshot
			return nil
		}
	}

	snap := FileSnapshot{
		FilePath:    absPath,
		Content:     string(content),
		Timestamp:   time.Now(),
		Description: description,
		Checksum:    checksum,
	}

	h.mu.Lock()
	h.snapshots[absPath] = append(h.snapshots[absPath], snap)
	h.mu.Unlock()

	// Trim if over capacity
	h.trimSnapshots(absPath)

	return h.persist(snap)
}

// trimSnapshots removes old snapshots when over capacity limits.
// It keeps at most maxSnapshots per file, and removes snapshots older than
// maxAge while keeping at least minKeep snapshots.
func (h *SnapshotHistory) trimSnapshots(absPath string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	snaps := h.snapshots[absPath]
	if len(snaps) <= h.maxSnapshots {
		return
	}

	// Keep the most recent maxSnapshots
	keep := snaps[len(snaps)-h.maxSnapshots:]
	h.snapshots[absPath] = keep

	// Also trim by age, but keep at least minKeep
	if h.maxAge > 0 && len(h.snapshots[absPath]) > defaultMinKeep {
		cutoff := time.Now().Add(-h.maxAge)
		firstToKeep := 0
		for i, s := range h.snapshots[absPath] {
			if s.Timestamp.After(cutoff) {
				firstToKeep = i
				break
			}
			if i >= len(h.snapshots[absPath])-defaultMinKeep {
				break
			}
			firstToKeep = i + 1
		}
		if firstToKeep > 0 && len(h.snapshots[absPath])-firstToKeep >= defaultMinKeep {
			h.snapshots[absPath] = h.snapshots[absPath][firstToKeep:]
		}
	}
}

// RewindTo restores filePath to the state captured in snapshot at index.
// Index 0 is the oldest snapshot. Before restoring, the current file state
// is snapshotted so that redo is possible.
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

	// Snapshot current state before restoring so redo is possible
	if currentContent, err := os.ReadFile(absPath); err == nil && len(currentContent) > 0 {
		currentSnap := FileSnapshot{
			FilePath:    absPath,
			Content:     string(currentContent),
			Timestamp:   time.Now(),
			Checksum:    computeChecksum(string(currentContent)),
			Description: fmt.Sprintf("restore: to v%d", index+1),
		}
		h.mu.Lock()
		h.snapshots[absPath] = append(h.snapshots[absPath], currentSnap)
		h.mu.Unlock()
		_ = h.persist(currentSnap)
	}

	if snap.Content == "" || snap.Deleted {
		// Original snapshot captured a non-existent/deleted file; delete it.
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

// restoreInternal restores a file by a number of distinct-content steps.
// It collapses snapshots by unique checksum (skipping tombstones) before
// calculating how many steps back to go. Before restoring, the current
// file state is snapshotted so that redo is possible.
func (h *SnapshotHistory) restoreInternal(filePath string, steps int) (string, error) {
	if steps < 1 {
		return "", fmt.Errorf("steps must be at least 1")
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %s: %w", filePath, err)
	}

	h.mu.RLock()
	snaps := h.snapshots[absPath]
	h.mu.RUnlock()

	if len(snaps) == 0 {
		// Try loading from disk
		snaps = h.loadFromDisk(absPath)
	}

	// Build distinct-content states (skip tombstones, dedup by checksum)
	type distinctState struct {
		idx      int
		checksum string
	}
	var distinct []distinctState
	seen := make(map[string]bool)
	for i, s := range snaps {
		if s.Deleted {
			continue
		}
		if !seen[s.Checksum] {
			seen[s.Checksum] = true
			distinct = append(distinct, distinctState{idx: i, checksum: s.Checksum})
		}
	}

	if len(distinct) < 2 {
		return "", fmt.Errorf("no previous version available for %s (need at least 2 distinct states, have %d)", absPath, len(distinct))
	}

	targetIdx := len(distinct) - 1 - steps
	if targetIdx < 0 {
		return "", fmt.Errorf("cannot rewind %d step(s) for %s (only %d distinct states)", steps, absPath, len(distinct))
	}

	targetSnap := snaps[distinct[targetIdx].idx]
	targetVersion := distinct[targetIdx].idx + 1

	// Snapshot current state before restoring so redo is possible
	if currentContent, err := os.ReadFile(absPath); err == nil && len(currentContent) > 0 {
		currentSnap := FileSnapshot{
			FilePath:    absPath,
			Content:     string(currentContent),
			Timestamp:   time.Now(),
			Checksum:    computeChecksum(string(currentContent)),
			Description: fmt.Sprintf("restore: to v%d", targetVersion),
		}
		h.mu.Lock()
		h.snapshots[absPath] = append(h.snapshots[absPath], currentSnap)
		h.mu.Unlock()
		_ = h.persist(currentSnap)
	}

	if targetSnap.Content == "" || targetSnap.Deleted {
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("cannot delete %s: %w", absPath, err)
		}
		return "(file deleted — target version was empty)", nil
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", fmt.Errorf("cannot create directory for %s: %w", absPath, err)
	}
	if err := os.WriteFile(absPath, []byte(targetSnap.Content), 0644); err != nil {
		return "", fmt.Errorf("cannot restore %s: %w", absPath, err)
	}
	return targetSnap.Content, nil
}

// RestoreLast restores a file to its previous distinct-content version.
// Before restoring, the current file state is snapshotted so that redo is possible.
// Returns the restored content or an error.
func (h *SnapshotHistory) RestoreLast(filePath string) (string, error) {
	return h.restoreInternal(filePath, 1)
}

// RewindSteps restores a file to the version that is 'steps' distinct-content states back.
// Before rewinding, the current file state is snapshotted so that redo is possible.
// Returns the restored content or an error.
func (h *SnapshotHistory) RewindSteps(filePath string, steps int) (string, error) {
	return h.restoreInternal(filePath, steps)
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
	snaps := h.ListSnapshots(absPath)
	count := 0
	for _, s := range snaps {
		if !s.Deleted {
			count++
		}
	}
	return count
}

// Clear removes all in-memory snapshots. Disk files are left untouched.
func (h *SnapshotHistory) Clear() {
	h.mu.Lock()
	h.snapshots = make(map[string][]FileSnapshot)
	h.mu.Unlock()
}

// ClearPath removes all snapshots for a specific file (in-memory + disk).
func (h *SnapshotHistory) ClearPath(filePath string) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return
	}

	h.mu.Lock()
	delete(h.snapshots, absPath)
	h.mu.Unlock()

	// Remove disk files
	h.removeDiskSnapshots(absPath)
}

// ClearUnderDir removes all snapshots for files under a directory.
func (h *SnapshotHistory) ClearUnderDir(dir string) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	if !strings.HasSuffix(absDir, string(filepath.Separator)) {
		absDir += string(filepath.Separator)
	}

	h.mu.Lock()
	var toDelete []string
	for path := range h.snapshots {
		if strings.HasPrefix(path, absDir) {
			toDelete = append(toDelete, path)
		}
	}
	for _, p := range toDelete {
		delete(h.snapshots, p)
	}
	h.mu.Unlock()

	for _, p := range toDelete {
		h.removeDiskSnapshots(p)
	}
}

// removeDiskSnapshots removes all snapshot files for a given absolute path from disk.
func (h *SnapshotHistory) removeDiskSnapshots(absPath string) {
	if h.snapDir == "" {
		return
	}

	safeName := strings.NewReplacer(
		string(filepath.Separator), "_",
		":", "_",
	).Replace(absPath)

	pattern := filepath.Join(h.snapDir, "*_"+safeName+".json")
	matches, _ := filepath.Glob(pattern)
	for _, f := range matches {
		os.Remove(f)
	}
}

// ListAllFiles returns all file paths that have snapshot history.
// It checks in-memory first, then falls back to scanning disk snapshots.
func (h *SnapshotHistory) ListAllFiles() []string {
	h.mu.RLock()
	inMemory := len(h.snapshots)
	h.mu.RUnlock()

	if inMemory > 0 {
		h.mu.RLock()
		defer h.mu.RUnlock()
		files := make([]string, 0, len(h.snapshots))
		for path := range h.snapshots {
			files = append(files, path)
		}
		sort.Strings(files)
		return files
	}

	// In-memory is empty; scan disk to discover all files with snapshots.
	return h.loadAllFromDisk()
}

// ResolveVersion resolves a version specifier to a 1-indexed version number.
// Supports: "v3"/"3" (absolute), "current"/"latest" (last), "last2" (2 back), tag name.
func (h *SnapshotHistory) ResolveVersion(filePath, spec string) (int, error) {
	if spec == "" {
		spec = "current"
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return 0, fmt.Errorf("cannot resolve path %s: %w", filePath, err)
	}

	snaps := h.ListSnapshots(absPath)
	if len(snaps) == 0 {
		return 0, fmt.Errorf("no snapshots for %s", absPath)
	}

	// Build active (non-deleted) view — this is what users see as "v1, v2, v3..."
	var active []FileSnapshot
	for _, s := range snaps {
		if !s.Deleted {
			active = append(active, s)
		}
	}
	total := len(active)
	if total == 0 {
		return 0, fmt.Errorf("no active snapshots for %s", absPath)
	}

	spec = strings.TrimSpace(spec)

	// "current" / "latest"
	if spec == "current" || spec == "latest" {
		return total, nil
	}

	// "v3" or "3" — absolute version number
	if strings.HasPrefix(spec, "v") {
		spec = spec[1:]
	}
	if v, err := fmt.Sscanf(spec, "%d", new(int)); v == 1 && err == nil {
		var n int
		fmt.Sscanf(spec, "%d", &n)
		if n < 1 || n > total {
			return 0, fmt.Errorf("version %d out of range (1-%d)", n, total)
		}
		return n, nil
	}

	// "lastN" — N versions back from current
	if strings.HasPrefix(spec, "last") {
		var n int
		if _, err := fmt.Sscanf(spec[4:], "%d", &n); err == nil && n >= 1 {
			if n > 0 && n < total {
				return total - n, nil
			}
			return 0, fmt.Errorf("last%d is out of range (only %d versions)", n, total)
		}
	}

	// Tag name — search descriptions in active snapshots only
	tag := "[" + spec + "]"
	activeVersion := 0
	for _, s := range snaps {
		if !s.Deleted {
			activeVersion++
		}
		if strings.Contains(s.Description, tag) && !s.Deleted {
			return activeVersion, nil
		}
	}

	return 0, fmt.Errorf("cannot resolve version specifier %q for %s", spec, absPath)
}

// AddTag adds a named tag to the latest non-deleted snapshot for a file.
// Tags are stored as [tagname] appended to the description.
// Returns true if a snapshot was tagged.
func (h *SnapshotHistory) AddTag(filePath, tag string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	snaps := h.snapshots[absPath]
	if len(snaps) == 0 {
		return false
	}

	// Find last non-deleted snapshot
	idx := -1
	for i := len(snaps) - 1; i >= 0; i-- {
		if !snaps[i].Deleted {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}

	// Append tag to description
	tagStr := "[" + tag + "]"
	if !strings.Contains(snaps[idx].Description, tagStr) {
		snaps[idx].Description += " " + tagStr
		snaps[idx].Description = strings.TrimSpace(snaps[idx].Description)
		_ = h.persist(snaps[idx])
	}

	return true
}

// TagEntry represents a tagged version.
type TagEntry struct {
	Version int
	Tag     string
}

// ListTags returns all tags for a file's snapshots.
func (h *SnapshotHistory) ListTags(filePath string) []TagEntry {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil
	}

	snaps := h.ListSnapshots(absPath)
	re := regexp.MustCompile(`\[([^\]]+)\]`)

	// Build active view for version numbering
	activeIdx := make(map[int]int) // snaps index -> active index (1-based)
	activeCount := 0
	for i, s := range snaps {
		if !s.Deleted {
			activeCount++
			activeIdx[i] = activeCount
		}
	}

	var tags []TagEntry
	for i, s := range snaps {
		if s.Deleted {
			continue
		}
		matches := re.FindAllStringSubmatch(s.Description, -1)
		for _, m := range matches {
			if len(m) > 1 {
				tags = append(tags, TagEntry{Version: activeIdx[i], Tag: m[1]})
			}
		}
	}
	return tags
}

// TimelineEntry represents a single entry in the cross-file timeline.
type TimelineEntry struct {
	Timestamp   time.Time
	FilePath    string
	Version     int
	Description string
}

// GetTimeline returns all snapshots across all files, sorted chronologically.
// If since is non-zero, only entries after that time are included.
func (h *SnapshotHistory) GetTimeline(since time.Time) []TimelineEntry {
	files := h.ListAllFiles()
	var entries []TimelineEntry

	for _, fp := range files {
		snaps := h.ListSnapshots(fp)
		for i, s := range snaps {
			if !since.IsZero() && s.Timestamp.Before(since) {
				continue
			}
			entries = append(entries, TimelineEntry{
				Timestamp:   s.Timestamp,
				FilePath:    fp,
				Version:     i + 1,
				Description: s.Description,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries
}

// SearchMode defines what kind of changes to search for.
type SearchMode int

const (
	SearchAdded   SearchMode = iota // lines that were added
	SearchRemoved                    // lines that were removed
	SearchChanged                    // lines that were added or removed
)

// HistorySearchResult represents a search match in a specific version.
type HistorySearchResult struct {
	Version int
	Lines   []string
}

// Search finds versions where text was added, removed, or changed.
// It compares consecutive snapshots using line set difference.
func (h *SnapshotHistory) Search(filePath, pattern string, mode SearchMode, ignoreCase bool) []HistorySearchResult {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil
	}

	snaps := h.ListSnapshots(absPath)
	if len(snaps) < 2 {
		return nil
	}

	rePattern := pattern
	if ignoreCase {
		rePattern = "(?i)" + rePattern
	}
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return nil
	}

	var results []HistorySearchResult

	for i := 1; i < len(snaps); i++ {
		prevLines := strings.Split(snaps[i-1].Content, "\n")
		currLines := strings.Split(snaps[i].Content, "\n")

		prevSet := make(map[string]bool)
		currSet := make(map[string]bool)
		for _, l := range prevLines {
			key := l
			if ignoreCase {
				key = strings.ToLower(l)
			}
			prevSet[key] = true
		}
		for _, l := range currLines {
			key := l
			if ignoreCase {
				key = strings.ToLower(l)
			}
			currSet[key] = true
		}

		var changedLines []string

		if mode == SearchAdded || mode == SearchChanged {
			for _, l := range currLines {
				key := l
				if ignoreCase {
					key = strings.ToLower(l)
				}
				if !prevSet[key] {
					changedLines = append(changedLines, "+"+l)
				}
			}
		}

		if mode == SearchRemoved || mode == SearchChanged {
			for _, l := range prevLines {
				key := l
				if ignoreCase {
					key = strings.ToLower(l)
				}
				if !currSet[key] {
					changedLines = append(changedLines, "-"+l)
				}
			}
		}

		// Filter by pattern
		var matched []string
		for _, l := range changedLines {
			lineContent := l[1:] // strip +/- prefix
			if re.MatchString(lineContent) {
				matched = append(matched, l)
			}
		}

		if len(matched) > 0 {
			results = append(results, HistorySearchResult{
				Version: i + 1,
				Lines:   matched,
			})
		}
	}

	return results
}

// loadAllFromDisk scans the snapshots directory and returns all unique file paths
// that have at least one snapshot on disk.
func (h *SnapshotHistory) loadAllFromDisk() []string {
	if h.snapDir == "" {
		return nil
	}

	matches, err := filepath.Glob(filepath.Join(h.snapDir, "*.json"))
	if err != nil || len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var snap FileSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		if snap.FilePath != "" {
			seen[snap.FilePath] = true
		}
	}

	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
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

	// Backfill empty descriptions and checksums for legacy snapshots
	for i := range snaps {
		if snaps[i].Checksum == "" {
			snaps[i].Checksum = computeChecksum(snaps[i].Content)
		}
		if snaps[i].Description == "" {
			if i == 0 && snaps[i].Content == "" {
				snaps[i].Description = "empty (pre-modification snapshot)"
			} else if i == 0 {
				snaps[i].Description = fmt.Sprintf("initial (%d bytes)", len(snaps[i].Content))
			} else {
				snaps[i].Description = fmt.Sprintf("v%d", i+1)
			}
		}
	}

	// Cache the result in memory.
	h.mu.Lock()
	if h.snapshots[absPath] == nil {
		h.snapshots[absPath] = snaps
	}
	h.mu.Unlock()

	return snaps
}
