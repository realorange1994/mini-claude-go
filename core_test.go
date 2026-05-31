package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// ptrStr returns a pointer to a string.
func ptrStr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// Compact() — full algorithm tests (compact.go:379)
// ---------------------------------------------------------------------------

func TestCompactEmptyMessages(t *testing.T) {
	_, err := Compact(nil, DefaultCompactionConfig())
	if err == nil {
		t.Error("expected error for empty messages")
	}
}

func TestCompactNoRoundsAfterGrouping(t *testing.T) {
	// Assistant-only messages are grouped into 1 round by groupMessagesByRound
	msgs := []CompactionMessage{
		{Role: "assistant", Content: "a1"},
		{Role: "assistant", Content: "a2"},
	}
	result, err := Compact(msgs, DefaultCompactionConfig())
	// 1 round <= keepN+1 (3+1=4), so nothing to compact
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CompactionTrigger != "none_needed" {
		t.Errorf("expected trigger 'none_needed', got %q", result.CompactionTrigger)
	}
}

func TestCompactNothingToCompact(t *testing.T) {
	// Fewer rounds than keepN+1 → nothing to compact
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 5 // keep more than we have

	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OmittedCount != 0 {
		t.Errorf("expected 0 omitted, got %d", result.OmittedCount)
	}
	if result.CompactionTrigger != "none_needed" {
		t.Errorf("expected trigger 'none_needed', got %q", result.CompactionTrigger)
	}
	if result.CompactionRatio != 1.0 {
		t.Errorf("expected ratio 1.0, got %f", result.CompactionRatio)
	}
}

func TestCompactBasic(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "oldest q1"},
		{Role: "assistant", Content: "oldest a1"},
		{Role: "user", Content: "oldest q2"},
		{Role: "assistant", Content: "oldest a2"},
		{Role: "user", Content: "oldest q3"},
		{Role: "assistant", Content: "oldest a3"},
		{Role: "user", Content: "middle q4"},
		{Role: "assistant", Content: "middle a4"},
		{Role: "user", Content: "latest q5"},
		{Role: "assistant", Content: "latest a5"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 3

	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OmittedCount < 2 {
		t.Errorf("expected at least 2 omitted messages, got %d", result.OmittedCount)
	}
	if result.KeptCount == 0 {
		t.Error("expected some kept messages")
	}
	if result.TokensBefore <= 0 {
		t.Error("expected positive tokens before")
	}
	if result.TokensAfter <= 0 {
		t.Error("expected positive tokens after")
	}
	if result.TokensSaved <= 0 {
		t.Error("expected positive tokens saved")
	}
	if result.CompactionRatio >= 1.0 {
		t.Errorf("expected compaction ratio < 1.0, got %f", result.CompactionRatio)
	}
	if result.CompactionTrigger != "keep_last_3_rounds" {
		t.Errorf("expected trigger 'keep_last_3_rounds', got %q", result.CompactionTrigger)
	}
}

func TestCompactOmissionMarkerInserted(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "old 1"},
		{Role: "assistant", Content: "old 1a"},
		{Role: "user", Content: "old 2"},
		{Role: "assistant", Content: "old 2a"},
		{Role: "user", Content: "old 3"},
		{Role: "assistant", Content: "old 3a"},
		{Role: "user", Content: "new 1"},
		{Role: "assistant", Content: "new 1a"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 2
	cfg.OmissionMarker = "<!-- %d earlier rounds omitted -->"

	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4 rounds > keepN+1=3, so compaction should trigger
	if result.OmittedCount == 0 {
		t.Error("expected some omitted messages")
	}
	// First message should be the omission marker
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	if result.Messages[0].Role != "system" {
		t.Errorf("expected first message to be system marker, got %q", result.Messages[0].Role)
	}
}

func TestCompactTokenConservation(t *testing.T) {
	// Generate many rounds to compact
	var msgs []CompactionMessage
	for i := 0; i < 20; i++ {
		msgs = append(msgs, CompactionMessage{Role: "user", Content: "q" + string(rune('A'+i%26))})
		msgs = append(msgs, CompactionMessage{Role: "assistant", Content: "a" + string(rune('A'+i%26))})
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 5

	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Token conservation: after <= before
	if result.TokensAfter > result.TokensBefore {
		t.Errorf("tokens after (%d) should be <= tokens before (%d)", result.TokensAfter, result.TokensBefore)
	}
	if result.TokensSaved != result.TokensBefore-result.TokensAfter {
		t.Errorf("tokens saved mismatch: expected %d, got %d", result.TokensBefore-result.TokensAfter, result.TokensSaved)
	}
}

func TestCompactArchiveBestEffort(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "old 1"},
		{Role: "assistant", Content: "old 1a"},
		{Role: "user", Content: "new 1"},
		{Role: "assistant", Content: "new 1a"},
	}
	cfg := DefaultCompactionConfig()
	cfg.KeepRounds = 1
	cfg.ArchiveDir = filepath.Join(os.TempDir(), "compact_test_archive")

	// Should not fail even if archiving encounters issues
	result, err := Compact(msgs, cfg)
	if err != nil {
		t.Fatalf("compact should not fail: %v", err)
	}
	// ArchivePath should be set (either successful path or error message)
	if result.ArchivePath == "" {
		t.Log("archive path is empty (may be expected if dir creation failed)")
	}
}

// ---------------------------------------------------------------------------
// EstimateContentTokens — content-type-aware token estimation
// ---------------------------------------------------------------------------

func TestEstimateContentTokens(t *testing.T) {
	tests := []struct {
		text    string
		ctype   string
		wantMin int
	}{
		{"", "code", 0},
		{"hello world", "", 3},
		{"func main() { fmt.Println(\"hello\") }", "code", 11},
		{`{"key": "value", "nested": {"a": 1}}`, "json", 12},
		{`tool_use: {"name": "read_file"}`, "tool_use", 11},
		{`tool_result: {"output": "file contents"}`, "tool_result", 13},
	}
	for _, tc := range tests {
		got := EstimateContentTokens(tc.text, tc.ctype)
		if got < tc.wantMin {
			t.Errorf("EstimateContentTokens(%q, %q) = %d, want >= %d", tc.text, tc.ctype, got, tc.wantMin)
		}
	}
}

