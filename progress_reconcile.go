package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── Progress Reconciliation (MiMo-Code 4B) ─────────────────────────────────
//
// Scans task progress files and compares them with the main checkpoint
// to produce a diff block. Ensures subagent progress is never lost
// between checkpoint updates.
//
// MiMo-Code source: session/checkpoint-progress-reconcile.ts (111 lines)

// ProgressEntry represents a task progress entry.
type ProgressEntry struct {
	TaskID      string
	Content     string
	WrittenAt   time.Time
	Reconciled  bool
}

// ProgressReconciler reconciles task progress with checkpoints.
type ProgressReconciler struct {
	progressDir     string
	checkpointDir   string
	lastReconciled  map[string]time.Time // taskID -> last reconciled time
}

// NewProgressReconciler creates a new progress reconciler.
func NewProgressReconciler(progressDir, checkpointDir string) *ProgressReconciler {
	return &ProgressReconciler{
		progressDir:    progressDir,
		checkpointDir:  checkpointDir,
		lastReconciled: make(map[string]time.Time),
	}
}

// ScanProgress scans progress files and returns entries.
func (r *ProgressReconciler) ScanProgress() []ProgressEntry {
	if r.progressDir == "" {
		return nil
	}

	entries, err := os.ReadDir(r.progressDir)
	if err != nil {
		return nil
	}

	var results []ProgressEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		taskID := strings.TrimSuffix(entry.Name(), ".md")
		path := filepath.Join(r.progressDir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		results = append(results, ProgressEntry{
			TaskID:    taskID,
			Content:   string(data),
			WrittenAt: info.ModTime(),
		})
	}

	return results
}

// BuildProgressDiff builds a diff of new and changed progress entries.
func (r *ProgressReconciler) BuildProgressDiff() []ProgressEntry {
	entries := r.ScanProgress()

	var diff []ProgressEntry
	for _, entry := range entries {
		lastReconciled, exists := r.lastReconciled[entry.TaskID]
		if !exists || entry.WrittenAt.After(lastReconciled) {
			diff = append(diff, entry)
		}
	}

	return diff
}

// RenderProgressDiffBlock renders the progress diff as a markdown block.
func (r *ProgressReconciler) RenderProgressDiffBlock(diff []ProgressEntry) string {
	if len(diff) == 0 {
		return ""
	}

	var sb string
	sb += "## Progress Diff\n\n"
	sb += fmt.Sprintf("_%d tasks updated since last reconciliation._\n\n", len(diff))

	for _, entry := range diff {
		sb += fmt.Sprintf("### Task: %s\n", entry.TaskID)
		sb += fmt.Sprintf("_Updated: %s_\n\n", entry.WrittenAt.Format(time.RFC3339))

		// Truncate content if too long
		content := entry.Content
		if len(content) > 500 {
			content = content[:500] + "\n[... truncated ...]"
		}
		sb += content + "\n\n"
	}

	return sb
}

// Reconcile marks entries as reconciled.
func (r *ProgressReconciler) Reconcile(entries []ProgressEntry) {
	now := time.Now()
	for _, entry := range entries {
		r.lastReconciled[entry.TaskID] = now
	}
}

// GetLastReconciled returns the last reconciled time for a task.
func (r *ProgressReconciler) GetLastReconciled(taskID string) time.Time {
	return r.lastReconciled[taskID]
}

// BuildProgressDiffForCheckpoint builds the full progress diff block
// for inclusion in a checkpoint.
func (r *ProgressReconciler) BuildProgressDiffForCheckpoint() string {
	diff := r.BuildProgressDiff()
	if len(diff) == 0 {
		return ""
	}

	block := r.RenderProgressDiffBlock(diff)
	r.Reconcile(diff)

	return block
}
