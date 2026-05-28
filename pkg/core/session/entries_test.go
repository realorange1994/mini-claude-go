package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSessionManagerV2(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "/tmp/sessions", "", true)
	if sm == nil {
		t.Fatal("sm should not be nil")
	}
	if sm.GetSessionID() == "" {
		t.Error("session ID should be set")
	}
}

func TestAppendMessage(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg := map[string]string{"role": "user", "content": "Hello"}
	data, _ := json.Marshal(msg)
	id := sm.AppendMessage(data)

	if id == "" {
		t.Error("should return entry ID")
	}
	if sm.GetLeafID() != id {
		t.Errorf("leaf should be %q, got %q", id, sm.GetLeafID())
	}
}

func TestModelChange(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg := map[string]string{"role": "user", "content": "Hi"}
	data, _ := json.Marshal(msg)
	sm.AppendMessage(data)

	id := sm.AppendModelChange("anthropic", "claude-sonnet-4-20250514")
	if id == "" {
		t.Error("should return entry ID")
	}

	ctx := sm.BuildSessionContext()
	if ctx.Model == nil {
		t.Fatal("model should be set")
	}
	if ctx.Model.ModelID != "claude-sonnet-4-20250514" {
		t.Errorf("ModelID = %q", ctx.Model.ModelID)
	}
}

func TestThinkingLevelChange(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg := map[string]string{"role": "user", "content": "Think"}
	data, _ := json.Marshal(msg)
	sm.AppendMessage(data)

	sm.AppendThinkingLevelChange("high")

	ctx := sm.BuildSessionContext()
	if ctx.ThinkingLevel != "high" {
		t.Errorf("ThinkingLevel = %q, want high", ctx.ThinkingLevel)
	}
}

func TestCompaction(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg1 := map[string]string{"role": "user", "content": "First"}
	data1, _ := json.Marshal(msg1)
	sm.AppendMessage(data1)

	msg2 := map[string]string{"role": "assistant", "content": "Response"}
	data2, _ := json.Marshal(msg2)
	id2 := sm.AppendMessage(data2)

	msg3 := map[string]string{"role": "user", "content": "Second"}
	data3, _ := json.Marshal(msg3)
	sm.AppendMessage(data3)

	// Compact — keep from id2 onward
	sm.AppendCompaction("Summary of earlier conversation", id2, 5000, nil, false)

	ctx := sm.BuildSessionContext()
	if len(ctx.Messages) == 0 {
		t.Error("should have messages after compaction")
	}
}

func TestLabelChange(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg := map[string]string{"role": "user", "content": "Hello"}
	data, _ := json.Marshal(msg)
	msgID := sm.AppendMessage(data)

	sm.AppendLabelChange(msgID, "important")
	label := sm.GetLabel(msgID)
	if label != "important" {
		t.Errorf("label = %q, want important", label)
	}

	// Clear label
	sm.AppendLabelChange(msgID, "")
	label = sm.GetLabel(msgID)
	if label != "" {
		t.Errorf("label should be cleared, got %q", label)
	}
}

func TestSessionInfo(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	sm.AppendSessionInfo("My Session")
	name := sm.GetSessionName()
	if name != "My Session" {
		t.Errorf("name = %q, want My Session", name)
	}

	// Update name
	sm.AppendSessionInfo("Updated Name")
	name = sm.GetSessionName()
	if name != "Updated Name" {
		t.Errorf("name = %q, want Updated Name", name)
	}
}

func TestBranchWithSummary(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg := map[string]string{"role": "user", "content": "Hello"}
	data, _ := json.Marshal(msg)
	msgID := sm.AppendMessage(data)

	newID := sm.BranchWithSummary(msgID, "Summary of conversation so far")
	if newID == "" {
		t.Error("should return new entry ID")
	}
	if sm.GetLeafID() != newID {
		t.Errorf("leaf should be %q", newID)
	}
}

func TestBranch(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg1 := map[string]string{"role": "user", "content": "First"}
	data1, _ := json.Marshal(msg1)
	id1 := sm.AppendMessage(data1)

	msg2 := map[string]string{"role": "user", "content": "Second"}
	data2, _ := json.Marshal(msg2)
	sm.AppendMessage(data2)

	// Branch back to first message
	err := sm.Branch(id1)
	if err != nil {
		t.Fatal(err)
	}
	if sm.GetLeafID() != id1 {
		t.Errorf("leaf should be %q after branch", id1)
	}
}

func TestGetBranch(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg1 := map[string]string{"role": "user", "content": "First"}
	data1, _ := json.Marshal(msg1)
	sm.AppendMessage(data1)

	msg2 := map[string]string{"role": "user", "content": "Second"}
	data2, _ := json.Marshal(msg2)
	sm.AppendMessage(data2)

	branch := sm.GetBranch("")
	if len(branch) != 2 {
		t.Errorf("branch should have 2 entries, got %d", len(branch))
	}
}