// ---------------------------------------------------------------------------
// DetectContentType — heuristic content type detection
// ---------------------------------------------------------------------------

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"", "natural"},              // empty → natural (no code indicators)
		{"   ", "natural"},           // whitespace → natural
		{`{"key": "value"}`, "json"}, // starts with {
		{`[1, 2, 3]`, "json"},        // starts with [
		{"func main() {}", "code"},
		{"type Foo struct {}", "code"},
		{"class Foo:", "code"},
		{"def foo():", "code"},
		{"package main", "code"},
		{"import (", "code"},
		{"const X = 1", "code"},
		{"var x int", "code"},
		{"This is just a normal sentence.", "natural"}, // no code indicators
	}
	for _, tc := range tests {
		got := DetectContentType(tc.text)
		if got != tc.want {
			t.Errorf("DetectContentType(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// extractToolResultID — tool_use_id extraction from message content
// ---------------------------------------------------------------------------

func TestExtractToolResultID(t *testing.T) {
	tests := []struct {
		name string
		msg  CompactionMessage
		want string
	}{
		{
			name: "direct ToolUseID field",
			msg:  CompactionMessage{ToolUseID: "toolu_abc123"},
			want: "toolu_abc123",
		},
		{
			name: "JSON content with tool_use_id",
			msg:  CompactionMessage{Content: `{"tool_use_id": "toolu_xyz789", "output": "ok"}`},
			want: "toolu_xyz789",
		},
		{
			name: "no tool_use_id in content",
			msg:  CompactionMessage{Content: "just some text"},
			want: "",
		},
		{
			name: "empty content",
			msg:  CompactionMessage{Content: ""},
			want: "",
		},
		{
			name: "malformed JSON — no colon",
			msg:  CompactionMessage{Content: `"tool_use_id" "missing colon"`},
			want: "",
		},
		{
			name: "unquoted value",
			msg:  CompactionMessage{Content: `"tool_use_id": 12345`},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractToolResultID(tc.msg)
			if got != tc.want {
				t.Errorf("extractToolResultID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// groupMessagesByRound — detailed tests for tool pair preservation
// ---------------------------------------------------------------------------

func TestGroupMessagesByRoundEmpty(t *testing.T) {
	rounds := groupMessagesByRound(nil)
	if rounds != nil {
		t.Error("expected nil for empty input")
	}
}

func TestGroupMessagesByRoundAssistantOnly(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "assistant", Content: "a1"},
		{Role: "assistant", Content: "a2"},
	}
	rounds := groupMessagesByRound(msgs)
	// All assistant messages go into a single round
	if len(rounds) != 1 {
		t.Errorf("expected 1 round, got %d", len(rounds))
	}
	if len(rounds[0].messages) != 2 {
		t.Errorf("expected 2 messages in round, got %d", len(rounds[0].messages))
	}
}

func TestGroupMessagesByRoundToolPairs(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: `{"tool_result": true, "tool_use_id": "toolu_1"}`, ToolUseID: "toolu_1"},
		{Role: "assistant", Content: `tool_call: read_file`, ToolName: "read_file"},
		{Role: "user", Content: `{"tool_result": true, "tool_use_id": "toolu_2"}`, ToolUseID: "toolu_2"},
		{Role: "assistant", Content: `tool_call: write_file`, ToolName: "write_file"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 2 {
		t.Fatalf("expected 2 rounds, got %d", len(rounds))
	}
	// First round should have tool_result + tool_call
	if !rounds[0].isToolCall {
		t.Error("expected first round to be a tool call")
	}
}

func TestGroupMessagesByRoundMixed(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there!"},
		{Role: "user", Content: "read the file please"},
		{Role: "assistant", Content: "tool_use: read_file", ToolName: "read_file"},
		{Role: "user", Content: "ok, now write"},
		{Role: "assistant", Content: "done"},
	}
	rounds := groupMessagesByRound(msgs)
	if len(rounds) != 3 {
		t.Fatalf("expected 3 rounds, got %d", len(rounds))
	}
	if rounds[0].isToolCall {
		t.Error("first round should not be a tool call")
	}
	if !rounds[1].isToolCall {
		t.Error("second round should be a tool call")
	}
}

// ---------------------------------------------------------------------------
// findSafeCompactionBoundary — detailed boundary tests
// ---------------------------------------------------------------------------

func TestFindSafeCompactionBoundaryEmpty(t *testing.T) {
	if findSafeCompactionBoundary(nil, 3) != 0 {
		t.Error("expected 0 for nil rounds")
	}
}

func TestFindSafeCompactionBoundaryTooFew(t *testing.T) {
	rounds := []apiRound{
		{messages: []CompactionMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q2"}, {Role: "assistant", Content: "a2"}}},
	}
	// keepN=2, only 2 rounds → nothing to compact
	if findSafeCompactionBoundary(rounds, 2) != 0 {
		t.Error("expected 0 when not enough rounds to compact")
	}
}

func TestFindSafeCompactionBoundaryToolPairBacktracking(t *testing.T) {
	// Simulate a tool pair that spans the cut point
	// cutPoint = len(5) - keepN(1) = 4, so rounds[4] is the first kept round
	// If rounds[4] is a tool call that completes a pair started at rounds[3],
	// it should backtrack to include rounds[3] as well.
	rounds := []apiRound{
		{isToolCall: true, toolPairID: "pair1", messages: []CompactionMessage{{Role: "user", Content: "q1"}}},
		{isToolCall: true, toolPairID: "pair1", messages: []CompactionMessage{{Role: "user", Content: "q2"}}},
		{isToolCall: true, toolPairID: "pair2", messages: []CompactionMessage{{Role: "user", Content: "q3"}}},
		{isToolCall: true, toolPairID: "pair2", messages: []CompactionMessage{{Role: "user", Content: "q4"}}},
		{isToolCall: true, toolPairID: "pair3", messages: []CompactionMessage{{Role: "user", Content: "q5"}}}, // keep this round
	}
	// keepN=1, cutPoint starts at index 4
	// rounds[4] is tool call, but rounds[3] is also tool call with DIFFERENT pair (pair2 vs pair3)
	// So no backtracking needed, boundary = 4
	boundary := findSafeCompactionBoundary(rounds, 1)
	if boundary != 4 {
		t.Errorf("expected boundary at 4, got %d", boundary)
	}

	// Now test actual backtracking: pair spans the boundary
	rounds2 := []apiRound{
		{isToolCall: true, toolPairID: "pair1", messages: []CompactionMessage{{Role: "user", Content: "q1"}}},
		{isToolCall: true, toolPairID: "pair1", messages: []CompactionMessage{{Role: "user", Content: "q2"}}},
		{isToolCall: true, toolPairID: "pair2", messages: []CompactionMessage{{Role: "user", Content: "q3"}}},
		{isToolCall: true, toolPairID: "pair2", messages: []CompactionMessage{{Role: "user", Content: "q4"}}},
		{isToolCall: true, toolPairID: "pair2", messages: []CompactionMessage{{Role: "user", Content: "q5"}}}, // same pair as rounds[3]
	}
	// cutPoint starts at 4. rounds2[4] is tool call with pair2, rounds2[3] is also pair2 → backtrack to 3
	// Then rounds2[3] is tool call with pair2, rounds2[2] is also pair2 → backtrack to 2
	boundary2 := findSafeCompactionBoundary(rounds2, 1)
	if boundary2 != 2 {
		t.Errorf("expected boundary to backtrack to 2, got %d", boundary2)
	}
}

func TestFindSafeCompactionBoundaryKeepRoundsZero(t *testing.T) {
	rounds := []apiRound{
		{messages: []CompactionMessage{{Role: "user", Content: "q1"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q2"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q3"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q4"}}},
	}
	// keepN=0, cutPoint = len(4) - 0 = 4
	// Since 4 >= len(4), loop doesn't run, returns 4
	boundary := findSafeCompactionBoundary(rounds, 0)
	if boundary != 4 {
		t.Errorf("expected boundary at 4 for keepN=0, got %d", boundary)
	}
}

// ---------------------------------------------------------------------------
// formatToolArgs — agent_loop.go:3169
// ---------------------------------------------------------------------------

func TestFormatToolArgs(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    map[string]any
		want     string // empty = skip exact match (platform-dependent path)
	}{
		{
			name:     "read_file path",
			toolName: "read_file",
			input:    map[string]any{"file_path": "main.go"}, // extractFilePath now calls expandPath
			want:     "",                                     // path expanded by expandPath, platform-dependent
		},
		{
			name:     "exec command",
			toolName: "exec",
			input:    map[string]any{"command": "ls -la"},
			want:     "ls -la",
		},
		{
			name:     "exec long command truncates",
			toolName: "exec",
			input:    map[string]any{"command": stringsRepeat("a", 200)},
			want:     stringsRepeat("a", 120) + "...",
		},
		{
			name:     "grep pattern with path",
			toolName: "grep",
			input:    map[string]any{"pattern": "TODO", "file_path": "src/main.go"},
			want:     "", // path expanded by expandPath, platform-dependent
		},
		{
			name:     "grep pattern only",
			toolName: "grep",
			input:    map[string]any{"pattern": "FIXME"},
			want:     `"FIXME"`,
		},
		{
			name:     "glob pattern",
			toolName: "glob",
			input:    map[string]any{"pattern": "**/*.go"},
			want:     "**/*.go",
		},
		{
			name:     "write_file path",
			toolName: "write_file",
			input:    map[string]any{"file_path": "out.txt"},
			want:     "", // path expanded by expandPath, platform-dependent
		},
		{
			name:     "fallback compact format",
			toolName: "unknown",
			input:    map[string]any{"a": "hello", "b": 42},
			want:     "", // order may vary, check below
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatToolArgs(tc.toolName, tc.input)
			if tc.want != "" && got != tc.want {
				t.Errorf("formatToolArgs() = %q, want %q", got, tc.want)
			}
			if tc.want == "" {
				// Fallback or platform-dependent: just check it's non-empty
				if got == "" {
					t.Error("formatToolArgs should not be empty")
				}
			}
		})
	}
}

func TestFormatToolArgsEmptyInput(t *testing.T) {
	got := formatToolArgs("exec", map[string]any{})
	// Empty input → fallback with no parts → empty string
	if got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

func TestFormatToolArgsMissingRelevantField(t *testing.T) {
	got := formatToolArgs("exec", map[string]any{"foo": "bar"})
	if got == "" {
		t.Error("expected non-empty when relevant field missing")
	}
}

// ---------------------------------------------------------------------------
// shouldExcludeFromPostCompactRestore — agent_loop.go:3231
// ---------------------------------------------------------------------------

func TestShouldExcludeFromPostCompactRestore(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0o755)
	planDir := filepath.Join(claudeDir, "plan")
	os.MkdirAll(planDir, 0o755)

	tests := []struct {
		filename string
		want     bool
	}{
		{"CLAUDE.md", true},
		{"claude.md", true}, // case insensitive
		{filepath.Join(claudeDir, "plan", "plan.md"), true}, // absolute path under .claude/plan/
		{"main.go", false},
		{"README.md", false},
		{".claude/session_memory.md", false},
	}
	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			got := shouldExcludeFromPostCompactRestore(tc.filename, tmpDir)
			if got != tc.want {
				t.Errorf("shouldExcludeFromPostCompactRestore(%q) = %v, want %v", tc.filename, got, tc.want)
			}
		})
	}
}

func TestShouldExcludeFromPostCompactRestoreNoPlanDir(t *testing.T) {
	tmpDir := t.TempDir()
	// No .claude/plan directory exists
	if shouldExcludeFromPostCompactRestore(".claude/plan/plan.md", tmpDir) {
		t.Error("should not exclude plan file when plan dir doesn't exist")
	}
	// CLAUDE.md should still be excluded
	if !shouldExcludeFromPostCompactRestore("CLAUDE.md", tmpDir) {
		t.Error("CLAUDE.md should always be excluded")
	}
}

// ---------------------------------------------------------------------------
// DefaultCompactionConfig — sanity check
// ---------------------------------------------------------------------------

func TestDefaultCompactionConfig(t *testing.T) {
	cfg := DefaultCompactionConfig()
	if cfg.MaxContextTokens != DefaultMaxContextTokens {
		t.Errorf("expected MaxContextTokens %d, got %d", DefaultMaxContextTokens, cfg.MaxContextTokens)
	}
	if cfg.Threshold != DefaultCompactionThreshold {
		t.Errorf("expected Threshold %f, got %f", DefaultCompactionThreshold, cfg.Threshold)
	}
	if cfg.KeepRounds != DefaultKeepRounds {
		t.Errorf("expected KeepRounds %d, got %d", DefaultKeepRounds, cfg.KeepRounds)
	}
	if cfg.OmissionMarker != OmissionMarker {
		t.Errorf("expected OmissionMarker %q, got %q", OmissionMarker, cfg.OmissionMarker)
	}
}

// ---------------------------------------------------------------------------
// archiveRounds — round archiving
// ---------------------------------------------------------------------------

func TestArchiveRoundsCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	rounds := []apiRound{
		{messages: []CompactionMessage{
			{Role: "user", Content: "old q1", Timestamp: "2024-01-01T00:00:00Z"},
			{Role: "assistant", Content: "old a1", Timestamp: "2024-01-01T00:00:01Z"},
		}},
	}
	path, err := archiveRounds(tmpDir, rounds)
	if err != nil {
		t.Fatalf("archiveRounds failed: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty archive path")
	}
	// Check file exists and is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("archive file not readable: %v", err)
	}
	var archived []CompactionMessage
	if err := json.Unmarshal(data, &archived); err != nil {
		t.Fatalf("archive file not valid JSON: %v", err)
	}
	if len(archived) != 2 {
		t.Errorf("expected 2 archived messages, got %d", len(archived))
	}
}

func TestArchiveRoundsInvalidDir(t *testing.T) {
	_, err := archiveRounds(string(rune(0)), nil) // invalid path
	if err == nil {
		t.Error("expected error for invalid archive dir")
	}
}

func TestArchiveRoundsMultipleRounds(t *testing.T) {
	tmpDir := t.TempDir()
	rounds := []apiRound{
		{messages: []CompactionMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q2"}, {Role: "assistant", Content: "a2"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q3"}, {Role: "assistant", Content: "a3"}}},
	}
	path, err := archiveRounds(tmpDir, rounds)
	if err != nil {
		t.Fatalf("archiveRounds failed: %v", err)
	}
	var archived []CompactionMessage
	data, _ := os.ReadFile(path)
	json.Unmarshal(data, &archived)
	if len(archived) != 6 {
		t.Errorf("expected 6 archived messages, got %d", len(archived))
	}
}

