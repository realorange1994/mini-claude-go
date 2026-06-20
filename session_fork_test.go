package main

import (
	"strings"
	"testing"
)

func TestForkSession_FromCheckpoint(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	// Add some entries
	sm.AddNote("state", "Working on auth", "test")
	sm.AddNote("decision", "Use Go 1.25", "test")

	// Write checkpoint
	cpID, err := sm.WriteCheckpoint(nil)
	if err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	// Fork from checkpoint
	forked, err := sm.ForkSession(cpID)
	if err != nil {
		t.Fatalf("fork session: %v", err)
	}

	// Verify forked has same entries
	entries := forked.GetRecentEntries(10)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify fork info
	info := forked.GetForkInfo()
	if info == "" {
		t.Error("expected fork info to be set")
	}
}

func TestForkSessionAtPoint(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	// Add entries
	sm.AddNote("state", "Step 1", "test")
	sm.AddNote("state", "Step 2", "test")
	sm.AddNote("state", "Step 3", "test")

	// Get entries for forking
	entries := sm.GetRecentEntries(10)

	// Fork at point 1 (should include Step 1 and Step 2)
	forked, err := sm.ForkSessionAtPoint(entries, 1)
	if err != nil {
		t.Fatalf("fork at point: %v", err)
	}

	// Verify forked has entries up to point
	forkedEntries := forked.GetRecentEntries(10)
	if len(forkedEntries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(forkedEntries))
	}
}

func TestForkSessionAtPoint_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	sm.AddNote("state", "Step 1", "test")

	entries := sm.GetRecentEntries(10)

	// Fork at invalid point
	_, err := sm.ForkSessionAtPoint(entries, 5)
	if err == nil {
		t.Error("expected error for out of range point")
	}

	// Fork at negative point
	_, err = sm.ForkSessionAtPoint(entries, -1)
	if err == nil {
		t.Error("expected error for negative point")
	}
}

func TestForkSession_CheckpointNotFound(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	// Fork from non-existent checkpoint
	_, err := sm.ForkSession("cp-nonexistent")
	if err == nil {
		t.Error("expected error for non-existent checkpoint")
	}
}

func TestForkSession_IndependentCopy(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	sm.AddNote("state", "Original", "test")

	cpID, _ := sm.WriteCheckpoint(nil)

	forked, _ := sm.ForkSession(cpID)

	// Modify original
	sm.AddNote("state", "Added after fork", "test")

	// Forked should not be affected
	entries := forked.GetRecentEntries(10)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in forked, got %d", len(entries))
	}
}

func TestGetForkInfo_NotForked(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	info := sm.GetForkInfo()
	if info != "" {
		t.Errorf("expected empty fork info, got %q", info)
	}
}

func TestForkSession_DifferentSessionFile(t *testing.T) {
	dir := t.TempDir()
	sm := NewSessionMemory(dir)

	sm.AddNote("state", "Original", "test")

	cpID, _ := sm.WriteCheckpoint(nil)

	forked, _ := sm.ForkSession(cpID)

	// Verify forked has different file path
	smPath := sm.filePath
	forkedPath := forked.filePath
	if smPath == forkedPath {
		t.Error("expected different file paths for forked session")
	}

	// Verify forked path contains fork prefix
	if !strings.Contains(forkedPath, "fork-") {
		t.Errorf("expected fork prefix in path, got %q", forkedPath)
	}
}
