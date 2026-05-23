package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ============================================================================
// Config constants tests
// Ported from upstream: src/utils/__tests__/configConstants.test.ts
// ============================================================================

func TestNotificationChannelsContains(t *testing.T) {
	required := []string{
		"auto",
		"iterm2",
		"iterm2_with_bell",
		"terminal_bell",
		"kitty",
		"ghostty",
		"notifications_disabled",
	}
	for _, r := range required {
		t.Run(r, func(t *testing.T) {
			if !sliceContains(NotificationChannels, r) {
				t.Errorf("NotificationChannels missing %q", r)
			}
		})
	}
}

func TestEditorModesContains(t *testing.T) {
	required := []string{
		"normal",
		"vim",
	}
	for _, r := range required {
		t.Run(r, func(t *testing.T) {
			if !sliceContains(EditorModes, r) {
				t.Errorf("EditorModes missing %q", r)
			}
		})
	}
}

func TestTeammateModesContains(t *testing.T) {
	required := []string{
		"auto",
		"tmux",
		"in-process",
	}
	for _, r := range required {
		t.Run(r, func(t *testing.T) {
			if !sliceContains(TeammateModes, r) {
				t.Errorf("TeammateModes missing %q", r)
			}
		})
	}
}

func TestNotificationChannelsNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, ch := range NotificationChannels {
		if seen[ch] {
			t.Errorf("duplicate entry in NotificationChannels: %q", ch)
		}
		seen[ch] = true
	}
}

func TestEditorModesNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range EditorModes {
		if seen[m] {
			t.Errorf("duplicate entry in EditorModes: %q", m)
		}
		seen[m] = true
	}
}

func TestTeammateModesNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range TeammateModes {
		if seen[m] {
			t.Errorf("duplicate entry in TeammateModes: %q", m)
		}
		seen[m] = true
	}
}

// -- Upstream Quality: Exact-value and ordering tests

func TestEditorModesExactLength(t *testing.T) {
	// From upstream: "has exactly 2 entries"
	if len(EditorModes) != 2 {
		t.Fatalf("expected EditorModes to have exactly 2 entries, got %d", len(EditorModes))
	}
}

func TestEditorModesOrdering(t *testing.T) {
	// From upstream: "is ordered: normal, vim"
	if len(EditorModes) >= 2 {
		if EditorModes[0] != "normal" {
			t.Errorf("EditorModes[0]: expected 'normal', got %q", EditorModes[0])
		}
		if EditorModes[1] != "vim" {
			t.Errorf("EditorModes[1]: expected 'vim', got %q", EditorModes[1])
		}
	}
}

func TestTeammateModesExactLength(t *testing.T) {
	// From upstream: "has exactly 3 entries"
	if len(TeammateModes) != 3 {
		t.Fatalf("expected TeammateModes to have exactly 3 entries, got %d", len(TeammateModes))
	}
}

func TestTeammateModesOrdering(t *testing.T) {
	// From upstream: "is ordered: auto, tmux, in-process"
	if len(TeammateModes) >= 3 {
		if TeammateModes[0] != "auto" {
			t.Errorf("TeammateModes[0]: expected 'auto', got %q", TeammateModes[0])
		}
		if TeammateModes[1] != "tmux" {
			t.Errorf("TeammateModes[1]: expected 'tmux', got %q", TeammateModes[1])
		}
		if TeammateModes[2] != "in-process" {
			t.Errorf("TeammateModes[2]: expected 'in-process', got %q", TeammateModes[2])
		}
	}
}

func TestNotificationChannelsExactLength(t *testing.T) {
	// From upstream: length assertion pattern
	expected := []string{
		"auto",
		"iterm2",
		"iterm2_with_bell",
		"terminal_bell",
		"kitty",
		"ghostty",
		"notifications_disabled",
	}
	if len(NotificationChannels) != len(expected) {
		t.Fatalf("expected NotificationChannels to have %d entries, got %d", len(expected), len(NotificationChannels))
	}
}

func TestNotificationChannelsExactValues(t *testing.T) {
	// From upstream: exact value array match
	expected := []string{
		"auto",
		"iterm2",
		"iterm2_with_bell",
		"terminal_bell",
		"kitty",
		"ghostty",
		"notifications_disabled",
	}
	for i, ch := range NotificationChannels {
		if ch != expected[i] {
			t.Errorf("NotificationChannels[%d]: expected %q, got %q", i, expected[i], ch)
		}
	}
}

