package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── CircularBuffer ─────────────────────────────────────────────────────────

func TestCircularBufferAppendAndGet(t *testing.T) {
	cb := NewCircularBuffer(5)
	cb.Append("a")
	cb.Append("b")
	cb.Append("c")

	all := cb.GetAll()
	if len(all) != 3 {
		t.Errorf("expected 3 entries, got %d", len(all))
	}
	if all[0] != "a" || all[1] != "b" || all[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", all)
	}
}

func TestCircularBufferOverflow(t *testing.T) {
	cb := NewCircularBuffer(3)
	cb.Append("a")
	cb.Append("b")
	cb.Append("c")
	cb.Append("d") // overwrites "a"

	all := cb.GetAll()
	if len(all) != 3 {
		t.Errorf("expected 3 entries, got %d", len(all))
	}
	if all[0] != "b" || all[1] != "c" || all[2] != "d" {
		t.Errorf("expected [b, c, d], got %v", all)
	}
}

func TestCircularBufferTail(t *testing.T) {
	cb := NewCircularBuffer(10)
	for i := 0; i < 8; i++ {
		cb.Append(string(rune('a' + i)))
	}

	tail := cb.Tail(3)
	if len(tail) != 3 {
		t.Errorf("expected 3 tail entries, got %d", len(tail))
	}
	if tail[0] != "f" || tail[1] != "g" || tail[2] != "h" {
		t.Errorf("expected [f, g, h], got %v", tail)
	}
}

func TestCircularBufferTailOverflow(t *testing.T) {
	cb := NewCircularBuffer(5)
	cb.Append("1")
	cb.Append("2")
	cb.Append("3")

	tail := cb.Tail(10) // request more than exists
	if len(tail) != 3 {
		t.Errorf("expected 3 entries (all available), got %d", len(tail))
	}
}

func TestCircularBufferLen(t *testing.T) {
	cb := NewCircularBuffer(3)
	if cb.Len() != 0 {
		t.Error("expected 0 for empty buffer")
	}
	cb.Append("x")
	cb.Append("y")
	if cb.Len() != 2 {
		t.Errorf("expected 2, got %d", cb.Len())
	}
}

func TestCircularBufferAppendMany(t *testing.T) {
	cb := NewCircularBuffer(10)
	cb.AppendMany([]string{"a", "b", "c"})
	if cb.Len() != 3 {
		t.Errorf("expected 3 after AppendMany, got %d", cb.Len())
	}
}

func TestCircularBufferEmpty(t *testing.T) {
	cb := NewCircularBuffer(5)
	if len(cb.GetAll()) != 0 {
		t.Error("expected empty slice for empty buffer")
	}
	if len(cb.Tail(5)) != 0 {
		t.Error("expected empty tail for empty buffer")
	}
}

// ─── BgTaskOutput Pipe Mode ─────────────────────────────────────────────────

func TestBgTaskOutputPipeModeWrite(t *testing.T) {
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "test1",
		FileMode: false,
	})
	defer b.Close()

	b.WriteStdout("line1\nline2\nline3\n")

	if b.GetStdout() == "" {
		t.Error("expected non-empty output")
	}
	if !strings.Contains(b.GetStdout(), "line1") {
		t.Error("expected 'line1' in output")
	}
}

func TestBgTaskOutputPipeModeStderr(t *testing.T) {
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "test2",
		FileMode: false,
	})
	defer b.Close()

	b.WriteStderr("error message")
	output := b.GetStdout()
	if !strings.Contains(output, "STDERR") {
		t.Error("expected 'STDERR' prefix in output")
	}
	if !strings.Contains(output, "error message") {
		t.Error("expected error text in output")
	}
}

func TestBgTaskOutputPipeModeTail(t *testing.T) {
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "test3",
		FileMode: false,
	})
	defer b.Close()

	for i := 0; i < 20; i++ {
		b.WriteStdout("line\n")
	}

	tail := b.Tail(5)
	if len(tail) != 5 {
		t.Errorf("expected 5 tail lines, got %d", len(tail))
	}
}

func TestBgTaskOutputPipeModeProgress(t *testing.T) {
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "test4",
		FileMode: false,
	})
	defer b.Close()

	b.WriteStdout("line1\nline2\nline3\n")
	b.UpdateProgress("processing", 100, 5)

	progress := b.GetProgress()
	if progress.TotalLines != 3 {
		t.Errorf("expected 3 total lines, got %d", progress.TotalLines)
	}
	if progress.Description != "processing" {
		t.Errorf("expected description 'processing', got %q", progress.Description)
	}
	if progress.TokenCount != 100 {
		t.Errorf("expected 100 tokens, got %d", progress.TokenCount)
	}
	if progress.ToolUseCount != 5 {
		t.Errorf("expected 5 tool uses, got %d", progress.ToolUseCount)
	}
}

