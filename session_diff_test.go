package main

import (
	"testing"
	"time"
)

func TestSessionDiff_RecordChange(t *testing.T) {
	d := NewSessionDiff("test-session")

	d.RecordChange("main.go", 10, 5)
	d.RecordChange("utils.go", 20, 3)

	summary := d.GetSummary()
	if summary.FilesChanged != 2 {
		t.Errorf("expected 2 files changed, got %d", summary.FilesChanged)
	}
	if summary.Additions != 30 {
		t.Errorf("expected 30 additions, got %d", summary.Additions)
	}
	if summary.Deletions != 8 {
		t.Errorf("expected 8 deletions, got %d", summary.Deletions)
	}
}

func TestSessionDiff_FormatDiff(t *testing.T) {
	d := NewSessionDiff("test-session")

	d.RecordChange("main.go", 10, 5)
	d.RecordChange("utils.go", 20, 3)

	diff := d.FormatDiff()
	if diff == "" {
		t.Error("expected non-empty diff")
	}
	if diff == "No files changed in this session." {
		t.Error("expected changes to be listed")
	}
}

func TestSessionDiff_FormatDiff_Empty(t *testing.T) {
	d := NewSessionDiff("test-session")

	diff := d.FormatDiff()
	if diff != "No files changed in this session." {
		t.Errorf("expected 'No files changed', got %q", diff)
	}
}

func TestSessionDiff_GetChangesSince(t *testing.T) {
	d := NewSessionDiff("test-session")

	d.RecordChange("main.go", 10, 5)
	time.Sleep(10 * time.Millisecond)
	since := time.Now()
	time.Sleep(10 * time.Millisecond)
	d.RecordChange("utils.go", 20, 3)

	changes := d.GetChangesSince(since)
	if len(changes) != 1 {
		t.Errorf("expected 1 change since, got %d", len(changes))
	}
	if len(changes) > 0 && changes[0].Path != "utils.go" {
		t.Errorf("expected utils.go, got %s", changes[0].Path)
	}
}

func TestSessionDiff_Clear(t *testing.T) {
	d := NewSessionDiff("test-session")

	d.RecordChange("main.go", 10, 5)
	d.Clear()

	summary := d.GetSummary()
	if summary.FilesChanged != 0 {
		t.Errorf("expected 0 files after clear, got %d", summary.FilesChanged)
	}
}

func TestSessionDiff_GetSummary(t *testing.T) {
	d := NewSessionDiff("test-session")

	d.RecordChange("main.go", 10, 5)

	summary := d.GetSummary()
	if summary.SessionID != "test-session" {
		t.Errorf("expected test-session, got %s", summary.SessionID)
	}
	if summary.StartTime == "" {
		t.Error("expected non-empty start time")
	}
}
