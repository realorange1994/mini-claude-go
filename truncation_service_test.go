package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTruncationService_SmallOutput(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)

	result := s.Truncate("bash", "short output")
	if result.Truncated {
		t.Error("expected no truncation for small output")
	}
	if result.Output != "short output" {
		t.Errorf("expected 'short output', got %q", result.Output)
	}
}

func TestTruncationService_LargeOutput(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)
	s.maxChars = 100

	output := strings.Repeat("x", 200)
	result := s.Truncate("bash", output)

	if !result.Truncated {
		t.Error("expected truncation for large output")
	}
	if len(result.Output) >= 200 {
		t.Error("expected output to be shortened")
	}
	if result.SavedPath == "" {
		t.Error("expected output to be saved to disk")
	}
}

func TestTruncationService_ErrorAware(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)
	s.maxChars = 100

	// Create output with error in tail
	head := strings.Repeat("line\n", 20)
	tail := "error: something failed\n"
	output := head + tail + strings.Repeat("x", 100)

	result := s.Truncate("bash", output)

	if !result.HasError {
		t.Error("expected error detection")
	}
	if !strings.Contains(result.Output, "error context preserved") {
		t.Error("expected error context message")
	}
}

func TestTruncationService_Pressure(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)
	s.maxChars = 5000
	s.SetPressure(true)

	output := strings.Repeat("x", 8000)
	result := s.Truncate("bash", output)

	if !result.Truncated {
		t.Error("expected truncation under pressure")
	}
	// Pressure mode uses PressureMaxChars (4000)
}

func TestTruncationService_Direction(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)
	s.maxChars = 100

	output := strings.Repeat("x", 200)

	// Test tail direction
	s.SetDirection(TruncTail)
	result := s.Truncate("bash", output)
	if !strings.Contains(result.Output, "[... truncated ...]") {
		t.Error("expected truncation marker")
	}

	// Test head direction
	s.SetDirection(TruncHead)
	result = s.Truncate("bash", output)
	if !strings.Contains(result.Output, "[... truncated ...]") {
		t.Error("expected truncation marker")
	}
}

func TestTruncationService_SaveToDisk(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)
	s.maxChars = 100

	output := strings.Repeat("x", 200)
	result := s.Truncate("bash", output)

	if result.SavedPath == "" {
		t.Fatal("expected saved path")
	}

	// Verify file exists
	if _, err := os.Stat(result.SavedPath); os.IsNotExist(err) {
		t.Error("expected saved file to exist")
	}

	// Verify file content
	data, _ := os.ReadFile(result.SavedPath)
	if string(data) != output {
		t.Error("expected saved file to contain full output")
	}
}

func TestTruncationService_Hint(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)
	s.maxChars = 100

	output := strings.Repeat("x", 200)
	result := s.Truncate("bash", output)

	if result.Hint == "" {
		t.Error("expected hint")
	}
	if !strings.Contains(result.Hint, "Full output saved") {
		t.Error("expected hint to mention saved file")
	}
}

func TestTruncationService_Cleanup(t *testing.T) {
	dir := t.TempDir()
	s := NewTruncationService(dir)

	// Create old file
	oldPath := filepath.Join(dir, "old_file.txt")
	os.WriteFile(oldPath, []byte("old"), 0644)

	// Create new file
	newPath := filepath.Join(dir, "new_file.txt")
	os.WriteFile(newPath, []byte("new"), 0644)

	// Modify old file time
	oldTime := s.getModTime(oldPath)
	s.setModTime(oldPath, oldTime.AddDate(0, -1, 0))

	removed := s.CleanupOldOutputs()
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func (s *TruncationService) getModTime(path string) time.Time {
	info, _ := os.Stat(path)
	return info.ModTime()
}

func (s *TruncationService) setModTime(path string, t time.Time) {
	os.Chtimes(path, t, t)
}
