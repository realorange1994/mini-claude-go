package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetRecentlyReadFiles(t *testing.T) {
	r := NewRegistry()

	dir := t.TempDir()
	// Create some files
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644)
	}

	// Mark files as read with different read times
	r.MarkFileRead(filepath.Join(dir, "a.go"))
	time.Sleep(15 * time.Millisecond)
	r.MarkFileRead(filepath.Join(dir, "b.go"))
	time.Sleep(15 * time.Millisecond)
	r.MarkFileRead(filepath.Join(dir, "c.go"))

	// Get all 3
	paths := r.GetRecentlyReadFiles(3)
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d", len(paths))
	}

	// Most recently read should be first
	if filepath.Base(paths[0]) != "c.go" {
		t.Errorf("expected first path to be c.go, got %s", filepath.Base(paths[0]))
	}
	if filepath.Base(paths[1]) != "b.go" {
		t.Errorf("expected second path to be b.go, got %s", filepath.Base(paths[1]))
	}
	if filepath.Base(paths[2]) != "a.go" {
		t.Errorf("expected third path to be a.go, got %s", filepath.Base(paths[2]))
	}

	// Limit to 2
	paths2 := r.GetRecentlyReadFiles(2)
	if len(paths2) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths2))
	}
	if filepath.Base(paths2[0]) != "c.go" {
		t.Errorf("expected first limited path to be c.go, got %s", filepath.Base(paths2[0]))
	}

	// Empty registry
	r2 := NewRegistry()
	paths3 := r2.GetRecentlyReadFiles(5)
	if len(paths3) != 0 {
		t.Errorf("expected 0 paths from empty registry, got %d", len(paths3))
	}
}

func TestCheckFileStale(t *testing.T) {
	r := NewRegistry()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("original"), 0644)

	// Not read yet = stale
	if !r.CheckFileStale(path) {
		t.Error("expected unread file to be stale")
	}

	// Read the file
	r.MarkFileRead(path)
	// Same content = not stale
	if r.CheckFileStale(path) {
		t.Error("expected just-read file to not be stale")
	}

	// Modify the file (ensure mtime changes)
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("modified"), 0644)
	// Modified = stale
	if !r.CheckFileStale(path) {
		t.Error("expected modified file to be stale")
	}
}
