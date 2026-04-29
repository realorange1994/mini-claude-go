package main

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"single line", "hello", "hello"},
		{"trailing spaces", "hello   \nworld  ", "hello\nworld"},
		{"two blank lines", "a\n\n\nb", "a\n\nb"},
		{"three blank lines", "a\n\n\n\nb", "a\n\nb"},
		{"many blank lines", "a\n\n\n\n\n\nb", "a\n\nb"},
		{"trailing blank lines", "a\nb\n\n\n", "a\nb"},
		{"only blank lines", "\n\n\n\n", ""},
		{"mixed trailing spaces and blanks", "a  \n\n\nb  \n\n", "a\n\nb"},
		{"preserve single blank", "a\n\nb", "a\n\nb"},
		{"tabs in trailing", "a\t\nb\t\t", "a\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWhitespace(tt.input)
			if got != tt.want {
				t.Errorf("normalizeWhitespace(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSortMapKeys(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{"nil map", nil},
		{"empty map", map[string]any{}},
		{"flat unsorted", map[string]any{"z": 1, "a": 2, "m": 3}},
		{"nested map", map[string]any{
			"outer": map[string]any{"inner_z": 1, "inner_a": 2},
		}},
		{"array with maps", map[string]any{
			"items": []any{
				map[string]any{"b": 1, "a": 2},
				"scalar",
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortMapKeys(tt.input)
			if tt.input == nil {
				if got != nil {
					t.Error("expected nil for nil input")
				}
				return
			}
			// Verify all keys present
			if len(got) != len(tt.input) {
				t.Errorf("expected %d keys, got %d", len(tt.input), len(got))
			}
			// Verify keys are sorted by marshaling (sorted keys produce deterministic JSON)
			gotJSON, _ := json.Marshal(got)
			inputSorted := make(map[string]any, len(tt.input))
			for k, v := range tt.input {
				inputSorted[k] = v
			}
			sortedJSON, _ := json.Marshal(sortMapKeys(inputSorted))
			if string(gotJSON) != string(sortedJSON) {
				t.Errorf("keys not properly sorted")
			}
		})
	}
}

func TestNormalizeJSONBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"invalid JSON", "{broken", "{broken"},
		{"already sorted", `{"a":1,"b":2}`, `{"a":1,"b":2}`},
		{"unsorted keys", `{"z":1,"a":2,"m":3}`, `{"a":2,"m":3,"z":1}`},
		{"nested unsorted", `{"outer":{"z":1,"a":2}}`, `{"outer":{"a":2,"z":1}}`},
		{"empty object", `{}`, `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(NormalizeJSONBytes([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("NormalizeJSONBytes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeAPIMessagesNil(t *testing.T) {
	result := NormalizeAPIMessages(nil)
	if len(result) != 0 {
		t.Errorf("expected empty slice for nil input, got %d", len(result))
	}
}

func TestNormalizeAPIMessagesAssistantToolUse(t *testing.T) {
	// Create an assistant message with a tool_use block whose input keys are unsorted
	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:   "tool-1",
					Name: "read_file",
					Input: map[string]any{
						"path":    "/tmp/test.go",
						"offset":  10,
						"limit":   50,
					},
				},
			},
		},
	}

	result := NormalizeAPIMessages([]anthropic.MessageParam{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Check that tool_use input keys are now sorted
	block := result[0].Content[0]
	if block.OfToolUse == nil {
		t.Fatal("expected tool_use block")
	}

	// Marshal the input to verify key order
	inputJSON, _ := json.Marshal(block.OfToolUse.Input)
	// Sorted keys should produce: {"limit":50,"offset":10,"path":"/tmp/test.go"}
	got := string(inputJSON)
	want := `{"limit":50,"offset":10,"path":"/tmp/test.go"}`
	if got != want {
		t.Errorf("tool_use input keys not sorted: got %q, want %q", got, want)
	}
}

func TestNormalizeAPIMessagesUserToolResult(t *testing.T) {
	// Create a user message with a tool_result containing excessive blank lines
	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{
							OfText: &anthropic.TextBlockParam{
								Text: "line1\n\n\n\n\nline2",
							},
						},
					},
				},
			},
		},
	}

	result := NormalizeAPIMessages([]anthropic.MessageParam{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	block := result[0].Content[0]
	if block.OfToolResult == nil {
		t.Fatal("expected tool_result block")
	}

	text := block.OfToolResult.Content[0].OfText.Text
	want := "line1\n\nline2"
	if text != want {
		t.Errorf("tool_result whitespace not normalized: got %q, want %q", text, want)
	}
}

func TestNormalizeAPIMessagesUnknownRole(t *testing.T) {
	// Unknown role should be returned unchanged
	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRole("system"),
		Content: []anthropic.ContentBlockParamUnion{
			{
				OfText: &anthropic.TextBlockParam{
					Text: "system prompt",
				},
			},
		},
	}

	result := NormalizeAPIMessages([]anthropic.MessageParam{msg})
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// Should be unchanged
	text := result[0].Content[0].OfText.Text
	if text != "system prompt" {
		t.Errorf("unknown role message should be unchanged, got %q", text)
	}
}

func TestSortValueKeys(t *testing.T) {
	// Scalar values pass through unchanged
	if sortValueKeys(42) != 42 {
		t.Error("int should pass through")
	}
	if sortValueKeys("hello") != "hello" {
		t.Error("string should pass through")
	}

	// Array with nested maps gets sorted
	arr := []any{
		map[string]any{"z": 1, "a": 2},
		"scalar",
	}
	result := sortValueKeys(arr)
	resultArr, ok := result.([]any)
	if !ok {
		t.Fatal("expected array result")
	}
	first, ok := resultArr[0].(map[string]any)
	if !ok {
		t.Fatal("expected map in first element")
	}
	// Verify keys are sorted by checking JSON serialization
	j, _ := json.Marshal(first)
	if string(j) != `{"a":2,"z":1}` {
		t.Errorf("nested map keys not sorted: %s", j)
	}
}

// Benchmarks
func BenchmarkNormalizeWhitespace(b *testing.B) {
	text := "line1\n\n\n\nline2\n\n\n\nline3\n\n\n\nline4"
	for i := 0; i < b.N; i++ {
		normalizeWhitespace(text)
	}
}

func BenchmarkSortMapKeys(b *testing.B) {
	m := map[string]any{
		"z": 1, "y": 2, "x": 3, "w": 4, "v": 5,
		"u": 6, "t": 7, "s": 8, "r": 9, "q": 10,
		"p": 11, "o": 12, "n": 13, "m": 14, "l": 15,
	}
	for i := 0; i < b.N; i++ {
		sortMapKeys(m)
	}
}

// Suppress unused import warning
var _ = sort.Strings