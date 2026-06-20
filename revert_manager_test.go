package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevertManager_CreateSnapshot(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	files := map[string][]byte{
		"main.go": []byte("package main"),
		"utils.go": []byte("package utils"),
	}

	snapshotID := m.CreateSnapshot("session-1", files)
	if snapshotID == "" {
		t.Error("expected non-empty snapshot ID")
	}
}

func TestRevertManager_RevertToFile(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	// Create original file
	originalPath := filepath.Join(dir, "main.go")
	os.WriteFile(originalPath, []byte("original content"), 0644)

	// Create snapshot
	files := map[string][]byte{
		"main.go": []byte("original content"),
	}
	snapshotID := m.CreateSnapshot("session-1", files)

	// Modify file
	os.WriteFile(originalPath, []byte("modified content"), 0644)

	// Revert
	err := m.RevertToFile("session-1", originalPath, snapshotID)
	if err != nil {
		t.Fatalf("revert failed: %v", err)
	}

	// Verify
	data, _ := os.ReadFile(originalPath)
	if string(data) != "original content" {
		t.Errorf("expected 'original content', got %q", string(data))
	}
}

func TestRevertManager_RevertSession(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	// Create original files
	path1 := filepath.Join(dir, "main.go")
	path2 := filepath.Join(dir, "utils.go")
	os.WriteFile(path1, []byte("original main"), 0644)
	os.WriteFile(path2, []byte("original utils"), 0644)

	// Create snapshot
	files := map[string][]byte{
		"main.go": []byte("original main"),
		"utils.go": []byte("original utils"),
	}
	snapshotID := m.CreateSnapshot("session-1", files)

	// Modify files
	os.WriteFile(path1, []byte("modified main"), 0644)
	os.WriteFile(path2, []byte("modified utils"), 0644)

	// Revert session
	err := m.RevertSession("session-1", snapshotID, []string{path1, path2})
	if err != nil {
		t.Fatalf("revert session failed: %v", err)
	}

	// Verify
	data1, _ := os.ReadFile(path1)
	data2, _ := os.ReadFile(path2)
	if string(data1) != "original main" {
		t.Errorf("expected 'original main', got %q", string(data1))
	}
	if string(data2) != "original utils" {
		t.Errorf("expected 'original utils', got %q", string(data2))
	}
}

func TestRevertManager_UndoRevert(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	// Create original file
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("original"), 0644)

	// Create snapshot
	files := map[string][]byte{"main.go": []byte("original")}
	snapshotID := m.CreateSnapshot("session-1", files)

	// Modify and revert
	os.WriteFile(path, []byte("modified"), 0644)
	m.RevertSession("session-1", snapshotID, []string{path})

	// Undo revert
	err := m.UndoRevert("session-1")
	if err != nil {
		t.Fatalf("undo revert failed: %v", err)
	}

	// Verify restored to modified
	data, _ := os.ReadFile(path)
	if string(data) != "modified" {
		t.Errorf("expected 'modified', got %q", string(data))
	}
}

func TestRevertManager_HasRevertState(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	if m.HasRevertState("session-1") {
		t.Error("expected no revert state initially")
	}

	// Create snapshot and revert
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("content"), 0644)
	files := map[string][]byte{"main.go": []byte("content")}
	snapshotID := m.CreateSnapshot("session-1", files)
	m.RevertSession("session-1", snapshotID, []string{path})

	if !m.HasRevertState("session-1") {
		t.Error("expected revert state after revert")
	}
}

func TestRevertManager_GetRevertState(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	if m.GetRevertState("session-1") != nil {
		t.Error("expected nil state initially")
	}
}

func TestRevertManager_UndoRevert_NoState(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	err := m.UndoRevert("session-1")
	if err == nil {
		t.Error("expected error for no revert state")
	}
}

func TestRevertManager_SaveLoadMetadata(t *testing.T) {
	dir := t.TempDir()
	m := NewRevertManager(dir)

	// Create snapshot and revert
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("content"), 0644)
	files := map[string][]byte{"main.go": []byte("content")}
	snapshotID := m.CreateSnapshot("session-1", files)
	m.RevertSession("session-1", snapshotID, []string{path})

	// Save metadata
	err := m.SaveRevertMetadata("session-1")
	if err != nil {
		t.Fatalf("save metadata failed: %v", err)
	}

	// Load metadata
	state, err := m.LoadRevertMetadata("session-1")
	if err != nil {
		t.Fatalf("load metadata failed: %v", err)
	}

	if state.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", state.SessionID)
	}
}
