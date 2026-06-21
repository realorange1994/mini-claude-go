package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── Workspace Trust Tests ──────────────────────────────────────────────────

func TestWorkspaceTrust_New(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceTrust(dir)
	if w == nil {
		t.Error("expected non-nil trust")
	}
}

func TestWorkspaceTrust_DangerousPaths(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceTrust(dir)

	// Test platform-specific dangerous paths
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" && !w.IsDangerous(homeDir) {
		t.Error("expected home dir to be dangerous")
	}
}

func TestWorkspaceTrust_TrustedPaths(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceTrust(dir)

	// Mark as trusted
	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	w.MarkTrusted(projectDir)

	if !w.IsTrusted(projectDir) {
		t.Error("expected project dir to be trusted")
	}
}

func TestWorkspaceTrust_RevokeTrust(t *testing.T) {
	dir := t.TempDir()
	w := NewWorkspaceTrust(dir)

	projectDir := filepath.Join(dir, "project")
	os.MkdirAll(projectDir, 0755)
	w.MarkTrusted(projectDir)
	w.RevokeTrust(projectDir)

	if w.IsTrusted(projectDir) {
		t.Error("expected trust to be revoked")
	}
}

// ─── Protected Paths Tests ──────────────────────────────────────────────────

func TestProtectedPaths_New(t *testing.T) {
	p := NewProtectedPaths()
	if p == nil {
		t.Error("expected non-nil paths")
	}
}

func TestProtectedPaths_GetProtected(t *testing.T) {
	p := NewProtectedPaths()
	paths := p.GetProtected()
	if len(paths) == 0 {
		t.Error("expected non-empty protected paths")
	}
}

// ─── Tail Turn Budget Tests ─────────────────────────────────────────────────

func TestSelectTailTurns_Basic(t *testing.T) {
	messages := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "how are you?"},
		{Role: "assistant", Content: "good"},
	}

	result := SelectTailTurns(messages, 10000, 2)
	if len(result.TailMessages) == 0 {
		t.Error("expected non-empty tail")
	}
}

func TestSelectTailTurns_SmallBudget(t *testing.T) {
	messages := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	result := SelectTailTurns(messages, 100, 1)
	if len(result.TailMessages) == 0 {
		t.Error("expected non-empty tail")
	}
}

// ─── Fork Context Cache Tests ───────────────────────────────────────────────

func TestForkContextCache_New(t *testing.T) {
	c := NewForkContextCache()
	if c == nil {
		t.Error("expected non-nil cache")
	}
}

func TestForkContextCache_CaptureGet(t *testing.T) {
	c := NewForkContextCache()

	ctx := c.Capture("agent-1", "system prompt", []string{"tool1"}, []string{"msg1"})
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	got := c.Get("agent-1")
	if got == nil {
		t.Fatal("expected cached context")
	}
	if got.Hash != ctx.Hash {
		t.Errorf("expected hash %s, got %s", ctx.Hash, got.Hash)
	}
}

func TestForkContextCache_Clear(t *testing.T) {
	c := NewForkContextCache()

	c.Capture("agent-1", "system", nil, nil)
	c.Clear("agent-1")

	if c.Get("agent-1") != nil {
		t.Error("expected nil after clear")
	}
}

// ─── Progress Checker Tests ─────────────────────────────────────────────────

func TestProgressChecker_New(t *testing.T) {
	dir := t.TempDir()
	c := NewProgressChecker(dir)
	if c == nil {
		t.Error("expected non-nil checker")
	}
}

func TestProgressChecker_ValidateProgress_NoFile(t *testing.T) {
	dir := t.TempDir()
	c := NewProgressChecker(dir)

	v, err := c.ValidateProgress("T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Valid {
		t.Error("expected invalid for missing file")
	}
}

func TestProgressChecker_WriteProgressTemplate(t *testing.T) {
	dir := t.TempDir()
	c := NewProgressChecker(dir)

	err := c.WriteProgressTemplate("T1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "T1", "progress.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected progress file to exist")
	}
}

// ─── History FTS Tests ──────────────────────────────────────────────────────

func TestHistoryService_New(t *testing.T) {
	h := NewHistoryService()
	if h == nil {
		t.Error("expected non-nil service")
	}
}

func TestHistoryService_Index(t *testing.T) {
	h := NewHistoryService()

	h.Index(HistoryEntry{
		ID:        "entry-1",
		SessionID: "session-1",
		Role:      "user",
		Content:   "hello world",
	})

	if h.Count() != 1 {
		t.Errorf("expected 1 entry, got %d", h.Count())
	}
}

func TestHistoryService_Search(t *testing.T) {
	h := NewHistoryService()

	h.Index(HistoryEntry{ID: "1", SessionID: "s1", Role: "user", Content: "hello world"})
	h.Index(HistoryEntry{ID: "2", SessionID: "s1", Role: "user", Content: "goodbye world"})

	results := h.Search("hello", 10)
	if len(results) == 0 {
		t.Error("expected search results")
	}
}

func TestHistoryService_Around(t *testing.T) {
	h := NewHistoryService()

	h.Index(HistoryEntry{ID: "1", SessionID: "s1", Role: "user", Content: "msg1", Timestamp: time.Now()})
	h.Index(HistoryEntry{ID: "2", SessionID: "s1", Role: "assistant", Content: "msg2", Timestamp: time.Now()})
	h.Index(HistoryEntry{ID: "3", SessionID: "s1", Role: "user", Content: "msg3", Timestamp: time.Now()})

	around := h.Around("2", 1, 1)
	if len(around) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(around))
	}
}

// ─── Event Store Tests ──────────────────────────────────────────────────────

func TestEventStore_New(t *testing.T) {
	s := NewEventStore()
	if s == nil {
		t.Error("expected non-nil store")
	}
}

func TestEventStore_Append(t *testing.T) {
	s := NewEventStore()

	s.Append(Event{
		Type:        EventSessionCreated,
		AggregateID: "session-1",
	})

	if s.Count() != 1 {
		t.Errorf("expected 1 event, got %d", s.Count())
	}
}

func TestEventStore_GetEvents(t *testing.T) {
	s := NewEventStore()

	s.Append(Event{Type: EventSessionCreated, AggregateID: "s1"})
	s.Append(Event{Type: EventMessageAdded, AggregateID: "s1"})
	s.Append(Event{Type: EventSessionCreated, AggregateID: "s2"})

	events := s.GetEvents("s1")
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestEventStore_Replay(t *testing.T) {
	s := NewEventStore()

	s.Append(Event{Type: EventSessionCreated, AggregateID: "s1"})
	s.Append(Event{Type: EventMessageAdded, AggregateID: "s1"})

	count := 0
	err := s.Replay("s1", ProjectorFunc(func(e Event) error {
		count++
		return nil
	}))
	if err != nil {
		t.Fatalf("replay error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 replays, got %d", count)
	}
}

// ProjectorFunc is a function that implements Projector.
type ProjectorFunc func(Event) error

func (f ProjectorFunc) Apply(event Event) error {
	return f(event)
}
