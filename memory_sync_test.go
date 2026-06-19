package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSyncManifest_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := NewSyncManifest(dir)

	m.UpdateState("test.md", SyncState{
		LastSync:    time.Now(),
		LastHash:    "abc123",
		LastVersion: 1,
		SessionID:   "session-1",
	})

	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	// Reload
	m2 := NewSyncManifest(dir)
	state, ok := m2.GetState("test.md")
	if !ok {
		t.Fatal("expected state to be loaded")
	}
	if state.LastHash != "abc123" {
		t.Errorf("expected hash 'abc123', got '%s'", state.LastHash)
	}
	if state.SessionID != "session-1" {
		t.Errorf("expected session 'session-1', got '%s'", state.SessionID)
	}
}

func TestSyncManager_SyncMemory(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.md")
	projectPath := filepath.Join(dir, "project.md")

	// Create initial files
	os.WriteFile(globalPath, []byte("## preference\n- Use Go 1.25\n"), 0644)
	os.WriteFile(projectPath, []byte("## decision\n- Use SQLite\n"), 0644)

	sm := NewSyncManager(dir, "session-1", globalPath, projectPath)

	// First sync should return entries
	entries, err := sm.SyncMemory()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 entries, got %d", len(entries))
	}

	// Second sync should return nothing (no changes)
	entries, err = sm.SyncMemory()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries on second sync, got %d", len(entries))
	}
}

func TestSyncManager_SyncMemory_FileChanged(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.md")

	os.WriteFile(globalPath, []byte("## preference\n- Use Go 1.25\n"), 0644)

	sm := NewSyncManager(dir, "session-1", globalPath, "")

	// First sync
	sm.SyncMemory()

	// Simulate another session modifying the file
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(globalPath, []byte("## preference\n- Use Go 1.25\n- Prefer table-driven tests\n"), 0644)

	// Second sync should detect changes
	entries, err := sm.SyncMemory()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 1 {
		t.Errorf("expected at least 1 entry after change, got %d", len(entries))
	}
}

func TestSyncManager_DetectConflicts(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.md")

	// Simulate another session's changes
	otherState := SyncState{
		LastSync:    time.Now(),
		LastHash:    "other-hash",
		LastVersion: 2,
		SessionID:   "session-2",
	}

	sm := NewSyncManager(dir, "session-1", globalPath, "")
	sm.manifest.UpdateState(globalPath, otherState)

	// Create a file that looks like it was modified by another session
	os.WriteFile(globalPath, []byte("## preference\n- Use Go 1.25\n- New preference from session 2\n"), 0644)

	localEntries := []MemoryEntry{
		{Category: "preference", Content: "Use Go 1.25", Timestamp: time.Now()},
	}

	conflicts := sm.DetectConflicts(localEntries)
	// May or may not find conflicts depending on similarity
	t.Logf("Found %d conflicts", len(conflicts))
}

func TestSyncManager_ExportImport(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.md")

	os.WriteFile(globalPath, []byte("## preference\n- Use Go 1.25\n"), 0644)

	sm := NewSyncManager(dir, "session-1", globalPath, "")

	// Export
	data, err := sm.ExportMemory(ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty export")
	}

	// Import into another directory
	dir2 := t.TempDir()
	globalPath2 := filepath.Join(dir2, "global.md")
	os.MkdirAll(filepath.Dir(globalPath2), 0o755)

	sm2 := NewSyncManager(dir2, "session-2", globalPath2, "")
	count, err := sm2.ImportMemory(data, "merge")
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 imported entry, got %d", count)
	}

	// Verify the file was created
	if _, err := os.Stat(globalPath2); os.IsNotExist(err) {
		t.Error("expected global.md to be created after import")
	}
}

func TestContentHashSync(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"short", "hello"},
		{"long", "this is a longer string for testing hash generation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contentHash(tt.content)
			if result == "" {
				t.Error("expected non-empty hash")
			}
			// Same content should produce same hash
			if contentHash(tt.content) != result {
				t.Error("same content should produce same hash")
			}
		})
	}
}

func TestParseSimpleEntries(t *testing.T) {
	data := `## preference
- Use Go 1.25
<!-- 2024-01-01T00:00:00Z -->
## decision
- Use SQLite
<!-- 2024-01-02T00:00:00Z -->
`

	entries := parseSimpleEntries(data)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Category != "preference" {
		t.Errorf("expected category 'preference', got '%s'", entries[0].Category)
	}
	if entries[0].Content != "Use Go 1.25" {
		t.Errorf("expected content 'Use Go 1.25', got '%s'", entries[0].Content)
	}
}

func TestFormatEntriesAsMarkdown(t *testing.T) {
	entries := []MemoryEntry{
		{Category: "preference", Content: "Use Go 1.25", Timestamp: time.Now()},
		{Category: "decision", Content: "Use SQLite", Timestamp: time.Now()},
	}

	result := formatEntriesAsMarkdown(entries)
	if !strings.Contains(result, "## preference") {
		t.Error("expected '## preference' in output")
	}
	if !strings.Contains(result, "- Use Go 1.25") {
		t.Error("expected '- Use Go 1.25' in output")
	}
}

func TestResolveLastWriteWins(t *testing.T) {
	conflict := SyncConflict{
		Local: []MemoryEntry{
			{Category: "state", Content: "project status", Timestamp: time.Now().Add(-time.Hour)},
		},
		Remote: []MemoryEntry{
			{Category: "state", Content: "project status updated", Timestamp: time.Now()},
		},
	}

	resolved := resolveLastWriteWins(conflict)
	if len(resolved) != 2 {
		t.Errorf("expected 2 resolved entries (different content), got %d", len(resolved))
	}
}

func TestResolveMerge(t *testing.T) {
	conflict := SyncConflict{
		Local: []MemoryEntry{
			{Category: "state", Content: "Local state", Timestamp: time.Now()},
		},
		Remote: []MemoryEntry{
			{Category: "state", Content: "Remote state", Timestamp: time.Now()},
		},
	}

	resolved := resolveMerge(conflict)
	if len(resolved) < 1 {
		t.Errorf("expected at least 1 resolved entry, got %d", len(resolved))
	}
}

func TestSyncManager_StartStop(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.md")
	os.WriteFile(globalPath, []byte("## test\n- entry\n"), 0644)

	sm := NewSyncManager(dir, "session-1", globalPath, "")
	sm.StartSyncLoop(100 * time.Millisecond)

	time.Sleep(250 * time.Millisecond)

	sm.Close()
	// Should not panic or hang
}
