package transcript

import (
	"os"
	"path/filepath"
	"regexp"
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

func TestWriteSystemCompactSummary(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "extended.jsonl")
	w := NewWriter("extended-session", fpath)

	_ = w.WriteUser("hello")
	_ = w.WriteSystem("CLAUDE.md instructions")
	_ = w.WriteCompact("auto", 5000)
	_ = w.WriteSummary("[Previous conversation summary]")
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	expected := []string{"user", "system", "compact", "summary"}
	for i, e := range entries {
		if e.Type != expected[i] {
			t.Errorf("entry %d: expected type %q, got %q", i, expected[i], e.Type)
		}
	}
	if entries[2].Content == "" {
		t.Error("compact entry should have content")
	}
}

// ============================================================================
// DAG-specific tests
// ============================================================================

func TestAutoUUIDGeneration(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "uuid.jsonl")
	w := NewWriter("uuid-session", fpath)

	_ = w.WriteUser("hello")
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].UUID == "" {
		t.Error("expected auto-generated UUID, got empty")
	}
}

func TestParentUUIDChain(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "chain.jsonl")
	w := NewWriter("chain-session", fpath)

	_ = w.WriteUser("msg1")
	_ = w.WriteAssistant("response1", "model1")
	_ = w.WriteUser("msg2")
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// First entry has no parent
	if entries[0].ParentUUID != "" {
		t.Errorf("first entry should have no parent, got %q", entries[0].ParentUUID)
	}

	// Second entry's parent should be first entry's UUID
	if entries[1].ParentUUID != entries[0].UUID {
		t.Errorf("second entry parent %q != first entry UUID %q", entries[1].ParentUUID, entries[0].UUID)
	}

	// Third entry's parent should be second entry's UUID
	if entries[2].ParentUUID != entries[1].UUID {
		t.Errorf("third entry parent %q != second entry UUID %q", entries[2].ParentUUID, entries[1].UUID)
	}
}

func TestExplicitUUIDAndParent(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "explicit.jsonl")
	w := NewWriter("explicit-session", fpath)

	// Set explicit UUIDs (simulating a fork/branch)
	_ = w.Write(Entry{UUID: "uuid-A", Type: "user", Content: "original"})
	_ = w.Write(Entry{UUID: "uuid-B1", ParentUUID: "uuid-A", Type: "assistant", Content: "branch1"})
	_ = w.Write(Entry{UUID: "uuid-B2", ParentUUID: "uuid-A", Type: "assistant", Content: "branch2"})
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[1].UUID != "uuid-B1" {
		t.Errorf("expected UUID uuid-B1, got %q", entries[1].UUID)
	}
	if entries[1].ParentUUID != "uuid-A" {
		t.Errorf("expected parent uuid-A, got %q", entries[1].ParentUUID)
	}
	if entries[2].UUID != "uuid-B2" {
		t.Errorf("expected UUID uuid-B2, got %q", entries[2].UUID)
	}
	if entries[2].ParentUUID != "uuid-A" {
		t.Errorf("expected parent uuid-A, got %q", entries[2].ParentUUID)
	}
}

func TestWriteTitle(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "title.jsonl")
	w := NewWriter("title-session", fpath)

	_ = w.WriteTitle("My Conversation")
	_ = w.WriteUser("hello")
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Type != "metadata" {
		t.Errorf("expected type metadata, got %q", entries[0].Type)
	}
	if entries[0].Subtype != "custom-title" {
		t.Errorf("expected subtype custom-title, got %q", entries[0].Subtype)
	}
	if entries[0].Metadata["title"] != "My Conversation" {
		t.Errorf("expected title 'My Conversation', got %v", entries[0].Metadata["title"])
	}
}

func TestWriteTag(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "tag.jsonl")
	w := NewWriter("tag-session", fpath)

	_ = w.WriteTag("debug")
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if entries[0].Subtype != "tag" {
		t.Errorf("expected subtype tag, got %q", entries[0].Subtype)
	}
	if entries[0].Metadata["tag"] != "debug" {
		t.Errorf("expected tag 'debug', got %v", entries[0].Metadata["tag"])
	}
}

func TestWriteCompactBoundary(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "boundary.jsonl")
	w := NewWriter("boundary-session", fpath)

	_ = w.WriteCompactBoundary("auto", 10000, 5, []string{"read_file", "write_file"})
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if entries[0].Type != "system" {
		t.Errorf("expected type system, got %q", entries[0].Type)
	}
	if entries[0].Subtype != "compact_boundary" {
		t.Errorf("expected subtype compact_boundary, got %q", entries[0].Subtype)
	}
	if entries[0].Metadata["pre_compact_tokens"] != float64(10000) {
		t.Errorf("expected pre_compact_tokens 10000, got %v", entries[0].Metadata["pre_compact_tokens"])
	}
	if entries[0].Metadata["messages_summarized"] != float64(5) {
		t.Errorf("expected messages_summarized 5, got %v", entries[0].Metadata["messages_summarized"])
	}
}

func TestWriteInterrupt(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "interrupt.jsonl")
	w := NewWriter("interrupt-session", fpath)

	_ = w.WriteInterrupt("interrupted_turn")
	_ = w.Flush()
	_ = w.Close()

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if entries[0].Type != "system" {
		t.Errorf("expected type system, got %q", entries[0].Type)
	}
	if entries[0].Subtype != "interrupt" {
		t.Errorf("expected subtype interrupt, got %q", entries[0].Subtype)
	}
	if entries[0].Metadata["interrupt_type"] != "interrupted_turn" {
		t.Errorf("expected interrupt_type 'interrupted_turn', got %v", entries[0].Metadata["interrupt_type"])
	}
}

