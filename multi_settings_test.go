package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Ported from upstream: src/utils/__tests__/settings.test.ts
// Tests for MultiSourceSettings (multi-level settings hierarchy)

func TestMultiSourceSettingsDefault(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)
	if ms == nil {
		t.Fatal("NewMultiSourceSettings returned nil")
	}

	sources := ms.Sources()
	if len(sources) != 5 {
		t.Fatalf("expected 5 sources, got %d", len(sources))
	}

	// Verify source levels in order
	if sources[0].Level != SettingsDefault {
		t.Errorf("source[0] should be default, got %v", sources[0].Level)
	}
	if sources[1].Level != SettingsGlobal {
		t.Errorf("source[1] should be global, got %v", sources[1].Level)
	}
	if sources[2].Level != SettingsProject {
		t.Errorf("source[2] should be project, got %v", sources[2].Level)
	}
	if sources[3].Level != SettingsWorktree {
		t.Errorf("source[3] should be worktree, got %v", sources[3].Level)
	}
	if sources[4].Level != SettingsSession {
		t.Errorf("source[4] should be session, got %v", sources[4].Level)
	}
}

func TestMultiSourceSettingsGetNonExistent(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)

	v, ok := ms.Get("nonexistent_key")
	if ok {
		t.Errorf("expected key not found, got %v", v)
	}
}

func TestMultiSourceSettingsSetSessionOverride(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)

	ms.SetSession("test_key", "session_value")

	v, ok := ms.Get("test_key")
	if !ok {
		t.Fatal("expected key to be found")
	}
	if v != "session_value" {
		t.Errorf("expected 'session_value', got %v", v)
	}
}

func TestMultiSourceSettingsProjectOverridesGlobal(t *testing.T) {
	// Create global settings in ~/.claude/
	homeDir := t.TempDir()
	claudeHome := filepath.Join(homeDir, ".claude")
	os.MkdirAll(claudeHome, 0755)
	globalSettings := `{"model": "claude-global", "maxTurns": 50}`
	os.WriteFile(filepath.Join(claudeHome, "settings.json"), []byte(globalSettings), 0644)

	// Create project settings
	projectDir := t.TempDir()
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)
	projectSettings := `{"model": "claude-project"}`
	os.WriteFile(filepath.Join(projectDir, ".claude", "settings.json"), []byte(projectSettings), 0644)

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	originalUserProfile := os.Getenv("USERPROFILE")
	// On Windows, USERPROFILE takes precedence; use that for the test
	os.Setenv("USERPROFILE", homeDir)
	os.Setenv("HOME", homeDir)
	defer func() {
		os.Setenv("HOME", originalHome)
		if originalUserProfile == "" {
			os.Unsetenv("USERPROFILE")
		} else {
			os.Setenv("USERPROFILE", originalUserProfile)
		}
	}()

	ms := NewMultiSourceSettings(projectDir)

	// Project should override global for "model"
	model := ms.GetString("model")
	if model != "claude-project" {
		t.Errorf("expected 'claude-project', got %q", model)
	}

	// Global value should be inherited for "maxTurns"
	maxTurns := ms.GetInt("maxTurns")
	if maxTurns != 50 {
		t.Errorf("expected maxTurns=50 from global, got %d", maxTurns)
	}
}

func TestMultiSourceSettingsWorktreeOverridesProject(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	// Project settings
	projectSettings := `{"model": "claude-project", "maxTurns": 90}`
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(projectSettings), 0644)

	// Worktree settings
	worktreeSettings := `{"model": "claude-worktree"}`
	os.WriteFile(filepath.Join(dir, ".claude", "settings.local.json"), []byte(worktreeSettings), 0644)

	ms := NewMultiSourceSettings(dir)

	// Worktree should override project
	model := ms.GetString("model")
	if model != "claude-worktree" {
		t.Errorf("expected 'claude-worktree', got %q", model)
	}

	// Project value should be inherited for "maxTurns"
	maxTurns := ms.GetInt("maxTurns")
	if maxTurns != 90 {
		t.Errorf("expected maxTurns=90 from project, got %d", maxTurns)
	}
}