func TestBgTaskOutputSetComplete(t *testing.T) {
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "test5",
		FileMode: false,
	})
	defer b.Close()

	b.WriteStdout("output\n")
	b.SetComplete(0)

	if !b.IsComplete() {
		t.Error("expected IsComplete()=true after SetComplete")
	}
	if b.GetExitCode() != 0 {
		t.Errorf("expected exit code 0, got %d", b.GetExitCode())
	}

	progress := b.GetProgress()
	if !progress.IsComplete {
		t.Error("expected progress.IsComplete=true")
	}
	if progress.ExitCode != 0 {
		t.Errorf("expected progress.ExitCode=0, got %d", progress.ExitCode)
	}
}

// ─── BgTaskOutput File Mode ─────────────────────────────────────────────────

func TestBgTaskOutputFileModeWrite(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "output.txt")

	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:     "test6",
		OutputPath: outPath,
		FileMode:   true,
	})
	defer b.Close()

	b.WriteStdout("file content\n")
	b.SetComplete(0)

	// Read the file directly
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "file content") {
		t.Error("expected 'file content' in output file")
	}
}

func TestBgTaskOutputFileModeTail(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "output.txt")

	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:     "test7",
		OutputPath: outPath,
		FileMode:   true,
	})
	defer b.Close()

	b.WriteStdout("a\nb\nc\nd\ne\n")
	b.SetComplete(0)

	tail := b.Tail(3)
	if len(tail) != 3 {
		t.Errorf("expected 3 tail lines, got %d", len(tail))
	}
}

func TestBgTaskOutputFileModeProgress(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "output.txt")

	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:     "test8",
		OutputPath: outPath,
		FileMode:   true,
	})
	defer b.Close()

	b.WriteStdout("line1\nline2\n")

	progress := b.GetProgress()
	if progress.TotalLines != 2 {
		t.Errorf("expected 2 total lines, got %d", progress.TotalLines)
	}
}

// ─── BgTaskOutputStore ──────────────────────────────────────────────────────

func TestBgTaskOutputStoreRegisterAndGet(t *testing.T) {
	store := NewBgTaskOutputStore()
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "stored1",
		FileMode: false,
	})

	store.Register(b)
	got := store.Get("stored1")
	if got == nil {
		t.Fatal("expected to find registered task")
	}
	if got.GetTaskID() != "stored1" {
		t.Errorf("expected task ID 'stored1', got %q", got.GetTaskID())
	}
}

func TestBgTaskOutputStoreRemove(t *testing.T) {
	store := NewBgTaskOutputStore()
	b := NewBgTaskOutput(BgTaskOutputConfig{
		TaskID:   "remove1",
		FileMode: false,
	})

	store.Register(b)
	store.Remove("remove1")

	got := store.Get("remove1")
	if got != nil {
		t.Error("expected nil after removal")
	}
}

func TestBgTaskOutputStoreList(t *testing.T) {
	store := NewBgTaskOutputStore()
	store.Register(NewBgTaskOutput(BgTaskOutputConfig{TaskID: "a", FileMode: false}))
	store.Register(NewBgTaskOutput(BgTaskOutputConfig{TaskID: "b", FileMode: false}))

	ids := store.List()
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}
}

func TestBgTaskOutputStoreGetNotFound(t *testing.T) {
	store := NewBgTaskOutputStore()
	got := store.Get("nonexistent")
	if got != nil {
		t.Error("expected nil for non-existent task")
	}
}

// ─── Utility Functions ──────────────────────────────────────────────────────

func TestGenerateOutputPath(t *testing.T) {
	path := GenerateOutputPath("/tmp/base", "task123")
	expected := filepath.FromSlash("/tmp/base/.claude/task-outputs/task123.txt")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestReadFileTail(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "tail.txt")
	os.WriteFile(fp, []byte("line1\nline2\nline3\nline4\nline5\n"), 0o644)

	tail, err := ReadFileTail(fp, 20)
	if err != nil {
		t.Fatal(err)
	}
	if tail == "" {
		t.Error("expected non-empty tail")
	}
}

func TestReadFileTailEmpty(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "empty.txt")
	os.WriteFile(fp, []byte{}, 0o644)

	tail, err := ReadFileTail(fp, 100)
	if err != nil {
		t.Fatal(err)
	}
	if tail != "" {
		t.Errorf("expected empty tail, got %q", tail)
	}
}

func TestCountFileLines(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "lines.txt")
	os.WriteFile(fp, []byte("a\nb\nc\n"), 0o644)

	count, err := CountFileLines(fp)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 lines, got %d", count)
	}
}

func TestCountFileLinesNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "noterm.txt")
	os.WriteFile(fp, []byte("a\nb\nc"), 0o644) // no trailing newline

	count, err := CountFileLines(fp)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 lines (last without newline), got %d", count)
	}
}

func TestCountFileLinesEmpty(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "empty.txt")
	os.WriteFile(fp, []byte{}, 0o644)

	count, err := CountFileLines(fp)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 lines for empty file, got %d", count)
	}
}
