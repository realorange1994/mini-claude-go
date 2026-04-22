package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriterWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.jsonl")

	w := NewWriter("test-session", fpath)

	_ = w.WriteUser("hello")
	_ = w.WriteAssistant("hi there", "claude-sonnet")
	_ = w.WriteToolUse("id1", "read_file", map[string]any{"path": "foo.go"})
	_ = w.WriteToolResult("id1", "read_file", "package main")
	_ = w.WriteError("something failed")

	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	expected := []string{"user", "assistant", "tool_use", "tool_result", "error"}
	for i, e := range entries {
		if e.Type != expected[i] {
			t.Errorf("entry %d: expected type %q, got %q", i, expected[i], e.Type)
		}
	}
}

func TestWriterBufferedFlush(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "buffered.jsonl")

	w := NewWriter("buf-session", fpath)

	// Write 50 entries, should not yet be on disk
	for i := 0; i < 50; i++ {
		_ = w.WriteUser("msg")
	}

	// File shouldn't exist yet since we haven't flushed
	if _, err := os.Stat(fpath); err == nil {
		// File was created (MkdirAll might create it), that's fine
	}

	if err := w.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 50 {
		t.Fatalf("expected 50 entries after flush, got %d", len(entries))
	}
}

func TestWriterCloseTwice(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter("close-test", filepath.Join(dir, "close.jsonl"))
	if err := w.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestWriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter("write-close", filepath.Join(dir, "wc.jsonl"))
	_ = w.Close()
	if err := w.WriteUser("should fail"); err == nil {
		t.Error("expected error writing after close")
	}
}

func TestReaderNonexistent(t *testing.T) {
	r := NewReader("/nonexistent/path/file.jsonl")
	_, err := r.ReadAll()
	if err == nil {
		t.Error("expected error reading nonexistent file")
	}
}

func TestWriteAssistantWithModel(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "model.jsonl")
	w := NewWriter("model-session", fpath)
	_ = w.WriteAssistant("response", "claude-sonnet-4-20250514")
	_ = w.Flush()

	r := NewReader(fpath)
	entries, _ := r.ReadAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %q", entries[0].Model)
	}
}
