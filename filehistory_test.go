package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── Core SnapshotHistory tests ───

func TestSnapshotTakeAndRewind(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("original"), 0644)

	sh := NewSnapshotHistory(dir)
	if err := sh.TakeSnapshot(fp); err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	// Modify the file
	os.WriteFile(fp, []byte("modified"), 0644)
	data, _ := os.ReadFile(fp)
	if string(data) != "modified" {
		t.Fatalf("expected 'modified', got %s", data)
	}

	// Rewind
	if err := sh.RewindTo(fp, 0); err != nil {
		t.Fatalf("RewindTo: %v", err)
	}

	data, _ = os.ReadFile(fp)
	if string(data) != "original" {
		t.Errorf("expected 'original' after rewind, got %s", data)
	}
}

func TestSnapshotList(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "list_test.txt")
	os.WriteFile(fp, []byte("v1"), 0644)

	sh := NewSnapshotHistory(dir)
	_ = sh.TakeSnapshot(fp)
	os.WriteFile(fp, []byte("v2"), 0644)
	_ = sh.TakeSnapshot(fp)

	snapshots := sh.ListSnapshots(fp)
	if len(snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(snapshots))
	}
}

func TestSnapshotRewindNonexistentFile(t *testing.T) {
	dir := t.TempDir()
	sh := NewSnapshotHistory(dir)
	err := sh.RewindTo(filepath.Join(dir, "nope.txt"), 0)
	if err == nil {
		t.Error("expected error rewinding nonexistent file")
	}
}

func TestSnapshotTakeNonexistentDir(t *testing.T) {
	dir := t.TempDir()
	snapDir := filepath.Join(dir, "snapshots_subdir")
	sh := NewSnapshotHistory(snapDir)
	fp := filepath.Join(dir, "snap_file.txt")
	os.WriteFile(fp, []byte("content"), 0644)

	if err := sh.TakeSnapshot(fp); err != nil {
		t.Fatalf("TakeSnapshot with subdir: %v", err)
	}
}

func TestSnapshotRewindIndexOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "idx.txt")
	os.WriteFile(fp, []byte("first"), 0644)

	sh := NewSnapshotHistory(dir)
	_ = sh.TakeSnapshot(fp)

	err := sh.RewindTo(fp, 99)
	if err == nil {
		t.Error("expected error rewinding to out-of-bounds index")
	}
}

func TestSnapshotMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.txt")
	f2 := filepath.Join(dir, "b.txt")
	os.WriteFile(f1, []byte("a1"), 0644)
	os.WriteFile(f2, []byte("b1"), 0644)

	sh := NewSnapshotHistory(dir)
	_ = sh.TakeSnapshot(f1)
	_ = sh.TakeSnapshot(f2)

	os.WriteFile(f1, []byte("a2"), 0644)
	os.WriteFile(f2, []byte("b2"), 0644)

	_ = sh.RewindTo(f1, 0)
	_ = sh.RewindTo(f2, 0)

	data1, _ := os.ReadFile(f1)
	data2, _ := os.ReadFile(f2)
	if string(data1) != "a1" {
		t.Errorf("expected a1, got %s", data1)
	}
	if string(data2) != "b1" {
		t.Errorf("expected b1, got %s", data2)
	}
}

// ─── New feature tests ───

func TestTakeSnapshotWithDesc(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	os.WriteFile(fp, []byte("content"), 0644)

	sh := NewSnapshotHistory(dir)
	if err := sh.TakeSnapshotWithDesc(fp, "before edit_file"); err != nil {
		t.Fatalf("TakeSnapshotWithDesc failed: %v", err)
	}

	snaps := sh.ListSnapshots(fp)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].Description != "before edit_file" {
		t.Errorf("expected description 'before edit_file', got %q", snaps[0].Description)
	}
}

