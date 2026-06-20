package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProgressReconciler_ScanProgress(t *testing.T) {
	dir := t.TempDir()
	progressDir := filepath.Join(dir, "tasks")
	os.MkdirAll(progressDir, 0755)

	// Create progress files
	os.WriteFile(filepath.Join(progressDir, "T1.md"), []byte("Task 1 progress"), 0644)
	os.WriteFile(filepath.Join(progressDir, "T2.md"), []byte("Task 2 progress"), 0644)

	r := NewProgressReconciler(progressDir, "")
	entries := r.ScanProgress()

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestProgressReconciler_BuildProgressDiff(t *testing.T) {
	dir := t.TempDir()
	progressDir := filepath.Join(dir, "tasks")
	os.MkdirAll(progressDir, 0755)

	os.WriteFile(filepath.Join(progressDir, "T1.md"), []byte("Task 1 progress"), 0644)

	r := NewProgressReconciler(progressDir, "")
	diff := r.BuildProgressDiff()

	if len(diff) != 1 {
		t.Errorf("expected 1 diff, got %d", len(diff))
	}
}

func TestProgressReconciler_Reconcile(t *testing.T) {
	dir := t.TempDir()
	progressDir := filepath.Join(dir, "tasks")
	os.MkdirAll(progressDir, 0755)

	os.WriteFile(filepath.Join(progressDir, "T1.md"), []byte("Task 1 progress"), 0644)

	r := NewProgressReconciler(progressDir, "")
	diff := r.BuildProgressDiff()
	r.Reconcile(diff)

	// After reconcile, no new diff
	diff2 := r.BuildProgressDiff()
	if len(diff2) != 0 {
		t.Errorf("expected 0 diff after reconcile, got %d", len(diff2))
	}
}

func TestProgressReconciler_RenderProgressDiffBlock(t *testing.T) {
	dir := t.TempDir()
	progressDir := filepath.Join(dir, "tasks")
	os.MkdirAll(progressDir, 0755)

	os.WriteFile(filepath.Join(progressDir, "T1.md"), []byte("Task 1 progress"), 0644)

	r := NewProgressReconciler(progressDir, "")
	diff := r.BuildProgressDiff()
	block := r.RenderProgressDiffBlock(diff)

	if block == "" {
		t.Error("expected non-empty block")
	}
}

func TestProgressReconciler_RenderProgressDiffBlock_Empty(t *testing.T) {
	r := NewProgressReconciler("", "")
	block := r.RenderProgressDiffBlock(nil)

	if block != "" {
		t.Error("expected empty block for nil diff")
	}
}

func TestProgressReconciler_BuildProgressDiffForCheckpoint(t *testing.T) {
	dir := t.TempDir()
	progressDir := filepath.Join(dir, "tasks")
	os.MkdirAll(progressDir, 0755)

	os.WriteFile(filepath.Join(progressDir, "T1.md"), []byte("Task 1 progress"), 0644)

	r := NewProgressReconciler(progressDir, "")
	block := r.BuildProgressDiffForCheckpoint()

	if block == "" {
		t.Error("expected non-empty block")
	}

	// Second call should be empty (already reconciled)
	block2 := r.BuildProgressDiffForCheckpoint()
	if block2 != "" {
		t.Error("expected empty block after reconcile")
	}
}

func TestProgressReconciler_GetLastReconciled(t *testing.T) {
	dir := t.TempDir()
	progressDir := filepath.Join(dir, "tasks")
	os.MkdirAll(progressDir, 0755)

	os.WriteFile(filepath.Join(progressDir, "T1.md"), []byte("Task 1 progress"), 0644)

	r := NewProgressReconciler(progressDir, "")
	diff := r.BuildProgressDiff()
	r.Reconcile(diff)

	lastReconciled := r.GetLastReconciled("T1")
	if lastReconciled.IsZero() {
		t.Error("expected non-zero last reconciled time")
	}
}

func TestProgressReconciler_NoProgressDir(t *testing.T) {
	r := NewProgressReconciler("", "")

	entries := r.ScanProgress()
	if entries != nil {
		t.Error("expected nil entries for empty dir")
	}
}
