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

// ─── Memory Sync System ─────────────────────────────────────────────────────

// SyncState represents the synchronization state of a memory file.
type SyncState struct {
	LastSync    time.Time `json:"last_sync"`
	LastHash    string    `json:"last_hash"`
	LastVersion int       `json:"last_version"`
	SessionID   string    `json:"session_id"`
}

// SyncManifest tracks the sync state of all memory files across sessions.
type SyncManifest struct {
	mu       sync.RWMutex
	entries  map[string]SyncState // key: file path
	filePath string
	dirty    bool
}

// NewSyncManifest creates a new sync manifest.
func NewSyncManifest(projectDir string) *SyncManifest {
	dir := filepath.Join(projectDir, ".claude", "memory")
	os.MkdirAll(dir, 0o755)

	m := &SyncManifest{
		entries:  make(map[string]SyncState),
		filePath: filepath.Join(dir, "sync_manifest.json"),
	}
	m.load()
	return m
}

// load reads the manifest from disk.
func (m *SyncManifest) load() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}
	var entries map[string]SyncState
	if err := json.Unmarshal(data, &entries); err == nil {
		m.entries = entries
	}
}

// Save persists the manifest to disk.
func (m *SyncManifest) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.dirty {
		return nil
	}

	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := m.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, m.filePath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	m.dirty = false
	return nil
}

// GetState returns the sync state for a file.
func (m *SyncManifest) GetState(filePath string) (SyncState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.entries[filePath]
	return state, ok
}

// UpdateState updates the sync state for a file.
func (m *SyncManifest) UpdateState(filePath string, state SyncState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[filePath] = state
	m.dirty = true
}

// ─── Memory Sync Manager ────────────────────────────────────────────────────

// SyncManager handles cross-session memory synchronization.
type SyncManager struct {
	mu          sync.RWMutex
	projectDir  string
	sessionID   string
	manifest    *SyncManifest
	globalPath  string
	projectPath string
	watchPaths  []string
	stopCh      chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
}

// NewSyncManager creates a new sync manager.
func NewSyncManager(projectDir, sessionID, globalPath, projectPath string) *SyncManager {
	sm := &SyncManager{
		projectDir:  projectDir,
		sessionID:   sessionID,
		manifest:    NewSyncManifest(projectDir),
		globalPath:  globalPath,
		projectPath: projectPath,
		stopCh:      make(chan struct{}),
	}

	// Collect paths to watch
	sm.watchPaths = []string{globalPath, projectPath}

	return sm
}

// SyncMemory synchronizes memory from disk, detecting changes from other sessions.
// Returns a list of entries that were updated from other sessions.
func (sm *SyncManager) SyncMemory() ([]MemoryEntry, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var updated []MemoryEntry

	// Sync global memory
	if sm.globalPath != "" {
		entries, err := sm.syncFile(sm.globalPath, ScopeGlobal)
		if err != nil {
			return nil, fmt.Errorf("sync global: %w", err)
		}
		updated = append(updated, entries...)
	}

	// Sync project memory
	if sm.projectPath != "" {
		entries, err := sm.syncFile(sm.projectPath, ScopeProject)
		if err != nil {
			return nil, fmt.Errorf("sync project: %w", err)
		}
		updated = append(updated, entries...)
	}

	return updated, nil
}

// syncFile synchronizes a single memory file.
func (sm *SyncManager) syncFile(filePath string, scope MemoryScope) ([]MemoryEntry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Check if file changed since last sync
	currentHash := contentHash(string(data))
	lastState, hasState := sm.manifest.GetState(filePath)

	if hasState && lastState.LastHash == currentHash {
		return nil, nil // No changes
	}

	// Parse entries from file
	entries := parseSimpleEntries(string(data))

	// Update manifest
	sm.manifest.UpdateState(filePath, SyncState{
		LastSync:    time.Now(),
		LastHash:    currentHash,
		LastVersion: lastState.LastVersion + 1,
		SessionID:   sm.sessionID,
	})

	return entries, nil
}