func TestMultipleSnapshotsWithDescriptions(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")

	sh := NewSnapshotHistory(dir)

	// v1: file doesn't exist yet
	sh.TakeSnapshot(fp)

	// Write file and take another snapshot
	os.WriteFile(fp, []byte("version 1"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after write_file")

	// Edit and take another snapshot
	os.WriteFile(fp, []byte("version 2"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after edit_file")

	snaps := sh.ListSnapshots(fp)
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}

	// v1 should be empty (file didn't exist)
	if snaps[0].Content != "" {
		t.Errorf("v1 should be empty, got %q", snaps[0].Content)
	}
	if snaps[1].Content != "version 1" {
		t.Errorf("v2 content mismatch: got %q", snaps[1].Content)
	}
	if snaps[2].Content != "version 2" {
		t.Errorf("v3 content mismatch: got %q", snaps[2].Content)
	}

	// Check descriptions
	if snaps[0].Description != "" {
		t.Errorf("v1 description should be empty, got %q", snaps[0].Description)
	}
	if snaps[1].Description != "after write_file" {
		t.Errorf("v2 description mismatch: got %q", snaps[1].Description)
	}
	if snaps[2].Description != "after edit_file" {
		t.Errorf("v3 description mismatch: got %q", snaps[2].Description)
	}
}

func TestRestoreLast(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")

	sh := NewSnapshotHistory(dir)

	// Create 3 versions
	sh.TakeSnapshot(fp) // v1: empty
	os.WriteFile(fp, []byte("version 1"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after write_file") // v2
	os.WriteFile(fp, []byte("version 2"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after edit_file") // v3

	// Current file should be "version 2"
	data, _ := os.ReadFile(fp)
	if string(data) != "version 2" {
		t.Fatalf("current content should be 'version 2', got %q", string(data))
	}

	// Restore last (should go back to v2 = "version 1")
	content, err := sh.RestoreLast(fp)
	if err != nil {
		t.Fatalf("RestoreLast failed: %v", err)
	}
	if content != "version 1" {
		t.Errorf("restored content should be 'version 1', got %q", content)
	}

	// File on disk should now be "version 1"
	data, _ = os.ReadFile(fp)
	if string(data) != "version 1" {
		t.Errorf("disk content should be 'version 1', got %q", string(data))
	}

	// After restore, there should be a new snapshot (pre-restore state)
	snaps := sh.ListSnapshots(fp)
	if len(snaps) != 4 {
		t.Errorf("expected 4 snapshots after restore (3 original + 1 pre-restore), got %d", len(snaps))
	}
	// The last snapshot should be the pre-restore state ("version 2")
	if snaps[3].Content != "version 2" {
		t.Errorf("pre-restore snapshot should be 'version 2', got %q", snaps[3].Content)
	}
}

func TestRewindSteps(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")

	sh := NewSnapshotHistory(dir)

	// Create 4 versions
	sh.TakeSnapshot(fp) // v1: empty
	os.WriteFile(fp, []byte("v1"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after write") // v2
	os.WriteFile(fp, []byte("v2"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after edit1") // v3
	os.WriteFile(fp, []byte("v3"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after edit2") // v4

	// Rewind 2 steps: from v4, go back 2 → v2 ("v1")
	content, err := sh.RewindSteps(fp, 2)
	if err != nil {
		t.Fatalf("RewindSteps failed: %v", err)
	}
	if content != "v1" {
		t.Errorf("rewound content should be 'v1', got %q", content)
	}

	// File on disk should be "v1"
	data, _ := os.ReadFile(fp)
	if string(data) != "v1" {
		t.Errorf("disk content should be 'v1', got %q", string(data))
	}

	// Pre-restore snapshot should exist
	snaps := sh.ListSnapshots(fp)
	if len(snaps) != 5 {
		t.Errorf("expected 5 snapshots after rewind, got %d", len(snaps))
	}
}

func TestRewindToEmptyDeletesFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")

	sh := NewSnapshotHistory(dir)
	sh.TakeSnapshot(fp) // v1: empty
	os.WriteFile(fp, []byte("exists"), 0644)
	sh.TakeSnapshotWithDesc(fp, "after write") // v2

	// Rewind to v1 (empty) — should delete the file
	err := sh.RewindTo(fp, 0)
	if err != nil {
		t.Fatalf("RewindTo failed: %v", err)
	}

	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("file should be deleted after rewinding to empty snapshot")
	}
}

// ─── Disk persistence tests ───

func TestDiskPersistence(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "persist.txt")
	os.WriteFile(fp, []byte("persisted content"), 0644)

	sh := NewSnapshotHistory(dir)
	sh.TakeSnapshotWithDesc(fp, "test persistence")

	// Create a new SnapshotHistory pointing to the same directory
	sh2 := NewSnapshotHistory(dir)

	// Should load from disk
	snaps := sh2.ListSnapshots(fp)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot from disk, got %d", len(snaps))
	}
	if snaps[0].Content != "persisted content" {
		t.Errorf("content mismatch: got %q", snaps[0].Content)
	}
	if snaps[0].Description != "test persistence" {
		t.Errorf("description mismatch: got %q", snaps[0].Description)
	}
}

func TestListAllFiles(t *testing.T) {
	dir := t.TempDir()
	fp1 := filepath.Join(dir, "a.txt")
	fp2 := filepath.Join(dir, "b.txt")
	os.WriteFile(fp1, []byte("a"), 0644)
	os.WriteFile(fp2, []byte("b"), 0644)

	sh := NewSnapshotHistory(dir)
	sh.TakeSnapshot(fp1)
	sh.TakeSnapshot(fp2)

	files := sh.ListAllFiles()
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}

	// Test disk fallback
	sh2 := NewSnapshotHistory(dir)
	files2 := sh2.ListAllFiles()
	if len(files2) != 2 {
		t.Errorf("expected 2 files from disk, got %d", len(files2))
	}
}