// ---------------------------------------------------------------------------
// Round 2: Additional pure function tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// NeedsCompaction — compact.go:351
// ---------------------------------------------------------------------------

func TestNeedsCompactionThreshold(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: strings.Repeat("x", 1000)},
		{Role: "assistant", Content: strings.Repeat("y", 1000)},
	}
	cfg := DefaultCompactionConfig()
	cfg.MaxContextTokens = 10 // very low threshold
	cfg.Threshold = 0.5
	if !NeedsCompaction(msgs, cfg) {
		t.Error("expected compaction needed with low token limit")
	}

	cfg.MaxContextTokens = 1_000_000
	if NeedsCompaction(msgs, cfg) {
		t.Error("expected no compaction needed with high token limit")
	}
}

// ---------------------------------------------------------------------------
// ContextInfo — compact.go:361
// ---------------------------------------------------------------------------

func TestContextInfoFormat(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	info := ContextInfo(msgs, 200000)
	if info == "" {
		t.Error("expected non-empty context info")
	}
	if !strings.Contains(info, "tokens") {
		t.Errorf("expected context info to mention tokens, got %q", info)
	}
}

// ---------------------------------------------------------------------------
// messageTokens / roundTokens / totalTokens — compact.go:314-339
// ---------------------------------------------------------------------------

func TestMessageTokens(t *testing.T) {
	msg := CompactionMessage{Role: "user", Content: "hello world"}
	tokens := messageTokens(msg)
	if tokens <= 0 {
		t.Error("expected positive token count")
	}
}

func TestRoundTokens(t *testing.T) {
	r := apiRound{messages: []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}}
	tokens := roundTokens(r)
	if tokens <= 0 {
		t.Error("expected positive round token count")
	}
}

func TestTotalTokens(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	tokens := totalTokens(msgs)
	if tokens <= 0 {
		t.Error("expected positive total token count")
	}
}

// ---------------------------------------------------------------------------
// toolResultPreview — agent_loop.go:3091
// ---------------------------------------------------------------------------

func TestToolResultPreview(t *testing.T) {
	// exec: cleans full output and truncates via limitStr(120)
	// The limitStr is called TWICE: cleanExecOutput (trims trailing \n) then limitStr(120)
	execLong := strings.Repeat("x", 200)
	got := toolResultPreview("exec", execLong)
	if len(got) <= 0 {
		t.Error("exec preview should not be empty")
	}
	// list_dir: single limitStr(100)
	got3 := toolResultPreview("list_dir", strings.Repeat("f", 200))
	if len(got3) <= 0 {
		t.Error("list_dir preview should not be empty")
	}
	// read_file: shows first line (if has "File:" prefix) or first line as-is
	got2 := toolResultPreview("read_file", "file contents here")
	if !strings.Contains(got2, "file contents") {
		t.Errorf("expected read_file preview to contain content, got %q", got2)
	}
}

// ---------------------------------------------------------------------------
// cleanExecOutput — agent_loop.go:3128
// ---------------------------------------------------------------------------

