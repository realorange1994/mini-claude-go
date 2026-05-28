package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsManager_LoadSave(t *testing.T) {
	tmp := t.TempDir()
	sm := NewSettingsManager(filepath.Join(tmp, "global"), filepath.Join(tmp, "project"))

	// Save global settings
	sm.SetGlobal(Settings{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 8192,
	})
	if err := sm.Save(ScopeGlobal); err != nil {
		t.Fatal(err)
	}

	// Create new manager and reload
	sm2 := NewSettingsManager(sm.globalDir, sm.projectDir)
	if err := sm2.Load(); err != nil {
		t.Fatal(err)
	}

	merged := sm2.Merged()
	if merged.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want claude-sonnet-4-20250514", merged.Model)
	}
	if merged.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", merged.MaxTokens)
	}
}

func TestSettingsManager_ProjectOverride(t *testing.T) {
	tmp := t.TempDir()
	sm := NewSettingsManager(filepath.Join(tmp, "global"), filepath.Join(tmp, "project"))

	sm.SetGlobal(Settings{
		Model:     "claude-haiku",
		MaxTokens: 4096,
	})
	sm.SetProject(Settings{
		Model:     "claude-opus",
		MaxTokens: 32000,
	})

	if err := sm.Save(ScopeGlobal); err != nil {
		t.Fatal(err)
	}
	if err := sm.Save(ScopeProject); err != nil {
		t.Fatal(err)
	}

	merged := sm.Merged()
	if merged.Model != "claude-opus" {
		t.Errorf("Model = %q, want claude-opus (project override)", merged.Model)
	}
	if merged.MaxTokens != 32000 {
		t.Errorf("MaxTokens = %d, want 32000", merged.MaxTokens)
	}
}

func TestSettingsManager_PartialMerge(t *testing.T) {
	global := &Settings{
		Model:     "claude-haiku",
		MaxTokens: 4096,
		APIKey:    "key1",
	}
	project := &Settings{
		Model: "claude-opus",
		// MaxTokens intentionally zero — should keep global's
	}

	merged := mergeSettings(global, project)
	if merged.Model != "claude-opus" {
		t.Errorf("Model = %q, want claude-opus", merged.Model)
	}
	if merged.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096 (should keep global)", merged.MaxTokens)
	}
	if merged.APIKey != "key1" {
		t.Errorf("APIKey = %q, want key1", merged.APIKey)
	}
}

func TestSettingsManager_UpdateProject(t *testing.T) {
	sm := NewSettingsManager("", "")
	sm.SetProject(Settings{Model: "haiku"})

	sm.UpdateProject(func(s *Settings) {
		s.MaxTokens = 8192
	})

	p := sm.GetProject()
	if p.Model != "haiku" {
		t.Errorf("Model = %q, want haiku (should be preserved)", p.Model)
	}
	if p.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", p.MaxTokens)
	}
}

func TestSettingsManager_GlobalDir(t *testing.T) {
	tmp := t.TempDir()
	sm := NewSettingsManager(filepath.Join(tmp, "g"), filepath.Join(tmp, "p"))

	if sm.GlobalDir() != filepath.Join(tmp, "g") {
		t.Error("global dir mismatch")
	}
	if sm.ProjectDir() != filepath.Join(tmp, "p") {
		t.Error("project dir mismatch")
	}
}

func TestSettingsManager_LoadNonExistentProject(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "global")
	projectDir := filepath.Join(tmp, "no-project") // dir doesn't exist

	sm := NewSettingsManager(globalDir, projectDir)
	if err := sm.Load(); err != nil {
		t.Errorf("Load should succeed when project dir doesn't exist: %v", err)
	}
}

func TestSettingsManager_SaveCreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	sm := NewSettingsManager(filepath.Join(tmp, "new-global"), "")
	sm.SetGlobal(Settings{Model: "test"})

	if err := sm.Save(ScopeGlobal); err != nil {
		t.Fatalf("Save should create directory: %v", err)
	}

	_, err := os.Stat(filepath.Join(tmp, "new-global", "settings.json"))
	if err != nil {
		t.Error("settings.json should exist after Save")
	}
}