func TestMultiSourceSettingsSessionOverridesAll(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	// Project settings
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"model": "claude-project"}`), 0644)

	ms := NewMultiSourceSettings(dir)
	ms.SetSession("model", "claude-session")

	model := ms.GetString("model")
	if model != "claude-session" {
		t.Errorf("expected 'claude-session', got %q", model)
	}
}

func TestMultiSourceSettingsGetString(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)
	ms.SetSession("name", "test_name")

	got := ms.GetString("name")
	if got != "test_name" {
		t.Errorf("expected 'test_name', got %q", got)
	}

	// Non-existent key returns empty
	got2 := ms.GetString("missing")
	if got2 != "" {
		t.Errorf("expected empty string for missing key, got %q", got2)
	}

	// Non-string type returns empty
	ms.SetSession("count", 42)
	got3 := ms.GetString("count")
	if got3 != "" {
		t.Errorf("expected empty for non-string type, got %q", got3)
	}
}

func TestMultiSourceSettingsGetInt(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)
	ms.SetSession("count", 42.0)

	got := ms.GetInt("count")
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}

	// Non-existent key returns 0
	if ms.GetInt("missing") != 0 {
		t.Error("expected 0 for missing key")
	}

	// int type from SetSession
	ms.SetSession("int_val", 99)
	if ms.GetInt("int_val") != 99 {
		t.Errorf("expected 99, got %d", ms.GetInt("int_val"))
	}
}

func TestMultiSourceSettingsGetBool(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)
	ms.SetSession("enabled", true)
	ms.SetSession("disabled", false)

	if !ms.GetBool("enabled") {
		t.Error("expected true for enabled key")
	}
	if ms.GetBool("disabled") {
		t.Error("expected false for disabled key")
	}

	// Non-existent key returns false
	if ms.GetBool("missing") {
		t.Error("expected false for missing key")
	}

	// Non-bool type returns false
	ms.SetSession("count", 42)
	if ms.GetBool("count") {
		t.Error("expected false for non-bool type")
	}
}

func TestMultiSourceSettingsSourceOf(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)

	// Set a session value
	ms.SetSession("session_key", "value")

	source := ms.SourceOf("session_key")
	if source != SettingsSession {
		t.Errorf("expected source to be session, got %v", source)
	}

	// Non-existent key returns default
	source2 := ms.SourceOf("missing")
	if source2 != SettingsDefault {
		t.Errorf("expected source to be default, got %v", source2)
	}
}

func TestMultiSourceSettingsMerged(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"key_a": "project", "key_b": "project"}`), 0644)

	os.WriteFile(filepath.Join(dir, ".claude", "settings.local.json"),
		[]byte(`{"key_b": "worktree", "key_c": "worktree"}`), 0644)

	ms := NewMultiSourceSettings(dir)
	merged := ms.Merged()

	if merged["key_a"] != "project" {
		t.Errorf("key_a should be 'project', got %v", merged["key_a"])
	}
	if merged["key_b"] != "worktree" {
		t.Errorf("key_b should be 'worktree' (override), got %v", merged["key_b"])
	}
	if merged["key_c"] != "worktree" {
		t.Errorf("key_c should be 'worktree', got %v", merged["key_c"])
	}
}

func TestMultiSourceSettingsMergedContainsAllKeys(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)

	ms.SetSession("a", 1)
	ms.SetSession("b", "two")
	ms.SetSession("c", true)

	merged := ms.Merged()
	if len(merged) < 3 {
		t.Errorf("expected at least 3 keys, got %d", len(merged))
	}
}

func TestMultiSourceSettingsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)

	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"invalid json`), 0644)

	// Should not panic; file should be marked as not loaded
	ms := NewMultiSourceSettings(dir)

	sources := ms.Sources()
	// Project source (index 2) should not be loaded
	if sources[2].Loaded {
		t.Error("project source should not be loaded for invalid JSON")
	}
}

func TestMultiSourceSettingsEmptyProjectDir(t *testing.T) {
	ms := NewMultiSourceSettings("")

	// Should still have 5 sources
	if len(ms.Sources()) != 5 {
		t.Errorf("expected 5 sources, got %d", len(ms.Sources()))
	}

	// Project and worktree sources should have empty paths
	sources := ms.Sources()
	if sources[2].Path != "" {
		t.Error("project path should be empty for empty projectDir")
	}
	if sources[3].Path != "" {
		t.Error("worktree path should be empty for empty projectDir")
	}
}

func TestMultiSourceSettingsLevelString(t *testing.T) {
	if SettingsDefault.String() != "default" {
		t.Errorf("expected 'default', got %q", SettingsDefault.String())
	}
	if SettingsGlobal.String() != "global" {
		t.Errorf("expected 'global', got %q", SettingsGlobal.String())
	}
	if SettingsProject.String() != "project" {
		t.Errorf("expected 'project', got %q", SettingsProject.String())
	}
	if SettingsWorktree.String() != "worktree" {
		t.Errorf("expected 'worktree', got %q", SettingsWorktree.String())
	}
	if SettingsSession.String() != "session" {
		t.Errorf("expected 'session', got %q", SettingsSession.String())
	}

	// Unknown level
	unknown := SettingsLevel(999)
	if unknown.String() != "unknown" {
		t.Errorf("expected 'unknown', got %q", unknown.String())
	}
}

func TestMultiSourceSettingsTypeCoercion(t *testing.T) {
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)

	// Set values of different types
	ms.SetSession("string_val", "hello")
	ms.SetSession("int_val", 42.0)
	ms.SetSession("float_val", 3.14)
	ms.SetSession("bool_val", true)
}

func TestMultiSourceSettingsJSONMarshaling(t *testing.T) {
	// Verify settings file structure can be serialized
	sf := SettingsFile{
		Level:  SettingsProject,
		Path:   "/project/.claude/settings.json",
		Values: map[string]any{"key": "value"},
		Loaded: true,
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("failed to marshal SettingsFile: %v", err)
	}

	var sf2 SettingsFile
	if err := json.Unmarshal(data, &sf2); err != nil {
		t.Fatalf("failed to unmarshal SettingsFile: %v", err)
	}
	if sf2.Path != sf.Path {
		t.Errorf("path mismatch after roundtrip: %q vs %q", sf2.Path, sf.Path)
	}
	if sf2.Loaded != sf.Loaded {
		t.Error("loaded mismatch after roundtrip")
	}
}

func TestMultiSourceSettingsGetRecentlyReadFiles(t *testing.T) {
	// Verify that merged settings can be used for downstream operations
	dir := t.TempDir()
	ms := NewMultiSourceSettings(dir)

	ms.SetSession("feature_enabled", true)
	ms.SetSession("max_files", 10.0)

	merged := ms.Merged()
	if !merged["feature_enabled"].(bool) {
		t.Error("feature_enabled should be true")
	}
	if int(merged["max_files"].(float64)) != 10 {
		t.Error("max_files should be 10")
	}
}