// ============================================================================
// Feature flags tests
// Regression guard for flag persistence and CRUD
// ============================================================================

func TestNewFeatureFlagStoreNoFile(t *testing.T) {
	// Create store when no file exists -- should start empty
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	flags := store.List()
	if len(flags) != 0 {
		t.Error("new store without file should have no flags")
	}
}

func TestFeatureFlagStoreEnabledDefault(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	// Unknown flag should return false
	if store.Enabled("nonexistent") {
		t.Error("unknown flag should default to false")
	}
}

func TestFeatureFlagStoreEnable(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	store.Enable("test_flag", "A test feature")

	if !store.Enabled("test_flag") {
		t.Error("enabled flag should return true")
	}

	// Check persistence
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("expected flag file to exist, got error: %v", err)
	}
	var saved map[string]FeatureFlag
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("failed to parse saved flags: %v", err)
	}
	if f, ok := saved["test_flag"]; !ok || !f.Enabled {
		t.Error("enabled flag should be persisted")
	}
	if f, ok := saved["test_flag"]; ok && f.Description != "A test feature" {
		t.Errorf("expected description 'A test feature', got %q", f.Description)
	}
}

func TestFeatureFlagStoreDisable(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	store.Enable("test_flag", "A test feature")
	if !store.Enabled("test_flag") {
		t.Fatal("flag should be enabled before disable")
	}

	store.Disable("test_flag")
	if store.Enabled("test_flag") {
		t.Error("disabled flag should return false")
	}

	// Flag should still exist in store (just disabled)
	flags := store.List()
	found := false
	for _, f := range flags {
		if f.Name == "test_flag" {
			found = true
			if f.Enabled {
				t.Error("flag in list should show as disabled")
			}
		}
	}
	if !found {
		t.Error("disabled flag should still appear in list")
	}
}

func TestFeatureFlagStoreDisableNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	// Disabling nonexistent flag should not crash or create it
	store.Disable("nonexistent")
	if store.Enabled("nonexistent") {
		t.Error("nonexistent flag should remain false after disable")
	}
}

func TestFeatureFlagStoreList(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	store.Enable("flag_a", "First flag")
	store.Enable("flag_b", "Second flag")
	store.Enable("flag_c", "Third flag")

	flags := store.List()
	if len(flags) != 3 {
		t.Errorf("expected 3 flags, got %d", len(flags))
	}
}

func TestFeatureFlagStoreLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	// Pre-create the file
	initialFlags := map[string]FeatureFlag{
		"existing_flag": {Name: "existing_flag", Enabled: true, Description: "pre-existing"},
	}
	data, _ := json.MarshalIndent(initialFlags, "", "  ")
	os.WriteFile(file, data, 0o644)

	// Create store that loads from the file
	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}
	rawData, _ := os.ReadFile(file)
	json.Unmarshal(rawData, &store.flags)
	for name, f := range store.flags {
		f.Name = name
		store.flags[name] = f
	}

	if !store.Enabled("existing_flag") {
		t.Error("should load pre-existing enabled flag from file")
	}
}

func TestFeatureFlagStoreEnableDisableCycle(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	// Enable -> Disable -> Enable cycle
	store.Enable("cycle_flag", "test")
	if !store.Enabled("cycle_flag") {
		t.Fatal("should be enabled after first Enable")
	}

	store.Disable("cycle_flag")
	if store.Enabled("cycle_flag") {
		t.Fatal("should be disabled after Disable")
	}

	store.Enable("cycle_flag", "test again")
	if !store.Enabled("cycle_flag") {
		t.Fatal("should be enabled after second Enable")
	}
}

func TestFeatureFlagStruct(t *testing.T) {
	f := FeatureFlag{
		Name:        "test",
		Enabled:     true,
		Description: "desc",
	}
	if f.Name != "test" {
		t.Errorf("expected Name='test', got %q", f.Name)
	}
	if !f.Enabled {
		t.Error("expected Enabled=true")
	}
	if f.Description != "desc" {
		t.Errorf("expected Description='desc', got %q", f.Description)
	}
}

