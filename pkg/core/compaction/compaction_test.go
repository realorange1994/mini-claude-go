package compaction

import (
	"testing"
)

func TestTokenCounter_Count(t *testing.T) {
	tc := NewTokenCounter("test-model")

	tests := []struct {
		name     string
		input    string
		minToken int
		maxToken int
	}{
		{"empty", "", 0, 0},
		{"short", "hello world", 1, 10},
		{"long", "The quick brown fox jumps over the lazy dog", 5, 20},
		{"cjk", "你好世界", 4, 20},
		{"mixed", "Hello 你好 world 世界", 4, 20},
		{"code", "func main() { fmt.Println(\"hello\") }", 3, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := tc.Count(tt.input)
			if count < tt.minToken || count > tt.maxToken {
				t.Errorf("Count(%q) = %d, want between %d and %d", tt.input, count, tt.minToken, tt.maxToken)
			}
		})
	}
}

func TestTokenCounter_CountMessages(t *testing.T) {
	tc := NewTokenCounter("test-model")

	messages := []string{
		"Hello world",
		"This is a test",
		"你好世界",
	}

	total := tc.CountMessages(messages)
	if total <= 0 {
		t.Errorf("CountMessages returned %d, want > 0", total)
	}

	// Total should be >= sum of individual counts
	sum := tc.Count(messages[0]) + tc.Count(messages[1]) + tc.Count(messages[2])
	if total != sum {
		t.Errorf("CountMessages = %d, want %d (sum of individual)", total, sum)
	}
}

func TestCompactor_CompactMessages(t *testing.T) {
	compactor := NewCompactor("test-model", &CompactionStrategy{
		TargetTokens:  100,
		PreserveRecent: 2,
	})

	messages := []string{
		"First message with some content",
		"Second message with more content here",
		"Third message that is also quite important",
		"Fourth message with even more content",
		"Fifth message the most recent one",
	}

	result, tokens, err := compactor.CompactMessages(messages, 2)
	if err != nil {
		t.Fatalf("CompactMessages error: %v", err)
	}

	// Should always preserve at least the last 2 messages
	if len(result) < 2 {
		t.Errorf("CompactMessages returned %d messages, want at least 2", len(result))
	}

	// Result should end with the last 2 messages
	if result[len(result)-1] != messages[len(messages)-1] {
		t.Errorf("Last message not preserved, got %q", result[len(result)-1])
	}

	_ = tokens
}

func TestCompactor_ShouldCompact(t *testing.T) {
	compactor := NewCompactor("test-model", &CompactionStrategy{
		TargetTokens: 100,
	})

	if !compactor.ShouldCompact(200) {
		t.Error("ShouldCompact(200) = false, want true when tokens > target")
	}
	if compactor.ShouldCompact(50) {
		t.Error("ShouldCompact(50) = true, want false when tokens < target")
	}
}

func TestBranchSummarizer_Summarize(t *testing.T) {
	bs := NewBranchSummarizer("test-model")

	messages := []string{
		"I'll use the Read tool to check the file",
		"Read /path/to/file.go",
		"I decided to refactor this function",
		"bash: go test ./...",
	}

	summary, err := bs.Summarize("session-1", "main", messages, "test-model")
	if err != nil {
		t.Fatalf("Summarize error: %v", err)
	}

	if summary == "" {
		t.Error("Summarize returned empty string")
	}

	// Should contain branch info
	if !contains(summary, "main") {
		t.Error("Summary should contain branch name")
	}
}

func TestExtractKeyActions(t *testing.T) {
	messages := []string{
		"tool_use: Read file.go",
		"bash: go build ./...",
		"just some regular text",
	}

	actions := extractKeyActions(messages)
	if len(actions) == 0 {
		t.Error("extractKeyActions returned no actions")
	}
}

func TestExtractFileReferences(t *testing.T) {
	messages := []string{
		"I'll read the file main.go and then check config.yaml",
		"Let's look at /path/to/test.go",
	}

	files := extractFileReferences(messages)
	if len(files) == 0 {
		t.Error("extractFileReferences returned no files")
	}
}

func TestExtractDecisions(t *testing.T) {
	messages := []string{
		"I decided to use the simpler approach",
		"I'll refactor the code later",
		"just checking things",
	}

	decisions := extractDecisions(messages)
	if len(decisions) == 0 {
		t.Error("extractDecisions returned no decisions")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