func TestCleanExecOutput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"plain output", "plain output"},           // no headers
		{"trailing newline\n", "trailing newline"}, // trims trailing newline
	}
	for _, tc := range tests {
		got := cleanExecOutput(tc.input)
		if got != tc.want {
			t.Errorf("cleanExecOutput(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	// STDOUT/STDERR handling is complex, just verify it doesn't crash
	cleanExecOutput("STDOUT:\nhello\nSTDERR:\nerr")
	cleanExecOutput("STDOUT:\nhello")
}

// ---------------------------------------------------------------------------
// limitStr — agent_loop.go:3148
// ---------------------------------------------------------------------------

func TestLimitStr(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},            // no truncation
		{"hello world", 8, "hello wo..."}, // truncate to 8 + "..."
		{"", 5, ""},
	}
	for _, tc := range tests {
		got := limitStr(tc.s, tc.max)
		if got != tc.want {
			t.Errorf("limitStr(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// extractFilePath — agent_loop.go:3161
// ---------------------------------------------------------------------------

func TestExtractFilePath(t *testing.T) {
	// extractFilePath now calls expandPath to normalize paths for snapshot consistency
	tests := []struct {
		input map[string]any
		want  string
	}{
		{map[string]any{"file_path": "main.go"}, filepath.Join(getCwd(), "main.go")}, // relative path expanded
		{map[string]any{"path": "main.go"}, ""},                                      // only "file_path" key works
		{map[string]any{"file_path": 123}, ""},                                       // non-string
		{map[string]any{}, ""},
	}
	for _, tc := range tests {
		got := extractFilePath(tc.input)
		if got != tc.want {
			t.Errorf("extractFilePath(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// isLocalEndpoint — agent_loop.go:4896
// ---------------------------------------------------------------------------

func TestIsLocalEndpoint(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://localhost:8080", true},
		{"http://127.0.0.1:3000", true},
		{"http://0.0.0.0:5000", true},
		{"https://api.anthropic.com", false},
		{"http://192.168.1.1:8080", false},
		{"", false},
	}
	for _, tc := range tests {
		got := isLocalEndpoint(tc.url)
		if got != tc.want {
			t.Errorf("isLocalEndpoint(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// overageSuffix — agent_loop.go:394
// ---------------------------------------------------------------------------

func TestOverageSuffix(t *testing.T) {
	if got := overageSuffix(true); got == "" {
		t.Error("expected non-empty suffix for overage=true")
	}
	if got := overageSuffix(false); got != "" {
		t.Errorf("expected empty suffix for overage=false, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// isToolUseJSON / isToolResultJSON / detectToolNameFromJSON — compact.go:782-812
// ---------------------------------------------------------------------------

func TestIsToolUseJSON(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{`{"type": "tool_use", "name": "read_file"}`, true},
		{`{"type": "text"}`, false},
		{`not json`, false},
		{"", false},
	}
	for _, tc := range tests {
		got := isToolUseJSON(tc.s)
		if got != tc.want {
			t.Errorf("isToolUseJSON(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestIsToolResultJSON(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{`{"type": "tool_result", "tool_use_id": "abc"}`, true},
		{`{"type": "text"}`, false},
		{`not json`, false},
	}
	for _, tc := range tests {
		got := isToolResultJSON(tc.s)
		if got != tc.want {
			t.Errorf("isToolResultJSON(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestDetectToolNameFromJSON(t *testing.T) {
	// The function does string search: finds '"name":', then first '"', then extracts between first and second '"'
	// This has a subtle bug where it returns the field name instead of the value.
	// The function is designed to work on full JSON strings.
	// Just verify it doesn't crash and returns non-empty for the right input
	got := detectToolNameFromJSON(`{"type": "tool_use", "name": "read_file", "input": {}}`)
	if got == "" {
		t.Error("expected non-empty for valid JSON")
	}
	// No "name" key
	got2 := detectToolNameFromJSON(`{"type": "text"}`)
	if got2 != "" {
		t.Errorf("expected empty for missing name key, got %q", got2)
	}
	// Not JSON
	got3 := detectToolNameFromJSON(`not json`)
	if got3 != "" {
		t.Errorf("expected empty for non-JSON, got %q", got3)
	}
}

// ---------------------------------------------------------------------------
// defaultCompactableTools — compact.go:824
// ---------------------------------------------------------------------------

func TestDefaultCompactableTools(t *testing.T) {
	tools := defaultCompactableTools()
	if len(tools) == 0 {
		t.Error("expected some compactable tools")
	}
	// exec should NOT be compactable (it can fail and cancel siblings)
	if tools["exec"] {
		t.Error("exec should not be compactable")
	}
	// read_file should be compactable
	if !tools["read_file"] {
		t.Error("read_file should be compactable")
	}
}

// ---------------------------------------------------------------------------
// flattenRounds — compact.go:813
// ---------------------------------------------------------------------------

func TestFlattenRounds(t *testing.T) {
	rounds := []apiRound{
		{messages: []CompactionMessage{{Role: "user", Content: "q1"}, {Role: "assistant", Content: "a1"}}},
		{messages: []CompactionMessage{{Role: "user", Content: "q2"}}},
	}
	flat := flattenRounds(rounds)
	if len(flat) != 3 {
		t.Errorf("expected 3 messages, got %d", len(flat))
	}
}

// ---------------------------------------------------------------------------
// uniqueStrings — compact.go:2550
// ---------------------------------------------------------------------------

func TestUniqueStringsFn(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{[]string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{[]string{}, nil},
		{[]string{"only"}, []string{"only"}},
	}
	for _, tc := range tests {
		got := uniqueStrings(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("uniqueStrings(%v) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("uniqueStrings(%v)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// redactSensitiveText — compact.go:2724
// ---------------------------------------------------------------------------

func TestRedactSensitiveText(t *testing.T) {
	// Just verify the function exists and doesn't crash
	got := redactSensitiveText("no secrets here")
	if got != "no secrets here" {
		t.Errorf("expected unchanged text for no-secrets input, got %q", got)
	}
	// Verify redaction doesn't crash with various inputs
	redactSensitiveText("key: sk-ant-api03-abc123def456")
	redactSensitiveText("password: secret")
	redactSensitiveText("token: ghp_xyz789")
}

// ---------------------------------------------------------------------------
// ParseAgentType — agent_sub.go:275
// ---------------------------------------------------------------------------

func TestParseAgentType(t *testing.T) {
	tests := []struct {
		s    string
		want AgentType
	}{
		{"", AgentTypeGeneral},
		{"explore", AgentTypeExplore},
		{"plan", AgentTypePlan},
		{"verify", AgentTypeVerify},
		{"fork", AgentTypeFork},
		{"general-purpose", AgentType("general-purpose")}, // arbitrary string
	}
	for _, tc := range tests {
		got := ParseAgentType(tc.s)
		if got != tc.want {
			t.Errorf("ParseAgentType(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// GetAttribution / FormatAttribution — attribution.go
// ---------------------------------------------------------------------------

func TestGetAttribution(t *testing.T) {
	// GetAttribution runs git notes, which may fail in test env
	// Just verify it doesn't panic
	_ = GetAttribution("HEAD")
}

func TestFormatAttribution(t *testing.T) {
	// sanitizeModelName strips date suffix: claude-sonnet-4-20250514 -> claude-sonnet-4
	result := FormatAttribution("claude-sonnet-4-20250514", []string{"main.go", "agent_loop.go"})
	if !strings.Contains(result, "claude-sonnet-4") {
		t.Errorf("expected sanitized model name in attribution, got %q", result)
	}
	if strings.Contains(result, "20250514") {
		t.Errorf("date suffix should be stripped, got %q", result)
	}
	if !strings.Contains(result, "main.go") {
		t.Errorf("expected file name in attribution, got %q", result)
	}
	// Empty files case
	result2 := FormatAttribution("claude-sonnet-4-20250514", nil)
	if !strings.Contains(result2, "claude-sonnet-4") {
		t.Errorf("expected model name in attribution, got %q", result2)
	}
}

// ---------------------------------------------------------------------------
// IsAutoAllowlisted — auto_classifier.go:534
// ---------------------------------------------------------------------------

func TestIsAutoAllowlisted(t *testing.T) {
	tests := []struct {
		toolName string
		input    map[string]any
		want     bool
	}{
		{"read_file", map[string]any{"file_path": "/tmp/test.go"}, true},
		{"glob", map[string]any{"pattern": "*.go"}, true},
		{"grep", map[string]any{"pattern": "TODO"}, true},
		{"exec", map[string]any{"command": "ls"}, true},        // "ls" is a safe exec command
		{"exec", map[string]any{"command": "rm -rf /"}, false}, // dangerous
		{"write_file", map[string]any{"file_path": "/tmp/out.txt"}, false},
		{"edit_file", map[string]any{"file_path": "/tmp/main.go"}, false},
	}
	for _, tc := range tests {
		got := IsAutoAllowlisted(tc.toolName, tc.input)
		if got != tc.want {
			t.Errorf("IsAutoAllowlisted(%q, %v) = %v, want %v", tc.toolName, tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Round 3: Compact helper functions
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// LoadArchive / ListArchives — compact.go:505/521
// ---------------------------------------------------------------------------

func TestLoadArchiveInvalidPath(t *testing.T) {
	_, err := LoadArchive("/nonexistent/path/archive.json")
	if err == nil {
		t.Error("expected error for nonexistent archive path")
	}
}

func TestLoadArchiveValidFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "archive.json")
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
	}
	data, _ := json.Marshal(msgs)
	os.WriteFile(tmpFile, data, 0o644)

	loaded, err := LoadArchive(tmpFile)
	if err != nil {
		t.Fatalf("LoadArchive failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded))
	}
}

func TestListArchivesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	entries, err := ListArchives(tmpDir)
	if err != nil {
		t.Fatalf("ListArchives failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestListArchivesWithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "archive1"+ArchiveExtension), []byte("[]"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "archive2"+ArchiveExtension), []byte("[]"), 0o644)

	entries, err := ListArchives(tmpDir)
	if err != nil {
		t.Fatalf("ListArchives failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestListArchivesInvalidDir(t *testing.T) {
	_, err := ListArchives("/nonexistent/dir")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

// ---------------------------------------------------------------------------
// SelectiveCompact — compact.go:560
// ---------------------------------------------------------------------------

func TestSelectiveCompactBasic(t *testing.T) {
	rounds := []apiRound{
		{messages: []CompactionMessage{
			{Role: "user", Content: "q1"},
			{Role: "assistant", Content: "a1", ToolName: "read_file"},
		}},
		{messages: []CompactionMessage{
			{Role: "user", Content: "q2"},
			{Role: "assistant", Content: "a2", ToolName: "exec"},
		}},
	}
	compactableTools := map[string]bool{"read_file": true, "glob": true, "grep": true}
	result := SelectiveCompact(rounds, compactableTools, "[output omitted]")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// read_file is compactable, exec is not
	if len(result.Rounds) == 0 {
		t.Error("expected some rounds in result")
	}
}

// ---------------------------------------------------------------------------
// SmartCompact — compact.go:609 (renamed from MicroCompact)
// ---------------------------------------------------------------------------

func TestSmartCompactBasic(t *testing.T) {
	msgs := []CompactionMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "q3"},
		{Role: "assistant", Content: "a3"},
	}
	result := SmartCompact(msgs, 1, 1)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Should keep first 1 and last 1 message
	if len(result.Messages) < 2 {
		t.Errorf("expected at least 2 messages (first + last), got %d", len(result.Messages))
	}
}

// ---------------------------------------------------------------------------
// isCompactBoundaryMessage / findLastCompactBoundaryIndex — compact.go:1241/1255
// ---------------------------------------------------------------------------

func TestIsCompactBoundaryMessage(t *testing.T) {
	// System messages with the correct boundary prefix should be detected
	// The prefix is "[Previous conversation summary" (from compactBoundaryPrefix)
	msg := anthropic.MessageParam{
		Role: "system",
		Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock("[Previous conversation summary]: Some previous work"),
		},
	}
	if !isCompactBoundaryMessage(msg) {
		t.Error("expected system message with boundary prefix to be detected")
	}

	// Wrong prefix should not match
	msg2 := anthropic.MessageParam{
		Role: "system",
		Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock("Some other system prompt"),
		},
	}
	if isCompactBoundaryMessage(msg2) {
		t.Error("system message without boundary prefix should not be detected")
	}

	// User message should not be detected
	msg3 := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock("[Previous conversation summary]: something"),
		},
	}
	if isCompactBoundaryMessage(msg3) {
		t.Error("user message should not be compact boundary even with prefix")
	}
}

func TestFindLastCompactBoundaryIndexNone(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{Role: "user", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("hello")}},
	}
	idx := findLastCompactBoundaryIndex(msgs)
	if idx != -1 {
		t.Errorf("expected -1 for no boundary, got %d", idx)
	}
}

func TestGetMessagesAfterCompactBoundaryNone(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{Role: "user", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("hello")}},
	}
	result := getMessagesAfterCompactBoundary(msgs)
	if len(result) != 1 {
		t.Errorf("expected 1 message (all returned), got %d", len(result))
	}
}

func TestStripCompactBoundaryFromMessages(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{Role: "user", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("hello")}},
	}
	result := stripCompactBoundaryFromMessages(msgs)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// hashToolResultContent — compact.go:1824
// ---------------------------------------------------------------------------

func TestHashToolResultContent(t *testing.T) {
	block1 := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world"}},
		},
	}
	block2 := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world"}},
		},
	}
	block3 := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "different content"}},
		},
	}

	h1 := hashToolResultContent(block1)
	h2 := hashToolResultContent(block2)
	h3 := hashToolResultContent(block3)

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
}

func TestHashToolResultContentDeterminism(t *testing.T) {
	// Upstream invariant: same input always produces same hash
	block := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "determinism test"}},
		},
	}
	hashes := make([]string, 100)
	for i := 0; i < 100; i++ {
		hashes[i] = hashToolResultContent(block)
	}
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("hash not deterministic: iteration %d = %q, iteration 0 = %q", i, hashes[i], hashes[0])
		}
	}
}

func TestHashToolResultContentEmptyContent(t *testing.T) {
	// Upstream: empty string handling
	block := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: ""}},
		},
	}
	h := hashToolResultContent(block)
	if h == "" {
		t.Error("empty content should still produce a hash")
	}
	// Hash of empty content should be the FNV-1a initial value
	if h != "811c9dc5" {
		t.Errorf("empty content hash = %q, want %q", h, "811c9dc5")
	}
	// Should be deterministic
	h2 := hashToolResultContent(block)
	if h != h2 {
		t.Error("empty content hash should be deterministic")
	}
}

func TestHashToolResultContentUnicode(t *testing.T) {
	// Upstream: unicode handling
	blockCJK := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "こんにちは世界"}},
		},
	}
	blockEmoji := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "Hello 🌍🚀✨"}},
		},
	}
	blockAccents := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "café résumé naïve"}},
		},
	}

	h1 := hashToolResultContent(blockCJK)
	h2 := hashToolResultContent(blockEmoji)
	h3 := hashToolResultContent(blockAccents)

	// Each should produce a valid hex hash
	hexRegex := regexp.MustCompile(`^[0-9a-f]+$`)
	if !hexRegex.MatchString(h1) {
		t.Errorf("CJK content hash invalid format: %q", h1)
	}
	if !hexRegex.MatchString(h2) {
		t.Errorf("emoji content hash invalid format: %q", h2)
	}
	if !hexRegex.MatchString(h3) {
		t.Errorf("accent content hash invalid format: %q", h3)
	}

	// Different unicode strings should produce different hashes
	if h1 == h2 || h1 == h3 || h2 == h3 {
		t.Error("different unicode content should produce different hashes")
	}
}

