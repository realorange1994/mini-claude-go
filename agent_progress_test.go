package main

import (
	"strings"
	"testing"
)

// ─── subAgentProgressWriter ──────────────────────────────────────────────────

func TestSubAgentProgressWriterWrite(t *testing.T) {
	var lines []string
	cb := func(s string) { lines = append(lines, s) }
	w := NewSubAgentProgressWriter("test-agent", cb)

	n, err := w.Write([]byte("[+] exec: reading file\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == 0 {
		t.Error("Write should return non-zero byte count")
	}
}

func TestSubAgentProgressWriterSuppressThink(t *testing.T) {
	var lines []string
	cb := func(s string) { lines = append(lines, s) }
	w := NewSubAgentProgressWriter("test-agent", cb)

	w.Write([]byte("[THINK] analyzing the code\n"))
	w.Flush()

	// [THINK] lines should be suppressed entirely
	for _, l := range lines {
		if strings.Contains(l, "analyzing the code") {
			t.Error("THINK line content should not appear in output")
		}
	}
}

func TestSubAgentProgressWriterExtractToolName(t *testing.T) {
	var lines []string
	cb := func(s string) { lines = append(lines, s) }
	w := NewSubAgentProgressWriter("test-agent", cb)

	w.Write([]byte("[+] exec: reading file\n"))
	w.Flush()

	if w.toolCount != 1 {
		t.Errorf("expected 1 tool call, got %d", w.toolCount)
	}
	if w.lastToolName != "exec" {
		t.Errorf("expected tool name 'exec', got %q", w.lastToolName)
	}
}

func TestSubAgentProgressWriterErrLine(t *testing.T) {
	w := NewSubAgentProgressWriter("test-agent", func(string) {})

	w.Write([]byte("[ERR] file_read: permission denied\n"))
	w.Flush()

	if w.toolCount != 1 {
		t.Errorf("expected 1 tool call from ERR line, got %d", w.toolCount)
	}
	if w.lastToolName != "file_read" {
		t.Errorf("expected tool name 'file_read', got %q", w.lastToolName)
	}
}

func TestSubAgentProgressWriterMultipleTools(t *testing.T) {
	w := NewSubAgentProgressWriter("test-agent", func(string) {})

	w.Write([]byte("[+] exec: ls\n[+] grep: searching\n"))
	w.Flush()

	if w.toolCount != 2 {
		t.Errorf("expected 2 tool calls, got %d", w.toolCount)
	}
}

func TestSubAgentProgressWriterGetStats(t *testing.T) {
	w := NewSubAgentProgressWriter("test-agent", func(string) {})
	w.UpdateTokens(5000)
	w.SetToolCount(3)

	toolCount, totalTokens := w.GetStats()
	if toolCount != 3 {
		t.Errorf("expected 3 tool calls, got %d", toolCount)
	}
	if totalTokens != 5000 {
		t.Errorf("expected 5000 tokens, got %d", totalTokens)
	}
}

func TestSubAgentProgressWriterProgressLineFormat(t *testing.T) {
	var output string
	cb := func(s string) { output = s }
	w := NewSubAgentProgressWriter("my-agent", cb)

	w.Write([]byte("[+] exec: running\n"))

	if !strings.Contains(output, "[agent: my-agent]") {
		t.Error("progress line should contain agent description")
	}
	if !strings.Contains(output, "Running exec") {
		t.Error("progress line should contain 'Running exec'")
	}
	if !strings.Contains(output, "1 tool uses") {
		t.Error("progress line should contain tool count")
	}
}

func TestSubAgentProgressWriterNilCallback(t *testing.T) {
	w := NewSubAgentProgressWriter("test-agent", nil)
	w.Write([]byte("[+] exec: running\n"))
	w.Flush()
	// Should not panic
}

func TestSubAgentProgressWriterImplementsWriter(t *testing.T) {
	var _ interface{ Write([]byte) (int, error) } = NewSubAgentProgressWriter("test", nil)
}

func TestSubAgentProgressWriterUpdateTokens(t *testing.T) {
	var output string
	cb := func(s string) { output = s }
	w := NewSubAgentProgressWriter("test-agent", cb)

	w.UpdateTokens(10000)

	if !strings.Contains(output, "10000 tokens") {
		t.Error("progress line should contain token count after UpdateTokens")
	}
}

// ─── extractToolName ─────────────────────────────────────────────────────────

func TestExtractToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"exec: reading file", "exec"},
		{"file_read", "file_read"},
		{"edit_file: /path/to/file.go", "edit_file"},
		{"  write_file  ", "write_file"},
		{"", ""},
		{"exec", "exec"},
	}
	for _, tt := range tests {
		got := extractToolName(tt.input)
		if got != tt.want {
			t.Errorf("extractToolName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── stripAnsi ───────────────────────────────────────────────────────────────

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\x1b[31mred\x1b[0m", "red"},
		{"plain text", "plain text"},
		{"\x1b[1mbold\x1b[K", "bold"},
		{"\x1b[2Jclear", "clear"},
	}
	for _, tt := range tests {
		got := stripAnsi(tt.input)
		if got != tt.want {
			t.Errorf("stripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── parentOutputAdapter ─────────────────────────────────────────────────────

func TestParentOutputAdapter(t *testing.T) {
	var buf strings.Builder
	adapter := &parentOutputAdapter{parentWriter: &buf}
	n, err := adapter.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
	if buf.String() != "hello" {
		t.Errorf("expected 'hello', got %q", buf.String())
	}
}
