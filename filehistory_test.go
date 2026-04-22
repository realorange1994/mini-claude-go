package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
	// Should create the snapshots directory if it doesn't exist
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
