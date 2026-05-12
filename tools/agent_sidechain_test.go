package tools

import (
	"strings"
	"testing"
)

// ─── SidechainTranscript ───────────────────────────────────────────────────

func TestNewSidechainTranscript(t *testing.T) {
	sc := NewSidechainTranscript("parent-uuid-123", "/tmp/sessions", "worker")
	if sc.ParentUUID != "parent-uuid-123" {
		t.Errorf("expected ParentUUID 'parent-uuid-123', got %s", sc.ParentUUID)
	}
	if !strings.Contains(sc.Path, "sidechain-worker-") {
		t.Errorf("Path should contain 'sidechain-worker-', got %s", sc.Path)
	}
	if !strings.Contains(sc.Path, ".jsonl") {
		t.Errorf("Path should contain '.jsonl', got %s", sc.Path)
	}
	if !strings.Contains(sc.Path, "sessions") {
		t.Errorf("Path should contain session dir, got %s", sc.Path)
	}
}

func TestSidechainTranscriptWriterNil(t *testing.T) {
	sc := NewSidechainTranscript("p1", "/tmp", "test")
	if sc.Writer() != nil {
		t.Error("Writer should be nil before InitWriter")
	}
}

func TestSidechainTranscriptRecordBeforeInit(t *testing.T) {
	sc := NewSidechainTranscript("p1", "/tmp", "test")
	// These should all be no-ops (not panic)
	if err := sc.RecordToolUse("t1", "bash", nil); err != nil {
		t.Errorf("RecordToolUse before init should return nil, got: %v", err)
	}
	if err := sc.RecordToolResult("t1", "bash", "output"); err != nil {
		t.Errorf("RecordToolResult before init should return nil, got: %v", err)
	}
	if err := sc.RecordSystem("system msg"); err != nil {
		t.Errorf("RecordSystem before init should return nil, got: %v", err)
	}
	if err := sc.RecordError("error msg"); err != nil {
		t.Errorf("RecordError before init should return nil, got: %v", err)
	}
}

// mockWriter is a test implementation of writerIface
type mockWriter struct {
	messages []string
}

func (m *mockWriter) WriteUser(content string) error {
	m.messages = append(m.messages, "user:"+content)
	return nil
}
func (m *mockWriter) WriteAssistant(content, model string) error {
	m.messages = append(m.messages, "assistant:"+content)
	return nil
}
func (m *mockWriter) WriteToolUse(toolID, toolName string, args map[string]any) error {
	m.messages = append(m.messages, "tool_use:"+toolName)
	return nil
}
func (m *mockWriter) WriteToolResult(toolID, toolName, result string) error {
	m.messages = append(m.messages, "tool_result:"+toolName)
	return nil
}
func (m *mockWriter) WriteError(err string) error {
	m.messages = append(m.messages, "error:"+err)
	return nil
}
func (m *mockWriter) WriteSystem(content string) error {
	m.messages = append(m.messages, "system:"+content)
	return nil
}
func (m *mockWriter) FilePath() string {
	return "/mock/path.jsonl"
}

func TestSidechainTranscriptWithMockWriter(t *testing.T) {
	sc := NewSidechainTranscript("p1", "/tmp", "test")
	mw := &mockWriter{}
	sc.InitWriter(func(sessionID, filePath string) writerIface {
		return mw
	})

	if sc.Writer() == nil {
		t.Fatal("Writer should not be nil after InitWriter")
	}

	sc.RecordToolUse("t1", "bash", nil)
	sc.RecordToolResult("t1", "bash", "output")
	sc.RecordSystem("init")
	sc.RecordError("oops")

	if len(mw.messages) != 4 {
		t.Errorf("expected 4 messages, got %d: %v", len(mw.messages), mw.messages)
	}
	if mw.messages[0] != "tool_use:bash" {
		t.Errorf("expected 'tool_use:bash', got %s", mw.messages[0])
	}
	if mw.messages[1] != "tool_result:bash" {
		t.Errorf("expected 'tool_result:bash', got %s", mw.messages[1])
	}
	if mw.messages[2] != "system:init" {
		t.Errorf("expected 'system:init', got %s", mw.messages[2])
	}
	if mw.messages[3] != "error:oops" {
		t.Errorf("expected 'error:oops', got %s", mw.messages[3])
	}
}

func TestSidechainTranscriptInitWriterIdempotent(t *testing.T) {
	sc := NewSidechainTranscript("p1", "/tmp", "test")
	mw1 := &mockWriter{}
	mw2 := &mockWriter{}

	sc.InitWriter(func(sessionID, filePath string) writerIface {
		return mw1
	})
	sc.InitWriter(func(sessionID, filePath string) writerIface {
		return mw2
	})

	// Should still use the first writer
	sc.RecordSystem("test")
	if len(mw1.messages) != 1 {
		t.Error("should use first writer")
	}
	if len(mw2.messages) != 0 {
		t.Error("should NOT use second writer (InitWriter is idempotent)")
	}
}