// DetectConflicts checks if any memory entries conflict with other sessions.
func (sm *SyncManager) DetectConflicts(currentEntries []MemoryEntry) []SyncConflict {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var conflicts []SyncConflict

	for _, filePath := range sm.watchPaths {
		state, ok := sm.manifest.GetState(filePath)
		if !ok {
			continue
		}

		// Check if another session modified the file
		if state.SessionID != sm.sessionID && time.Since(state.LastSync) < 5*time.Minute {
			// Read the file to see what changed
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			otherEntries := parseSimpleEntries(string(data))
			conflict := findConflicts(currentEntries, otherEntries, filePath)
			if conflict != nil {
				conflicts = append(conflicts, *conflict)
			}
		}
	}

	return conflicts
}

// SyncConflict represents a conflict between two sessions.
type SyncConflict struct {
	FilePath   string        `json:"file_path"`
	Scope      MemoryScope   `json:"scope"`
	Local      []MemoryEntry `json:"local"`
	Remote     []MemoryEntry `json:"remote"`
	Resolution string        `json:"resolution"`
}

// ResolveConflicts applies a resolution strategy to conflicts.
func (sm *SyncManager) ResolveConflicts(conflicts []SyncConflict, strategy string) []MemoryEntry {
	var resolved []MemoryEntry

	for _, conflict := range conflicts {
		switch strategy {
		case "last-write-wins":
			// Keep the most recently modified entries
			resolved = append(resolved, resolveLastWriteWins(conflict)...)
		case "merge":
			// Merge all entries, deduplicating
			resolved = append(resolved, resolveMerge(conflict)...)
		case "local":
			// Keep only local entries
			resolved = append(resolved, conflict.Local...)
		case "remote":
			// Keep only remote entries
			resolved = append(resolved, conflict.Remote...)
		default:
			// Default to merge
			resolved = append(resolved, resolveMerge(conflict)...)
		}
	}

	return resolved
}

// resolveLastWriteWins keeps the most recently modified entries.
func resolveLastWriteWins(conflict SyncConflict) []MemoryEntry {
	// Group by category+content, keep newest
	seen := make(map[string]MemoryEntry)
	for _, e := range conflict.Local {
		key := e.Category + ":" + e.Content
		existing, ok := seen[key]
		if !ok || e.Timestamp.After(existing.Timestamp) {
			seen[key] = e
		}
	}
	for _, e := range conflict.Remote {
		key := e.Category + ":" + e.Content
		existing, ok := seen[key]
		if !ok || e.Timestamp.After(existing.Timestamp) {
			seen[key] = e
		}
	}

	result := make([]MemoryEntry, 0, len(seen))
	for _, e := range seen {
		result = append(result, e)
	}
	return result
}

// resolveMerge merges all entries, deduplicating by content similarity.
func resolveMerge(conflict SyncConflict) []MemoryEntry {
	all := append(conflict.Local, conflict.Remote...)
	merged, _ := DeduplicateEntries(all)
	return merged
}

// findConflicts finds conflicting entries between local and remote.
func findConflicts(local, remote []MemoryEntry, filePath string) *SyncConflict {
	// Find entries that exist in both but differ
	var conflictLocal, conflictRemote []MemoryEntry

	for _, l := range local {
		for _, r := range remote {
			if l.Category == r.Category && l.Content != r.Content {
				similarity := ContentSimilarity(l.Content, r.Content)
				if similarity > 0.5 && similarity < SimilarityThreshold {
					conflictLocal = append(conflictLocal, l)
					conflictRemote = append(conflictRemote, r)
				}
			}
		}
	}

	if len(conflictLocal) == 0 {
		return nil
	}

	return &SyncConflict{
		FilePath: filePath,
		Local:    conflictLocal,
		Remote:   conflictRemote,
	}
}

// ExportMemory exports memory entries to a shareable format.
func (sm *SyncManager) ExportMemory(scope MemoryScope) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var filePath string
	switch scope {
	case ScopeGlobal:
		filePath = sm.globalPath
	case ScopeProject:
		filePath = sm.projectPath
	default:
		return nil, fmt.Errorf("cannot export session scope")
	}

	if filePath == "" {
		return nil, fmt.Errorf("no path for scope %s", scope)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	export := MemoryExport{
		Scope:     scope,
		Content:   string(data),
		ExportedAt: time.Now(),
		SessionID: sm.sessionID,
		Version:   1,
	}

	return json.MarshalIndent(export, "", "  ")
}