func TestGetTree(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg1 := map[string]string{"role": "user", "content": "First"}
	data1, _ := json.Marshal(msg1)
	sm.AppendMessage(data1)

	msg2 := map[string]string{"role": "user", "content": "Second"}
	data2, _ := json.Marshal(msg2)
	sm.AppendMessage(data2)

	tree := sm.GetTree()
	if len(tree) == 0 {
		t.Error("tree should not be empty")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	sessionDir := filepath.Join(tmp, "sessions")

	sm := NewSessionManagerV2("/tmp/test", sessionDir, "", true)

	msg := map[string]string{"role": "user", "content": "Hello"}
	data, _ := json.Marshal(msg)
	sm.AppendMessage(data)

	msg2 := map[string]string{"role": "assistant", "content": "Hi there!"}
	data2, _ := json.Marshal(msg2)
	sm.AppendMessage(data2)

	sm.AppendModelChange("anthropic", "claude-sonnet-4-20250514")

	// Verify file exists
	sessionFile := sm.GetSessionFile()
	if sessionFile == "" {
		t.Fatal("session file should be set")
	}

	// Verify file is not empty
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("session file should exist: %v", err)
	}

	// Load session from file
	sm2 := NewSessionManagerV2("/tmp/test", sessionDir, sessionFile, true)
	if sm2.GetSessionID() != sm.GetSessionID() {
		t.Errorf("session ID mismatch: %q vs %q", sm2.GetSessionID(), sm.GetSessionID())
	}

	entries := sm2.GetEntries()
	if len(entries) == 0 {
		t.Error("loaded session should have entries")
	}
}

func TestCustomEntry(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)

	msg := map[string]string{"role": "user", "content": "Hello"}
	data, _ := json.Marshal(msg)
	sm.AppendMessage(data)

	id := sm.AppendCustomEntry("diagnostic", map[string]interface{}{"key": "value"})
	if id == "" {
		t.Error("should return entry ID")
	}
}

func TestGetHeader(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)
	h := sm.GetHeader()
	if h == nil {
		t.Fatal("header should exist")
	}
	if h.Type != "session" {
		t.Errorf("header type = %q, want session", h.Type)
	}
	if h.Version != CurrentSessionVersion {
		t.Errorf("header version = %d, want %d", h.Version, CurrentSessionVersion)
	}
}

func TestBuildSessionContext_Empty(t *testing.T) {
	sm := NewSessionManagerV2("/tmp/test", "", "", false)
	ctx := sm.BuildSessionContext()
	if ctx.ThinkingLevel != "off" {
		t.Errorf("default thinking level should be off, got %q", ctx.ThinkingLevel)
	}
}

func TestParseFileEntry(t *testing.T) {
	tests := []struct {
		json string
		typeStr string
	}{
		{`{"type":"session","id":"s1","version":3,"timestamp":"2025-01-01T00:00:00Z"}`, "session"},
		{`{"type":"message","id":"e1","parentId":"","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"hi"}}`, "message"},
		{`{"type":"thinking_level_change","id":"e2","parentId":"e1","timestamp":"2025-01-01T00:00:00Z","thinkingLevel":"high"}`, "thinking_level_change"},
		{`{"type":"model_change","id":"e3","parentId":"e2","timestamp":"2025-01-01T00:00:00Z","provider":"anthropic","modelId":"sonnet"}`, "model_change"},
		{`{"type":"compaction","id":"e4","parentId":"e3","timestamp":"2025-01-01T00:00:00Z","summary":"compacted","firstKeptEntryId":"e2"}`, "compaction"},
		{`{"type":"branch_summary","id":"e5","parentId":"e3","timestamp":"2025-01-01T00:00:00Z","fromId":"e2","summary":"branched"}`, "branch_summary"},
		{`{"type":"custom","id":"e6","parentId":"e5","timestamp":"2025-01-01T00:00:00Z","customType":"test"}`, "custom"},
		{`{"type":"label","id":"e7","parentId":"e6","timestamp":"2025-01-01T00:00:00Z","targetId":"e1","label":"important"}`, "label"},
		{`{"type":"session_info","id":"e8","parentId":"e7","timestamp":"2025-01-01T00:00:00Z","name":"My Session"}`, "session_info"},
	}

	for _, tt := range tests {
		e, err := parseFileEntry([]byte(tt.json))
		if err != nil {
			t.Errorf("parse %s: %v", tt.typeStr, err)
			continue
		}
		if e.entryType() != tt.typeStr {
			t.Errorf("type = %q, want %q", e.entryType(), tt.typeStr)
		}
	}
}

func TestParseFileEntry_Unknown(t *testing.T) {
	_, err := parseFileEntry([]byte(`{"type":"unknown_type"}`))
	if err == nil {
		t.Error("should error for unknown type")
	}
}