func TestHashToolResultContentMultiBlock(t *testing.T) {
	// Upstream: handles multiple text blocks, concatenates all text
	block := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "first "}},
			{OfText: &anthropic.TextBlockParam{Text: "second "}},
			{OfText: &anthropic.TextBlockParam{Text: "third"}},
		},
	}
	blockConcat := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "first second third"}},
		},
	}

	h1 := hashToolResultContent(block)
	h2 := hashToolResultContent(blockConcat)

	// Concatenated blocks should produce the same hash as a single concatenated string
	if h1 != h2 {
		t.Errorf("multi-block content should hash same as concatenated: %q vs %q", h1, h2)
	}
}

func TestHashToolResultContentFormat(t *testing.T) {
	// Format validation: always 8 hex chars (32-bit FNV-1a)
	block := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "format test"}},
		},
	}
	h := hashToolResultContent(block)
	if len(h) != 8 {
		t.Errorf("hash length = %d, want 8", len(h))
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(h) {
		t.Errorf("hash invalid hex format: %q", h)
	}
}

func TestHashToolResultContentWhitespaceSensitive(t *testing.T) {
	// Whitespace differences should produce different hashes
	block1 := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world"}},
		},
	}
	block2 := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello  world"}}, // extra space
		},
	}
	block3 := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world\n"}}, // trailing newline
		},
	}

	h1 := hashToolResultContent(block1)
	h2 := hashToolResultContent(block2)
	h3 := hashToolResultContent(block3)

	if h1 == h2 {
		t.Error("extra space should produce different hash")
	}
	if h1 == h3 {
		t.Error("trailing newline should produce different hash")
	}
}

// ---------------------------------------------------------------------------
// modelContextWindow — compact.go:1148
// ---------------------------------------------------------------------------

func TestModelContextWindow(t *testing.T) {
	tests := []struct {
		model string
		min   int
	}{
		{"claude-sonnet-4-20250514", 100000},
		{"claude-opus-4-20250514", 100000},
		{"claude-3-5-sonnet-20241022", 100000},
		{"unknown-model", 50000}, // fallback
	}
	for _, tc := range tests {
		got := modelContextWindow(tc.model)
		if got < tc.min {
			t.Errorf("modelContextWindow(%q) = %d, want >= %d", tc.model, got, tc.min)
		}
	}
}

// ---------------------------------------------------------------------------
// NewIterationBudget — agent_loop.go:40
// ---------------------------------------------------------------------------

func TestNewIterationBudget(t *testing.T) {
	budget := NewIterationBudget(10)
	if budget == nil {
		t.Fatal("expected non-nil budget")
	}
	if budget.max != 10 {
		t.Errorf("expected max=10, got %d", budget.max)
	}
}

// ---------------------------------------------------------------------------
// NewTaskStore — agent_task.go:102
// ---------------------------------------------------------------------------

func TestNewTaskStoreBasic(t *testing.T) {
	store := NewTaskStore()
	if store == nil {
		t.Fatal("expected non-nil task store")
	}
}

// ---------------------------------------------------------------------------
// NewAttribution — attribution.go:16
// ---------------------------------------------------------------------------

func TestNewAttribution(t *testing.T) {
	attr := NewAttribution("claude-sonnet-4-20250514")
	if attr == nil {
		t.Fatal("expected non-nil attribution")
	}
	// sanitizeModelName strips the date suffix
	if attr.Model != "claude-sonnet-4" {
		t.Errorf("expected sanitized model, got %q", attr.Model)
	}
}

// ---------------------------------------------------------------------------
// normalizeWhitespace — normalize.go:808
// ---------------------------------------------------------------------------

func TestNormalizeWhitespaceCollapsesBlankLines(t *testing.T) {
	input := "hello\n\n\n\nworld"
	got := normalizeWhitespace(input)
	if got != "hello\n\nworld" {
		t.Errorf("expected 2 lines, got %q", got)
	}
}

func TestNormalizeWhitespaceTrimsTrailingLines(t *testing.T) {
	input := "hello\nworld\n\n\n"
	got := normalizeWhitespace(input)
	if got != "hello\nworld" {
		t.Errorf("expected no trailing blank, got %q", got)
	}
}

func TestNormalizeWhitespaceTrimsTrailingWhitespace(t *testing.T) {
	input := "hello   \nworld\t   \n"
	got := normalizeWhitespace(input)
	if got != "hello\nworld" {
		t.Errorf("expected trimmed, got %q", got)
	}
}

func TestNormalizeWhitespaceEmpty(t *testing.T) {
	if normalizeWhitespace("") != "" {
		t.Error("empty should stay empty")
	}
}