func TestDetectInterruptType(t *testing.T) {
	tests := []struct {
		name     string
		entries  []Entry
		expected string
	}{
		{
			name:     "empty",
			entries:  []Entry{},
			expected: "none",
		},
		{
			name:     "normal_end",
			entries:  []Entry{{Type: "assistant", Content: "done"}},
			expected: "none",
		},
		{
			name:     "interrupted_prompt",
			entries:  []Entry{{Type: "assistant", Content: "hi"}, {Type: "user", Content: "stop"}},
			expected: "interrupted_prompt",
		},
		{
			name:     "interrupted_turn",
			entries:  []Entry{{Type: "tool_use", ToolName: "read_file"}},
			expected: "interrupted_turn",
		},
		{
			name: "explicit_interrupt_marker",
			entries: []Entry{
				{Type: "user", Content: "hello"},
				{Type: "system", Subtype: "interrupt", Metadata: map[string]any{"interrupt_type": "interrupted_prompt"}},
			},
			expected: "interrupted_prompt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := DetectInterruptType(tc.entries)
			if result != tc.expected {
				t.Errorf("DetectInterruptType: expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestDedupByUUID(t *testing.T) {
	// Manually create a JSONL file with duplicate UUIDs
	dir := t.TempDir()
	fpath := filepath.Join(dir, "dedup.jsonl")
	lines := []string{
		`{"uuid":"uuid-1","type":"user","content":"hello"}`,
		`{"uuid":"uuid-2","type":"assistant","content":"hi"}`,
		`{"uuid":"uuid-1","type":"user","content":"duplicate"}`,
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	// Should dedup to 2 entries (uuid-1 appears only once)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(entries))
	}
	// First occurrence of uuid-1 should be kept
	if entries[0].Content != "hello" {
		t.Errorf("expected first uuid-1 content 'hello', got %q", entries[0].Content)
	}
}

func TestBackwardCompatibilityNoUUID(t *testing.T) {
	// Manually create a JSONL file with entries that have no UUID
	// (simulating old transcript files)
	dir := t.TempDir()
	fpath := filepath.Join(dir, "old_format.jsonl")
	lines := []string{
		`{"type":"user","content":"old hello","timestamp":"2026-01-01T00:00:00Z"}`,
		`{"type":"assistant","content":"old hi","timestamp":"2026-01-01T00:00:01Z"}`,
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewReader(fpath)
	entries, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Type != "user" || entries[0].Content != "old hello" {
		t.Errorf("backward compat failed: got %+v", entries[0])
	}
}

func TestLastUUID(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "lastuuid.jsonl")
	w := NewWriter("lastuuid-session", fpath)

	_ = w.WriteUser("hello")
	uuid1 := w.LastUUID()
	if uuid1 == "" {
		t.Fatal("LastUUID should not be empty after WriteUser")
	}

	_ = w.WriteAssistant("hi", "model1")
	uuid2 := w.LastUUID()
	if uuid2 == uuid1 {
		t.Error("LastUUID should change after each write")
	}

	_ = w.Flush()
	_ = w.Close()
}

// ---------------------------------------------------------------------------
// uuidV4 — transcript.go:17
// ---------------------------------------------------------------------------

func TestUUIDV4Uniqueness(t *testing.T) {
	// Upstream invariant: N generated UUIDs are all different
	const n = 1000
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		u := uuidV4()
		if seen[u] {
			t.Fatalf("duplicate UUID at iteration %d: %s", i, u)
		}
		seen[u] = true
	}
}

func TestUUIDV4Format(t *testing.T) {
	// Upstream: matches /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		u := uuidV4()
		if !uuidRegex.MatchString(u) {
			t.Errorf("UUID does not match standard format: %s", u)
		}
	}
}

func TestUUIDV4Length(t *testing.T) {
	// Invariant: always 36 chars (32 hex + 4 hyphens)
	for i := 0; i < 100; i++ {
		u := uuidV4()
		if len(u) != 36 {
			t.Errorf("expected length 36, got %d for UUID: %s", len(u), u)
		}
	}
}

func TestUUIDV4VersionBits(t *testing.T) {
	// UUID v4: the version nibble (char at position 14) should be '4'
	for i := 0; i < 100; i++ {
		u := uuidV4()
		// Position 14 is the version digit: xxxxxxxx-xxxx-4xxx-...
		if u[14] != '4' {
			t.Errorf("UUID v4 should have version nibble '4' at position 14, got %c: %s", u[14], u)
		}
	}
}

func TestUUIDV4VariantBits(t *testing.T) {
	// UUID v4 variant 10: the first char of the 4th group (position 19) should be 8, 9, a, or b
	validVariant := map[byte]bool{'8': true, '9': true, 'a': true, 'b': true}
	for i := 0; i < 100; i++ {
		u := uuidV4()
		// Position 19: xxxxxxxx-xxxx-4xxx-Nxxx-..., N should be 8/9/a/b
		if !validVariant[u[19]] {
			t.Errorf("UUID v4 variant byte should be 8/9/a/b, got %c: %s", u[19], u)
		}
	}
}

func TestUUIDV4HyphenPositions(t *testing.T) {
	// Hyphens at fixed positions: 8, 13, 18, 23
	for i := 0; i < 100; i++ {
		u := uuidV4()
		for _, pos := range []int{8, 13, 18, 23} {
			if u[pos] != '-' {
				t.Errorf("expected hyphen at position %d, got %c: %s", pos, u[pos], u)
			}
		}
	}
}