// -- Upstream Quality: Concurrent access to feature flags

func TestFeatureFlagStoreConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	var wg sync.WaitGroup
	// Multiple goroutines writing flags concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.Enable(fmt.Sprintf("concurrent_flag_%d", idx), "concurrent test")
		}(i)
	}
	// Multiple goroutines reading flags concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = store.Enabled(fmt.Sprintf("concurrent_flag_%d", idx))
			_ = store.List()
		}(i)
	}
	wg.Wait()

	// All 10 flags should be persisted
	flags := store.List()
	if len(flags) != 10 {
		t.Errorf("expected 10 flags after concurrent writes, got %d", len(flags))
	}
}

// -- Upstream Quality: Persistence roundtrip integrity

func TestFeatureFlagStorePersistenceRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	// Enable multiple flags with different descriptions
	store.Enable("flag_a", "Description A")
	store.Enable("flag_b", "Description B")
	store.Disable("flag_a")

	// Reload from file
	store2 := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}
	rawData, _ := os.ReadFile(file)
	json.Unmarshal(rawData, &store2.flags)
	for name, f := range store2.flags {
		f.Name = name
		store2.flags[name] = f
	}

	// Verify flag states match
	if store2.Enabled("flag_a") {
		t.Error("flag_a should be disabled after reload")
	}
	if !store2.Enabled("flag_b") {
		t.Error("flag_b should be enabled after reload")
	}

	// Verify descriptions are preserved
	flags := store2.List()
	for _, f := range flags {
		if f.Name == "flag_a" && f.Description != "Description A" {
			t.Errorf("flag_a description = %q, want 'Description A'", f.Description)
		}
		if f.Name == "flag_b" && f.Description != "Description B" {
			t.Errorf("flag_b description = %q, want 'Description B'", f.Description)
		}
	}
}

// -- Upstream Quality: JSON roundtrip integrity

func TestFeatureFlagJSONRoundtrip(t *testing.T) {
	flags := map[string]FeatureFlag{
		"test_flag": {Name: "test_flag", Enabled: true, Description: "test desc"},
	}
	data, err := json.Marshal(flags)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded map[string]FeatureFlag
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if f, ok := decoded["test_flag"]; !ok {
		t.Error("test_flag missing after roundtrip")
	} else {
		// Name has json:"-" so it won't survive roundtrip
		// That's expected -- Name is derived from map key
		if f.Name != "" {
			t.Errorf("Name should be empty after JSON roundtrip (has json:\"-\" tag), got %q", f.Name)
		}
		if !f.Enabled {
			t.Error("Enabled should be true after roundtrip")
		}
		if f.Description != "test desc" {
			t.Errorf("Description = %q, want 'test desc'", f.Description)
		}
	}
}

// -- Upstream Quality: Save failure recovery

func TestFeatureFlagStoreSaveInvalidPath(t *testing.T) {
	store := &FeatureFlagStore{
		file:  "/nonexistent_dir_12345/feature_flags.json",
		flags: make(map[string]FeatureFlag),
	}

	// Save to invalid path should not crash
	store.Enable("test", "test")
	// Should silently fail (no panic)
}

// -- Upstream Quality: Load corrupted file

func TestFeatureFlagStoreLoadCorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	// Write invalid JSON
	os.WriteFile(file, []byte("not valid json{{{"), 0o644)

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}
	rawData, _ := os.ReadFile(file)
	json.Unmarshal(rawData, &store.flags)
	for name, f := range store.flags {
		f.Name = name
		store.flags[name] = f
	}

	// Store should be empty (corrupted file should not crash)
	flags := store.List()
	if len(flags) != 0 {
		t.Errorf("expected 0 flags from corrupted file, got %d", len(flags))
	}
}

// -- Upstream Quality: Enable with empty description

func TestFeatureFlagStoreEmptyDescription(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "feature_flags.json")

	store := &FeatureFlagStore{
		file:  file,
		flags: make(map[string]FeatureFlag),
	}

	store.Enable("empty_desc", "")
	flags := store.List()
	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
	if flags[0].Description != "" {
		t.Errorf("expected empty description, got %q", flags[0].Description)
	}
}