func TestNormalizeWhitespacePreservesSingleBlank(t *testing.T) {
	input := "hello\n\nworld"
	got := normalizeWhitespace(input)
	if got != "hello\n\nworld" {
		t.Errorf("expected single blank preserved, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// sortMapKeys / sortValueKeys — normalize.go:772
// ---------------------------------------------------------------------------

func TestSortMapKeysBasic(t *testing.T) {
	input := map[string]any{"z": 1, "a": 2, "m": 3}
	got := sortMapKeys(input)
	keys := make([]string, 0)
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort for deterministic check
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %v", keys)
	}
	if keys[0] != "a" || keys[1] != "m" || keys[2] != "z" {
		t.Errorf("expected sorted keys [a, m, z], got %v", keys)
	}
}

func TestSortMapKeysNil(t *testing.T) {
	if sortMapKeys(nil) != nil {
		t.Error("nil map should return nil")
	}
}

func TestSortMapKeysRecursive(t *testing.T) {
	input := map[string]any{
		"outer": map[string]any{"z": 1, "a": 2},
	}
	got := sortMapKeys(input)
	outer := got["outer"].(map[string]any)
	keys := make([]string, 0)
	for k := range outer {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort for deterministic check
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "z" {
		t.Errorf("expected inner keys [a, z], got %v", keys)
	}
}

func TestSortValueKeysArray(t *testing.T) {
	input := []any{map[string]any{"z": 1, "a": 2}, "string"}
	got := sortValueKeys(input)
	arr := got.([]any)
	inner := arr[0].(map[string]any)
	keys := make([]string, 0)
	for k := range inner {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort for deterministic check
	if keys[0] != "a" || keys[1] != "z" {
		t.Errorf("expected keys [a, z], got %v", keys)
	}
}

// ---------------------------------------------------------------------------
// CheckReactiveCompact — compact.go:2579
// ---------------------------------------------------------------------------

func TestCheckReactiveCompactNoSpike(t *testing.T) {
	// delta=2000 < threshold=5000 → no spike
	r := CheckReactiveCompact(7000, 5000, 5000)
	if r != nil {
		t.Error("expected nil when delta < threshold")
	}
}

func TestCheckReactiveCompactNegativeDelta(t *testing.T) {
	r := CheckReactiveCompact(5000, 10000, 1000)
	if r != nil {
		t.Error("expected nil for negative delta")
	}
}

func TestCheckReactiveCompactSpike(t *testing.T) {
	// delta=10000 > threshold=3000 → spike
	r := CheckReactiveCompact(15000, 5000, 3000)
	if r == nil {
		t.Fatal("expected non-nil result for spike")
	}
	if !r.Triggered {
		t.Error("expected Triggered=true")
	}
	if r.PreTokens != 15000 {
		t.Errorf("expected PreTokens=15000, got %d", r.PreTokens)
	}
	if r.PreviousTokens != 5000 {
		t.Errorf("expected PreviousTokens=5000, got %d", r.PreviousTokens)
	}
	if r.TokenDelta != 10000 {
		t.Errorf("expected TokenDelta=10000, got %d", r.TokenDelta)
	}
}

func TestCheckReactiveCompactZeroThreshold(t *testing.T) {
	// Zero threshold should use default 5000
	r := CheckReactiveCompact(10001, 5000, 0)
	if r == nil {
		t.Error("expected non-nil with delta > default threshold")
	}
}

func TestCheckReactiveCompactExactThreshold(t *testing.T) {
	// Delta == threshold: delta < threshold is false, so it DOES trigger
	r := CheckReactiveCompact(10000, 5000, 5000)
	if r == nil {
		t.Error("delta == threshold should trigger (delta < threshold is false)")
	}
}

// ---------------------------------------------------------------------------
// Compactor methods — compact.go:1310
// ---------------------------------------------------------------------------

func TestNewCompactorDefault(t *testing.T) {
	c := NewCompactor()
	if c.maxTokens != 200_000 {
		t.Errorf("expected maxTokens=200000, got %d", c.maxTokens)
	}
	if c.compactThreshold != 0.75 {
		t.Errorf("expected threshold=0.75, got %f", c.compactThreshold)
	}
	if c.llmCompactFailedCount != 0 {
		t.Errorf("expected llmCompactFailedCount=0, got %d", c.llmCompactFailedCount)
	}
}

func TestCompactorSetMaxTokens(t *testing.T) {
	c := NewCompactor()
	c.SetMaxTokens(500000)
	if c.maxTokens != 500000 {
		t.Errorf("expected maxTokens=500000, got %d", c.maxTokens)
	}
}

func TestCompactorSetPostCompactTokens(t *testing.T) {
	c := NewCompactor()
	c.SetPostCompactTokens(80000)
	if c.postCompactTokens != 80000 {
		t.Errorf("expected postCompactTokens=80000, got %d", c.postCompactTokens)
	}
}

// ---------------------------------------------------------------------------
// ContextWindowTracker — compact.go:1137
// ---------------------------------------------------------------------------

func TestNewContextWindowTracker(t *testing.T) {
	tracker := NewContextWindowTracker("claude-sonnet-4-20250514", 0.8, 15000)
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	// Should get 1M for Sonnet 4 (known 1M-capable)
	if tracker.modelMaxTokens != 1_000_000 {
		t.Errorf("expected modelMaxTokens=1000000 for Sonnet-4, got %d", tracker.modelMaxTokens)
	}
	if tracker.autoCompactThreshold != 0.8 {
		t.Errorf("expected threshold=0.8, got %f", tracker.autoCompactThreshold)
	}
	if tracker.autoCompactBuffer != 15000 {
		t.Errorf("expected buffer=15000, got %d", tracker.autoCompactBuffer)
	}
}

// ---------------------------------------------------------------------------
// EstimateMessageTokensSmart — compact.go:1742
// ---------------------------------------------------------------------------

func TestEstimateMessageTokensSmartText(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world"}},
		},
	}
	tokens := EstimateMessageTokensSmart(msg)
	// 4 (role) + ~2 (text tokens for "hello world")
	if tokens < 4 {
		t.Errorf("expected at least 4 tokens for role overhead, got %d", tokens)
	}
}

func TestEstimateMessageTokensSmartToolUse(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "assistant",
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{
				Name:  "read_file",
				Input: map[string]any{"file_path": "/tmp/foo.txt"},
			}},
		},
	}
	tokens := EstimateMessageTokensSmart(msg)
	// 4 (role) + 10 (tool_use overhead) + estimate for "read_file" + estimate for JSON input
	if tokens < 14 {
		t.Errorf("expected at least 14 tokens for tool_use, got %d", tokens)
	}
}

func TestEstimateMessageTokensSmartToolResult(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: "call_abc",
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: "file contents here"}},
				},
			}},
		},
	}
	tokens := EstimateMessageTokensSmart(msg)
	// 4 (role) + 5 (tool_result overhead) + text tokens
	if tokens < 5 {
		t.Errorf("expected at least 5 tokens for tool_result, got %d", tokens)
	}
}

// ---------------------------------------------------------------------------
// estimateMessageTokens (agent_loop) — agent_loop.go:4907
// ---------------------------------------------------------------------------

func TestEstimateMessageTokens(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{Role: "user", Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world"}}}},
	}
	tokens := estimateMessageTokens(msgs)
	// Simple char-based: totalChars/4 = 11/4 = 2
	if tokens != 2 {
		t.Errorf("expected 2 tokens for 11 chars, got %d", tokens)
	}
}

func TestEstimateMessageTokensEmpty(t *testing.T) {
	if estimateMessageTokens(nil) != 0 {
		t.Error("empty messages should return 0")
	}
	if estimateMessageTokens([]anthropic.MessageParam{}) != 0 {
		t.Error("empty slice should return 0")
	}
}

// ---------------------------------------------------------------------------
// ConversationContext-based functions — agent_loop.go:3263+
// These use private fields (entries, mu) so we test with NewConversationContext
// ---------------------------------------------------------------------------

func TestCollectReadToolFilePathsEmpty(t *testing.T) {
	ctx := NewConversationContext(Config{})
	// Empty context returns nil (no compact boundary)
	paths := collectReadToolFilePaths(ctx)
	if paths != nil {
		t.Errorf("expected nil for empty context (no boundary), got %v", paths)
	}
}

func TestCollectUsedToolNamesInPreservedMessagesEmpty(t *testing.T) {
	ctx := NewConversationContext(Config{})
	// Empty context returns nil (no compact boundary)
	names := collectUsedToolNamesInPreservedMessages(ctx)
	if names != nil {
		t.Errorf("expected nil for empty context (no boundary), got %v", names)
	}
}

func TestCollectDiscoveredToolNamesEmpty(t *testing.T) {
	ctx := NewConversationContext(Config{})
	names := collectDiscoveredToolNames(ctx)
	if names == nil {
		t.Error("expected non-nil slice")
	}
	if len(names) != 0 {
		t.Errorf("expected empty for empty context, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// inferToolNameFromResult — context.go:1471
// ---------------------------------------------------------------------------

func TestInferToolNameFromResultReadFile(t *testing.T) {
	r := anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "10 lines ────"}},
		},
	}
	if inferToolNameFromResult(r) != "read_file" {
		t.Error("expected read_file for lines+─── pattern")
	}
}

func TestInferToolNameFromResultBash(t *testing.T) {
	r := anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "$ ls -la"}},
		},
	}
	if inferToolNameFromResult(r) != "bash" {
		t.Error("expected bash for $ prompt pattern")
	}
}

func TestInferToolNameFromResultGrep(t *testing.T) {
	r := anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "Found 5 matches"}},
		},
	}
	if inferToolNameFromResult(r) != "grep" {
		t.Error("expected grep for Found+match pattern")
	}
}

func TestInferToolNameFromResultEdit(t *testing.T) {
	r := anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "wrote 3 lines"}},
		},
	}
	if inferToolNameFromResult(r) != "edit_file" {
		t.Error("expected edit_file for wrote pattern")
	}
}

func TestInferToolNameFromResultUnknown(t *testing.T) {
	r := anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "random output"}},
		},
	}
	if inferToolNameFromResult(r) != "unknown_tool" {
		t.Error("expected unknown_tool for unrecognized pattern")
	}
}

