package main

import (
	"math"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hi", 1},
		{"hello world", 3},
		{stringsRepeat("a", 400), 100},
	}
	for _, tc := range tests {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", short(tc.input), got, tc.want)
		}
	}
}

func TestContextUsage(t *testing.T) {
	u := NewContextUsage(200000)

	u.Record(50000, "claude-sonnet")
	if len(u.history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(u.history))
	}

	frac := u.UsageFraction(50000)
	if math.Abs(frac-0.25) > 0.01 {
		t.Errorf("expected usage fraction 0.25, got %f", frac)
	}
}

func TestNeedsCompaction(t *testing.T) {
	// Create enough content to exceed the threshold
	bigContent := stringsRepeat("x", 300)
	msgs := []CompactionMessage{
		{Role: "user", Content: bigContent},
		{Role: "assistant", Content: bigContent},
	}
	cfg := DefaultCompactionConfig()
	cfg.MaxContextTokens = 100
	cfg.Threshold = 0.75

	if !NeedsCompaction(msgs, cfg) {
		t.Error("expected compaction needed for content exceeding threshold")
	}

	cfg.MaxContextTokens = 1000000
	if NeedsCompaction(msgs, cfg) {
		t.Error("expected no compaction needed for large context")
	}
}

func TestContextInfo(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "what is go?"},
		{Role: "assistant", Content: "Go is a programming language"},
	}
	info := ContextInfo(msgs, 200000)
	if info == "" {
		t.Error("expected non-empty context info")
	}
	// Should mention token count
	t.Logf("ContextInfo: %s", info)
}

func TestGroupMessagesByRound(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 2 {
		t.Errorf("expected 2 rounds, got %d", len(rounds))
	}
}

func TestFindSafeCompactionBoundary(t *testing.T) {
	rounds := []apiRound{
		{indices: []int{0, 1}, messages: []CompactionMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}},
		{indices: []int{2, 3}, messages: []CompactionMessage{{Role: "user", Content: "q2"}, {Role: "assistant", Content: "a2"}}},
		{indices: []int{4, 5}, messages: []CompactionMessage{{Role: "user", Content: "q3"}, {Role: "assistant", Content: "a3"}}},
		{indices: []int{6, 7}, messages: []CompactionMessage{{Role: "user", Content: "q4"}, {Role: "assistant", Content: "a4"}}},
	}

	// Keep last 2 rounds, boundary should be at index 2 (third round)
	boundary := findSafeCompactionBoundary(rounds, 2)
	if boundary != 2 {
		t.Errorf("expected boundary at 2, got %d", boundary)
	}
}

func TestSmartCompact(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "original question", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "assistant", Content: "long answer 1", Timestamp: "2024-01-01T00:00:01Z"},
		{Role: "user", Content: "followup", Timestamp: "2024-01-01T00:00:02Z"},
		{Role: "assistant", Content: "long answer 2", Timestamp: "2024-01-01T00:00:03Z"},
		{Role: "user", Content: "latest q", Timestamp: "2024-01-01T00:00:04Z"},
		{Role: "assistant", Content: "latest a", Timestamp: "2024-01-01T00:00:05Z"},
	}

	result := SmartCompact(msgs, 1, 1)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CollapsedTurns < 1 {
		t.Errorf("expected at least 1 collapsed turn, got %d", result.CollapsedTurns)
	}
	if len(result.Messages) == 0 {
		t.Error("expected some messages after compaction")
	}
}

func TestMicroCompact(t *testing.T) {
	// Simulate a conversation with multiple rounds
	msgs := []CompactionMessage{
		{Role: "user", Content: "run a command", Timestamp: "2024-01-01T00:00:00Z"},
		{Role: "assistant", Content: "ok", Timestamp: "2024-01-01T00:00:01Z"},
		{Role: "user", Content: "[tool result]", ToolName: "exec", Timestamp: "2024-01-01T00:00:02Z"},
		{Role: "assistant", Content: "done", Timestamp: "2024-01-01T00:00:03Z"},
		{Role: "user", Content: "another command", Timestamp: "2024-01-01T00:00:04Z"},
		{Role: "assistant", Content: "ok2", Timestamp: "2024-01-01T00:00:05Z"},
		{Role: "user", Content: "[tool result 2]", ToolName: "exec", Timestamp: "2024-01-01T00:00:06Z"},
		{Role: "assistant", Content: "done2", Timestamp: "2024-01-01T00:00:07Z"},
		{Role: "user", Content: "final question", Timestamp: "2024-01-01T00:00:08Z"},
	}

	result := MicroCompact(msgs, DefaultMicroCompactConfig())
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
	// Middle tool-heavy turns should be compacted, first user + last messages preserved
	t.Logf("MicroCompact: %d -> %d messages", len(msgs), len(result))
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, n*len(s))
	for i := 0; i < n; i++ {
		copy(out[i*len(s):], s)
	}
	return string(out)
}

func short(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}