// ImportMemory imports memory entries from a shareable format.
func (sm *SyncManager) ImportMemory(data []byte, strategy string) (int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var export MemoryExport
	if err := json.Unmarshal(data, &export); err != nil {
		return 0, fmt.Errorf("invalid export format: %w", err)
	}

	// Parse imported entries
	importedEntries := parseSimpleEntries(export.Content)

	// Read current entries
	var currentEntries []MemoryEntry
	var filePath string
	switch export.Scope {
	case ScopeGlobal:
		filePath = sm.globalPath
	case ScopeProject:
		filePath = sm.projectPath
	default:
		return 0, fmt.Errorf("cannot import session scope")
	}

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err == nil {
			currentEntries = parseSimpleEntries(string(data))
		}
	}

	// Merge with strategy
	conflict := SyncConflict{
		FilePath: filePath,
		Local:    currentEntries,
		Remote:   importedEntries,
	}
	merged := sm.ResolveConflicts([]SyncConflict{conflict}, strategy)

	// Write merged entries
	if filePath != "" {
		content := formatEntriesAsMarkdown(merged)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return 0, err
		}
	}

	return len(merged), nil
}

// MemoryExport represents a shareable memory export.
type MemoryExport struct {
	Scope      MemoryScope `json:"scope"`
	Content    string      `json:"content"`
	ExportedAt time.Time   `json:"exported_at"`
	SessionID  string      `json:"session_id"`
	Version    int         `json:"version"`
}

// ─── Helper Functions ────────────────────────────────────────────────────────

// contentHash returns a simple hash of content for change detection.
func contentHash(content string) string {
	if len(content) == 0 {
		return "empty"
	}
	// Use length + first/last 16 chars as a simple hash
	prefix := content
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	suffix := content
	if len(suffix) > 16 {
		suffix = suffix[len(suffix)-16:]
	}
	return fmt.Sprintf("%d:%s:%s", len(content), prefix, suffix)
}

// parseSimpleEntries parses entries from a simple markdown format.
func parseSimpleEntries(data string) []MemoryEntry {
	var entries []MemoryEntry
	lines := strings.Split(data, "\n")
	var currentCategory string
	lastTimestamp := time.Now()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Category header: ## Category
		if strings.HasPrefix(trimmed, "## ") {
			currentCategory = strings.TrimSpace(trimmed[3:])
			continue
		}

		// Timestamp comment: <!-- timestamp -->
		if strings.HasPrefix(trimmed, "<!-- ") && strings.HasSuffix(trimmed, " -->") {
			ts := strings.TrimPrefix(trimmed, "<!-- ")
			ts = strings.TrimSuffix(ts, " -->")
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				lastTimestamp = t
			}
			continue
		}

		// Bullet point: - content
		if strings.HasPrefix(trimmed, "- ") && currentCategory != "" {
			content := strings.TrimSpace(trimmed[2:])
			if content == "" {
				continue
			}
			entries = append(entries, MemoryEntry{
				Category:  currentCategory,
				Content:   content,
				Timestamp: lastTimestamp,
				Source:    "sync",
			})
		}
	}

	return entries
}

// formatEntriesAsMarkdown formats entries as a simple markdown file.
func formatEntriesAsMarkdown(entries []MemoryEntry) string {
	// Group by category
	groups := make(map[string][]MemoryEntry)
	var categories []string
	for _, e := range entries {
		if _, ok := groups[e.Category]; !ok {
			categories = append(categories, e.Category)
		}
		groups[e.Category] = append(groups[e.Category], e)
	}
	sort.Strings(categories)

	var sb strings.Builder
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("## %s\n", cat))
		for _, e := range groups[cat] {
			sb.WriteString(fmt.Sprintf("- %s\n", e.Content))
			sb.WriteString(fmt.Sprintf("<!-- %s -->\n", e.Timestamp.Format(time.RFC3339)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// StartSyncLoop starts a background goroutine that periodically syncs memory.
func (sm *SyncManager) StartSyncLoop(interval time.Duration) {
	sm.wg.Add(1)
	go func() {
		defer sm.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-sm.stopCh:
				return
			case <-ticker.C:
				sm.SyncMemory()
				sm.manifest.Save()
			}
		}
	}()
}

// Close stops the sync manager.
func (sm *SyncManager) Close() {
	sm.stopOnce.Do(func() {
		close(sm.stopCh)
	})
	sm.wg.Wait()
	sm.manifest.Save()
}