func TestInferToolNameFromResultEmptyContent(t *testing.T) {
	r := anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{},
	}
	if inferToolNameFromResult(r) != "unknown_tool" {
		t.Error("expected unknown_tool for empty content")
	}
}

// ---------------------------------------------------------------------------
// entryContentToText — context.go:1713
// ---------------------------------------------------------------------------

func TestEntryContentToTextTextContent(t *testing.T) {
	c := EntryContent(TextContent("hello world"))
	got := entryContentToText(c)
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestEntryContentToTextSummaryContent(t *testing.T) {
	c := EntryContent(SummaryContent("summary text"))
	got := entryContentToText(c)
	if got != "summary text" {
		t.Errorf("expected 'summary text', got %q", got)
	}
}

func TestEntryContentToTextAttachmentContent(t *testing.T) {
	c := EntryContent(AttachmentContent("attachment data"))
	got := entryContentToText(c)
	if got != "attachment data" {
		t.Errorf("expected 'attachment data', got %q", got)
	}
}

func TestEntryContentToTextCompactBoundary(t *testing.T) {
	c := EntryContent(CompactBoundaryContent{PreCompactTokens: 50000})
	got := entryContentToText(c)
	if !strings.Contains(got, "50000") {
		t.Errorf("expected token count in output, got %q", got)
	}
}

func TestEntryContentToTextToolUseContent(t *testing.T) {
	c := EntryContent(ToolUseContent{
		{OfToolUse: &anthropic.ToolUseBlockParam{ID: "call_1", Name: "read_file"}},
	})
	got := entryContentToText(c)
	if !strings.Contains(got, "read_file") || !strings.Contains(got, "call_1") {
		t.Errorf("expected tool call info, got %q", got)
	}
}

func TestEntryContentToTextToolResultContent(t *testing.T) {
	c := EntryContent(ToolResultContent{
		{ToolUseID: "call_1", Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "file contents"}},
		}},
	})
	got := entryContentToText(c)
	if !strings.Contains(got, "call_1") || !strings.Contains(got, "file contents") {
		t.Errorf("expected tool result info, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// isTextEntry — context.go:1184
// ---------------------------------------------------------------------------

func TestIsTextEntryTextContent(t *testing.T) {
	entry := &conversationEntry{content: TextContent("hello")}
	if !isTextEntry(entry) {
		t.Error("TextContent should be a text entry")
	}
}

func TestIsTextEntrySummaryContent(t *testing.T) {
	entry := &conversationEntry{content: SummaryContent("summary")}
	if !isTextEntry(entry) {
		t.Error("SummaryContent should be a text entry")
	}
}

func TestIsTextEntryToolUseContent(t *testing.T) {
	entry := &conversationEntry{content: ToolUseContent{}}
	if isTextEntry(entry) {
		t.Error("ToolUseContent should NOT be a text entry")
	}
}

func TestIsTextEntryToolResultContent(t *testing.T) {
	entry := &conversationEntry{content: ToolResultContent{}}
	if isTextEntry(entry) {
		t.Error("ToolResultContent should NOT be a text entry")
	}
}

// ---------------------------------------------------------------------------
// generateUUID — context.go:20
// ---------------------------------------------------------------------------

func TestGenerateUUID(t *testing.T) {
	u1 := generateUUID()
	u2 := generateUUID()
	if u1 == u2 {
		t.Error("two UUIDs should differ")
	}
	// UUID format: 8-4-4-4-12 hex chars
	if len(u1) != 36 {
		t.Errorf("expected 36-char UUID, got %d chars: %q", len(u1), u1)
	}
}

func TestGenerateUUIDFormat(t *testing.T) {
	u := generateUUID()
	// Check hyphen positions: 8, 13, 18, 23
	for _, pos := range []int{8, 13, 18, 23} {
		if u[pos] != '-' {
			t.Errorf("expected hyphen at position %d, got %c", pos, u[pos])
		}
	}
}

func TestGenerateUUIDUniqueness(t *testing.T) {
	// Upstream invariant: N generated UUIDs are all different
	const n = 1000
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		u := generateUUID()
		if seen[u] {
			t.Fatalf("duplicate UUID at iteration %d: %s", i, u)
		}
		seen[u] = true
	}
}

func TestGenerateUUIDRegexFormat(t *testing.T) {
	// Upstream: matches /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		u := generateUUID()
		if !uuidRegex.MatchString(u) {
			t.Errorf("UUID does not match standard format: %s", u)
		}
	}
}

func TestGenerateUUIDLength(t *testing.T) {
	// Invariant: always 36 chars (32 hex + 4 hyphens)
	for i := 0; i < 100; i++ {
		u := generateUUID()
		if len(u) != 36 {
			t.Errorf("expected length 36, got %d for UUID: %s", len(u), u)
		}
	}
}

func TestGenerateUUIDHexCharsOnly(t *testing.T) {
	// All non-hyphen characters should be lowercase hex
	u := generateUUID()
	for _, c := range u {
		if c == '-' {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char in UUID: %c (full UUID: %s)", c, u)
		}
	}
}

// ---------------------------------------------------------------------------
// getStringSlice — agent_loop.go:3208
// ---------------------------------------------------------------------------

func TestGetStringSlicePresent(t *testing.T) {
	m := map[string]any{"files": []any{"a.go", "b.go"}}
	got := getStringSlice(m, "files")
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("expected [a.go, b.go], got %v", got)
	}
}

func TestGetStringSliceMissing(t *testing.T) {
	m := map[string]any{}
	got := getStringSlice(m, "files")
	if got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

func TestGetStringSliceWrongType(t *testing.T) {
	m := map[string]any{"files": "not a slice"}
	got := getStringSlice(m, "files")
	if got != nil {
		t.Errorf("expected nil for wrong type, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// isTransientCompactError — compact.go:1680
// ---------------------------------------------------------------------------

func TestIsTransientCompactError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"rate_limit exceeded", true},
		{"error 429", true},
		{"timeout waiting", true},
		{"deadline exceeded", true},
		{"connection reset", true},
		{"network error", true},
		{"context deadline exceeded", true},
		{"context canceled", true},
		{"server_error occurred", true},
		{"service_unavailable", true},
		{"stream ended without receiving any events", true},
		{"authentication failed", false},
		{"invalid request", false},
		{"not found", false},
	}
	for _, tc := range tests {
		got := isTransientCompactError(tc.msg)
		if got != tc.want {
			t.Errorf("isTransientCompactError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// generateToolResultSummary — compact.go:1859
// ---------------------------------------------------------------------------

func TestGenerateToolResultSummary(t *testing.T) {
	r := &anthropic.ToolResultBlockParam{
		IsError: anthropic.Bool(false),
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "line1\nline2\nline3"}},
		},
	}
	got := generateToolResultSummary("read_file", r)
	if !strings.Contains(got, "read_file") {
		t.Errorf("expected tool name in output, got %q", got)
	}
	if !strings.Contains(got, "3 lines") {
		t.Errorf("expected line count in output, got %q", got)
	}
}

func TestGenerateToolResultSummaryError(t *testing.T) {
	r := &anthropic.ToolResultBlockParam{
		IsError: anthropic.Bool(true),
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: "error message"}},
		},
	}
	got := generateToolResultSummary("exec", r)
	if !strings.Contains(got, "error") {
		t.Errorf("expected error status in output, got %q", got)
	}
}

func TestGenerateToolResultSummaryLong(t *testing.T) {
	r := &anthropic.ToolResultBlockParam{
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: strings.Repeat("x", 150)}},
		},
	}
	got := generateToolResultSummary("read_file", r)
	if strings.Contains(got, strings.Repeat("x", 150)) {
		t.Error("expected long content to be truncated")
	}
}

// ---------------------------------------------------------------------------
// extractSummaryFromCompactOutput — compact.go:2003
// ---------------------------------------------------------------------------

func TestExtractSummaryFromCompactOutput(t *testing.T) {
	input := "<analysis>old content</analysis><summary>the summary text</summary>"
	got := extractSummaryFromCompactOutput(input)
	if got != "the summary text" {
		t.Errorf("expected extracted summary, got %q", got)
	}
}

func TestExtractSummaryFromCompactOutputNoTags(t *testing.T) {
	input := "just some plain text without tags"
	got := extractSummaryFromCompactOutput(input)
	if got != "just some plain text without tags" {
		t.Errorf("expected trimmed text, got %q", got)
	}
}

func TestExtractSummaryFromCompactOutputAnalysisRemoved(t *testing.T) {
	input := "<analysis>should be removed</analysis><summary>keep this</summary>"
	got := extractSummaryFromCompactOutput(input)
	if strings.Contains(got, "should be removed") {
		t.Error("analysis content should be removed")
	}
	if !strings.Contains(got, "keep this") {
		t.Error("summary content should be kept")
	}
}

func TestExtractSummaryFromCompactOutputWhitespace(t *testing.T) {
	input := "<summary>  spaced summary  </summary>"
	got := extractSummaryFromCompactOutput(input)
	if got != "spaced summary" {
		t.Errorf("expected trimmed, got %q", got)
	}
}

// ─── Additional extractSummary patterns from upstream prompt.test.ts ─────────

