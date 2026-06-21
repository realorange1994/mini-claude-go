package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotManager_New(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)
	if m == nil {
		t.Error("expected non-nil manager")
	}
}

func TestSnapshotManager_Init(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)

	err := m.Init()
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .git directory exists
	gitDir := filepath.Join(dir, ".claude", "snapshots", ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error("expected .git directory to exist")
	}
}

func TestSnapshotManager_Init_Idempotent(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)

	m.Init()
	m.Init() // Should not fail
}

func TestSnapshotManager_Track(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)
	m.Init()

	// Create test file
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	err := m.Track([]string{"test.txt"})
	if err != nil {
		t.Fatalf("track failed: %v", err)
	}
}

func TestSnapshotManager_Create(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)
	m.Init()

	// Create and track test file
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)
	m.Track([]string{"test.txt"})

	snapshot, err := m.Create("initial snapshot")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if snapshot.Hash == "" {
		t.Error("expected non-empty hash")
	}
	if snapshot.Message != "initial snapshot" {
		t.Errorf("expected 'initial snapshot', got %q", snapshot.Message)
	}
}

func TestSnapshotManager_List(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)
	m.Init()

	// Create test file and snapshot
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)
	m.Track([]string{"test.txt"})
	m.Create("snapshot 1")

	snapshots, err := m.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(snapshots) == 0 {
		t.Error("expected at least 1 snapshot")
	}
}

func TestSnapshotManager_Restore(t *testing.T) {
	dir := t.TempDir()
	m := NewSnapshotManager(dir)
	m.Init()

	// Create initial file
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("original"), 0644)
	m.Track([]string{"test.txt"})
	snapshot, _ := m.Create("original")

	// Modify file
	os.WriteFile(testFile, []byte("modified"), 0644)

	// Restore from snapshot
	err := m.Restore(snapshot.Hash, []string{"test.txt"})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Verify restored content
	data, _ := os.ReadFile(testFile)
	if string(data) != "original" {
		t.Errorf("expected 'original', got %q", string(data))
	}
}

func TestFormatSnapshotList(t *testing.T) {
	snapshots := []Snapshot{
		{Hash: "abc12345def", Message: "test snapshot"},
	}

	output := FormatSnapshotList(snapshots)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatSnapshotList_Empty(t *testing.T) {
	output := FormatSnapshotList(nil)
	if output != "No snapshots found." {
		t.Errorf("expected 'No snapshots found.', got %q", output)
	}
}