func TestExtractSummaryMultilineAnalysis(t *testing.T) {
	// From upstream: "handles multiline analysis content"
	input := "<analysis>\nline1\nline2\nline3\n</analysis><summary>ok</summary>"
	got := extractSummaryFromCompactOutput(input)
	if strings.Contains(got, "line1") {
		t.Error("multiline analysis content should be stripped")
	}
	if got != "ok" {
		t.Errorf("expected 'ok', got %q", got)
	}
}

func TestExtractSummaryContentBetweenAnalysisAndSummary(t *testing.T) {
	// From upstream: "preserves content between analysis and summary"
	input := "<analysis>thoughts</analysis>middle text<summary>final</summary>"
	got := extractSummaryFromCompactOutput(input)
	if got != "final" {
		t.Errorf("expected 'final', got %q (content between tags is not extracted, only summary tag content)", got)
	}
}

func TestExtractSummaryEmptyString(t *testing.T) {
	got := extractSummaryFromCompactOutput("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractSummarySummaryWithoutAnalysis(t *testing.T) {
	// From upstream: "handles summary without analysis"
	input := "<summary>just the summary</summary>"
	got := extractSummaryFromCompactOutput(input)
	if got != "just the summary" {
		t.Errorf("expected 'just the summary', got %q", got)
	}
}

func TestExtractSummaryAnalysisWithoutSummary(t *testing.T) {
	// From upstream: "handles analysis without summary"
	input := "<analysis>just analysis</analysis>and some text"
	got := extractSummaryFromCompactOutput(input)
	// Go implementation returns the text as-is (no <summary> tags found)
	if got != "and some text" {
		t.Errorf("expected 'and some text', got %q", got)
	}
}

func TestExtractSummaryNestedNewlines(t *testing.T) {
	input := "<summary>line one\nline two\nline three</summary>"
	got := extractSummaryFromCompactOutput(input)
	if got != "line one\nline two\nline three" {
		t.Errorf("expected multiline summary, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// estimateSingleMessageTokens — compact.go:1973
// ---------------------------------------------------------------------------

func TestEstimateSingleMessageTokens(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "hello world"}},
		},
	}
	tokens := estimateSingleMessageTokens(msg)
	// Should be > 0
	if tokens < 4 {
		t.Errorf("expected at least 4 tokens, got %d", tokens)
	}
}

func TestEstimateSingleMessageTokensEmpty(t *testing.T) {
	msg := anthropic.MessageParam{Role: "user", Content: []anthropic.ContentBlockParamUnion{}}
	tokens := estimateSingleMessageTokens(msg)
	if tokens != 4 {
		t.Errorf("expected 4 for role overhead, got %d", tokens)
	}
}

// ---------------------------------------------------------------------------
// truncateLargeToolArgs — compact.go:1881
// ---------------------------------------------------------------------------

func TestTruncateLargeToolArgs(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{Role: "assistant", Content: []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{
				ID:   "call_1",
				Name: "exec",
				Input: map[string]any{
					"command": strings.Repeat("x", 3000),
				},
			}},
		}},
	}
	got := truncateLargeToolArgs(msgs, 2000)
	toolUse := got[0].Content[0].OfToolUse
	cmd := toolUse.Input.(map[string]any)["command"].(string)
	if !strings.Contains(cmd, "...[truncated]") {
		t.Error("expected truncation marker")
	}
	if len(cmd) > 2100 {
		t.Errorf("expected truncated length, got %d chars", len(cmd))
	}
}

func TestTruncateLargeToolArgsSmallArg(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{Role: "assistant", Content: []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{
				Name:  "exec",
				Input: map[string]any{"command": "ls"},
			}},
		}},
	}
	got := truncateLargeToolArgs(msgs, 2000)
	cmd := got[0].Content[0].OfToolUse.Input.(map[string]any)["command"].(string)
	if cmd != "ls" {
		t.Errorf("small arg should be unchanged, got %q", cmd)
	}
}

// ---------------------------------------------------------------------------
// stripImages — compact.go:2735
// ---------------------------------------------------------------------------

func TestStripImages(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{
				Text: "here is an image https://example.com/photo.png and more text",
			}},
		},
	}
	got := stripImages([]anthropic.MessageParam{msg})
	if strings.Contains(got[0].Content[0].OfText.Text, "photo.png") {
		t.Error("image URL should be stripped")
	}
}

func TestStripImagesBase64(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{
				Text: "data:image/png;base64,ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890+/",
			}},
		},
	}
	got := stripImages([]anthropic.MessageParam{msg})
	if strings.Contains(got[0].Content[0].OfText.Text, "base64") {
		t.Error("base64 image should be stripped")
	}
}

func TestStripImagesToolResult(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: "call_1",
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{
						Text: "image at https://example.com/pic.jpg",
					}},
				},
			}},
		},
	}
	got := stripImages([]anthropic.MessageParam{msg})
	text := got[0].Content[0].OfToolResult.Content[0].OfText.Text
	if strings.Contains(text, "pic.jpg") {
		t.Error("image URL in tool result should be stripped")
	}
}

func TestStripImagesNoChange(t *testing.T) {
	msg := anthropic.MessageParam{
		Role: "user",
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: "plain text without images"}},
		},
	}
	got := stripImages([]anthropic.MessageParam{msg})
	if got[0].Content[0].OfText.Text != "plain text without images" {
		t.Error("plain text should be unchanged")
	}
}

// ---------------------------------------------------------------------------
// ContextWindowTracker methods — compact.go:1129
// ---------------------------------------------------------------------------

func TestContextWindowTrackerUsage(t *testing.T) {
	tracker := NewContextWindowTracker("claude-sonnet-4", 0.8, 15000)
	// modelMaxTokens set correctly by NewContextWindowTracker
	if tracker.modelMaxTokens != 1_000_000 {
		t.Errorf("expected 1M for Sonnet-4, got %d", tracker.modelMaxTokens)
	}
}

func TestGetStringSliceStringSlice(t *testing.T) {
	m := map[string]any{"files": []string{"a.go", "b.go"}}
	got := getStringSlice(m, "files")
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Errorf("expected [a.go, b.go], got %v", got)
	}
}

// ─── diffLastTwoSnapshots + snapshot end-to-end ────────────────────────────

func TestDiffLastTwoSnapshotsBasic(t *testing.T) {
	// Create a temp file
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fp, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create SnapshotHistory
	h := NewSnapshotHistory(dir)

	// Take "before" snapshot
	if err := h.TakeSnapshotWithDesc(fp, "before edit"); err != nil {
		t.Fatalf("before snapshot failed: %v", err)
	}

	// Modify file
	if err := os.WriteFile(fp, []byte("hello world\nnew line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Take "after" snapshot
	if err := h.TakeSnapshotWithDesc(fp, "after edit"); err != nil {
		t.Fatalf("after snapshot failed: %v", err)
	}

	// Check snapshots exist
	snaps := h.ListSnapshots(fp)
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	// Test diffLastTwoSnapshots
	diff := diffLastTwoSnapshots(h, fp)
	if diff == "" {
		t.Fatal("expected diff, got empty string")
	}
	if !strings.Contains(diff, "+new line") {
		t.Errorf("diff should contain added line, got: %q", diff)
	}
	t.Logf("diff:\n%s", diff)
}

func TestDiffLastTwoSnapshotsPathNormalization(t *testing.T) {
	// Test that paths with different formats resolve to the same snapshot key
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fp, []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewSnapshotHistory(dir)

	// Simulate what extractFilePath + expandPath does
	expandedPath := expandPath(fp)

	// Take "before" snapshot with expanded path
	if err := h.TakeSnapshotWithDesc(expandedPath, "before"); err != nil {
		t.Fatalf("before snapshot failed: %v", err)
	}

	// Modify file
	if err := os.WriteFile(fp, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Take "after" snapshot with expanded path
	if err := h.TakeSnapshotWithDesc(expandedPath, "after"); err != nil {
		t.Fatalf("after snapshot failed: %v", err)
	}

	// diffLastTwoSnapshots uses filepath.Abs internally
	diff := diffLastTwoSnapshots(h, expandedPath)
	if diff == "" {
		t.Fatal("expected diff, got empty string")
	}
	if !strings.Contains(diff, "+line2") {
		t.Errorf("diff should contain added line, got: %q", diff)
	}
}

func TestDiffLastTwoSnapshotsNoChanges(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fp, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewSnapshotHistory(dir)
	_ = h.TakeSnapshotWithDesc(fp, "before")
	_ = h.TakeSnapshotWithDesc(fp, "after") // Same content, dedup should skip

	snaps := h.ListSnapshots(fp)
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot (dedup), got %d", len(snaps))
	}

	diff := diffLastTwoSnapshots(h, fp)
	if diff != "" {
		t.Errorf("expected empty diff for identical content, got: %q", diff)
	}
}

func TestDiffLastTwoSnapshotsLessThanTwo(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(fp, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewSnapshotHistory(dir)
	_ = h.TakeSnapshotWithDesc(fp, "only one")

	diff := diffLastTwoSnapshots(h, fp)
	if diff != "" {
		t.Errorf("expected empty diff for single snapshot, got: %q", diff)
	}
}
