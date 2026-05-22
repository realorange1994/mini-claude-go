package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
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
	// Create an assistant message with a tool_use block whose input keys are unsorted.
	// After normalization pipeline:
	// 1. EnforceRoleAlternation: prepends synthetic user (first msg is assistant)
	// 2. EnsureToolResultPairing: appends synthetic user (orphan tool_use gets tool_result)
	// 3. FilterEmptyMessages: keeps everything (assistant has tool_use, not empty)
	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:   "tool-1",
					Name: "read_file",
					Input: map[string]any{
						"path":   "/tmp/test.go",
						"offset": 10,
						"limit":  50,
					},
				},
			},
		},
	}

	result := NormalizeAPIMessages([]anthropic.MessageParam{msg})
	// 3 messages: synthetic user (prefix), assistant, synthetic user (tool_result)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// First message should be synthetic user from role alternation
	if result[0].Role != anthropic.MessageParamRoleUser {
		t.Fatal("expected first message to be user (synthetic from alternation)")
	}

	// Second message should be the assistant with sorted tool_use keys
	if result[1].Role != anthropic.MessageParamRoleAssistant {
		t.Fatal("expected second message to be assistant")
	}
	block := result[1].Content[0]
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

	// Third message should be synthetic user with tool_result
	if result[2].Role != anthropic.MessageParamRoleUser {
		t.Fatal("expected third message to be user (synthetic from pairing)")
	}
	if result[2].Content[0].OfToolResult == nil {
		t.Fatal("expected tool_result in third message")
	}
	if result[2].Content[0].OfToolResult.ToolUseID != "tool-1" {
		t.Errorf("expected tool_result for tool-1, got %s", result[2].Content[0].OfToolResult.ToolUseID)
	}
}

func TestNormalizeAPIMessagesUserToolResult(t *testing.T) {
	// Create a user message with a tool_result containing excessive blank lines.
	// Include a matching assistant tool_use so the tool_result isn't stripped.
	assistantMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool-1",
					Name:  "read_file",
					Input: map[string]any{},
				},
			},
		},
	}
	userMsg := anthropic.MessageParam{
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

	result := NormalizeAPIMessages([]anthropic.MessageParam{assistantMsg, userMsg})
	// 3 messages: synthetic user (alternation prepended since first was assistant),
	// assistant with tool_use, user with tool_result
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// User message is third (synthetic user, assistant, original user)
	block := result[2].Content[0]
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

// ============================================================================
// Tests for P1-6a: StripVirtualMessages
// ============================================================================

func TestStripVirtualMessagesEmpty(t *testing.T) {
	result := StripVirtualMessages(nil)
	if len(result) != 0 {
		t.Errorf("expected empty for nil input, got %d", len(result))
	}
	result = StripVirtualMessages([]anthropic.MessageParam{})
	if len(result) != 0 {
		t.Errorf("expected empty for empty input, got %d", len(result))
	}
}

func TestStripVirtualMessagesVirtualOnly(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[virtual]"}},
			},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 0 {
		t.Errorf("expected 0 messages after stripping virtual, got %d", len(result))
	}
}

func TestStripVirtualMessagesSystemOnly(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[system]"}},
			},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 0 {
		t.Errorf("expected 0 messages after stripping system, got %d", len(result))
	}
}

func TestStripVirtualMessagesWhitespaceOnly(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "   "}},
			},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 0 {
		t.Errorf("expected 0 messages after stripping whitespace-only, got %d", len(result))
	}
}

func TestStripVirtualMessagesEmptyContent(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 0 {
		t.Errorf("expected 0 messages after stripping empty content, got %d", len(result))
	}
}

func TestStripVirtualMessagesMixedVirtualBlocks(t *testing.T) {
	// All text blocks are virtual markers → should be stripped
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[virtual]"}},
				{OfText: &anthropic.TextBlockParam{Text: "[system]"}},
				{OfText: &anthropic.TextBlockParam{Text: ""}},
			},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestStripVirtualMessagesKeepsRealContent(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hello, this is real content"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[virtual]"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Response"}},
			},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Content[0].OfText.Text != "Hello, this is real content" {
		t.Errorf("expected first message to be kept, got %q", result[0].Content[0].OfText.Text)
	}
	if result[1].Role != anthropic.MessageParamRoleAssistant {
		t.Errorf("expected second message to be assistant")
	}
}

func TestStripVirtualMessagesKeepsNonTextBlocks(t *testing.T) {
	// A user message with an image block should NOT be stripped even if
	// it also has a [virtual] text block.
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[virtual]"}},
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "dGVzdA==",
							MediaType: anthropic.Base64ImageSourceMediaTypeImagePNG,
						},
					},
				}},
			},
		},
	}
	result := StripVirtualMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message (has non-text block), got %d", len(result))
	}
}

// ============================================================================
// Tests for P1-6b: ReorderAttachmentsForAPI
// ============================================================================

func TestReorderAttachmentsEmpty(t *testing.T) {
	result := ReorderAttachmentsForAPI(nil)
	if len(result) != 0 {
		t.Errorf("expected empty for nil input, got %d", len(result))
	}
}

func TestReorderAttachmentsNoAttachments(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hello"}},
			},
		},
	}
	result := ReorderAttachmentsForAPI(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content[0].OfText.Text != "Hello" {
		t.Errorf("expected text block first, got %+v", result[0].Content[0])
	}
}

func TestReorderAttachmentsImageAfterText(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Here's the image:"}},
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "dGVzdA==",
							MediaType: anthropic.Base64ImageSourceMediaTypeImagePNG,
						},
					},
				}},
			},
		},
	}
	result := ReorderAttachmentsForAPI(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result[0].Content))
	}
	// Image should be first after reordering
	if result[0].Content[0].OfImage == nil {
		t.Error("expected image block first after reorder")
	}
	if result[0].Content[1].OfText == nil {
		t.Error("expected text block second after reorder")
	}
}

func TestReorderAttachmentsDocumentAfterToolResult(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "result"}},
					},
				}},
				{OfDocument: &anthropic.DocumentBlockParam{
					Source: anthropic.DocumentBlockParamSourceUnion{
						OfBase64: &anthropic.Base64PDFSourceParam{
							Data: "dGVzdA==",
						},
					},
				}},
			},
		},
	}
	result := ReorderAttachmentsForAPI(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	// tool_result should be first after reordering (hoisted before attachments)
	if result[0].Content[0].OfToolResult == nil {
		t.Error("expected tool_result block first after reorder")
	}
	if result[0].Content[1].OfDocument == nil {
		t.Error("expected document block second after reorder")
	}
}

func TestReorderAttachmentsMultipleAttachments(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Text 1"}},
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/1.png"},
					},
				}},
				{OfText: &anthropic.TextBlockParam{Text: "Text 2"}},
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/2.png"},
					},
				}},
			},
		},
	}
	result := ReorderAttachmentsForAPI(msgs)
	if len(result[0].Content) != 4 {
		t.Fatalf("expected 4 content blocks, got %d", len(result[0].Content))
	}
	// First two should be images (in original order), then two text blocks
	if result[0].Content[0].OfImage == nil || result[0].Content[1].OfImage == nil {
		t.Error("expected images first after reorder")
	}
	if result[0].Content[2].OfText == nil || result[0].Content[3].OfText == nil {
		t.Error("expected text blocks after images after reorder")
	}
}

func TestReorderAttachmentsAssistantUnchanged(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Response"}},
			},
		},
	}
	result := ReorderAttachmentsForAPI(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content[0].OfText.Text != "Response" {
		t.Error("assistant message should be unchanged")
	}
}

func TestReorderAttachmentsSingleBlockNoChange(t *testing.T) {
	// Single content block — no reordering needed
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/img.png"},
					},
				}},
			},
		},
	}
	result := ReorderAttachmentsForAPI(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content[0].OfImage == nil {
		t.Error("expected image block")
	}
}

// ============================================================================
// Tests for P1-6c: ValidateImagesForAPI
// ============================================================================

func TestValidateImagesEmpty(t *testing.T) {
	msgs, reasons := ValidateImagesForAPI(nil)
	if len(msgs) != 0 {
		t.Errorf("expected empty for nil input, got %d", len(msgs))
	}
	if reasons != nil {
		t.Errorf("expected nil reasons for nil input, got %v", reasons)
	}
}

func TestValidateImagesValidBase64(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "dGVzdA==",
							MediaType: anthropic.Base64ImageSourceMediaTypeImagePNG,
						},
					},
				}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) > 0 {
		t.Errorf("expected no reasons for valid image, got %v", reasons)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 1 {
		t.Errorf("expected 1 content block, got %d", len(result[0].Content))
	}
}

func TestValidateImagesValidURL(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/img.png"},
					},
				}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) > 0 {
		t.Errorf("expected no reasons for valid URL image, got %v", reasons)
	}
	if len(result[0].Content) != 1 {
		t.Errorf("expected 1 content block, got %d", len(result[0].Content))
	}
}

func TestValidateImagesNoSource(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d: %v", len(reasons), reasons)
	}
	if len(result[0].Content) != 0 {
		t.Errorf("expected 0 content blocks after removing invalid image, got %d", len(result[0].Content))
	}
}

func TestValidateImagesEmptyBase64Data(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "",
							MediaType: anthropic.Base64ImageSourceMediaTypeImagePNG,
						},
					},
				}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d: %v", len(reasons), reasons)
	}
	if reasons[0] != "image base64 source has empty data" {
		t.Errorf("unexpected reason: %s", reasons[0])
	}
	if len(result[0].Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result[0].Content))
	}
}

func TestValidateImagesUnsupportedMediaType(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "dGVzdA==",
							MediaType: anthropic.Base64ImageSourceMediaType("image/bmp"),
						},
					},
				}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d: %v", len(reasons), reasons)
	}
	if len(result[0].Content) != 0 {
		t.Errorf("expected 0 content blocks after removing unsupported media type, got %d", len(result[0].Content))
	}
}

func TestValidateImagesEmptyURL(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfURL: &anthropic.URLImageSourceParam{URL: ""},
					},
				}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d: %v", len(reasons), reasons)
	}
	if reasons[0] != "image URL source has empty url" {
		t.Errorf("unexpected reason: %s", reasons[0])
	}
	if len(result[0].Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result[0].Content))
	}
}

func TestValidateImagesMixed(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Check this image:"}},
				{OfImage: &anthropic.ImageBlockParam{}}, // invalid: no source
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/valid.png"},
					},
				}},
			},
		},
	}
	result, reasons := ValidateImagesForAPI(msgs)
	if len(reasons) != 1 {
		t.Fatalf("expected 1 reason, got %d: %v", len(reasons), reasons)
	}
	if len(result[0].Content) != 2 {
		t.Errorf("expected 2 content blocks (text + valid image), got %d", len(result[0].Content))
	}
	// First should be text, second should be valid image
	if result[0].Content[0].OfText == nil {
		t.Error("expected text block first")
	}
	if result[0].Content[1].OfImage == nil {
		t.Error("expected valid image block second")
	}
}

func TestValidateImagesAllSupportedMediaTypes(t *testing.T) {
	mediaTypes := []anthropic.Base64ImageSourceMediaType{
		anthropic.Base64ImageSourceMediaTypeImageJPEG,
		anthropic.Base64ImageSourceMediaTypeImagePNG,
		anthropic.Base64ImageSourceMediaTypeImageGIF,
		anthropic.Base64ImageSourceMediaTypeImageWebP,
	}
	for _, mt := range mediaTypes {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfImage: &anthropic.ImageBlockParam{
						Source: anthropic.ImageBlockParamSourceUnion{
							OfBase64: &anthropic.Base64ImageSourceParam{
								Data:      "dGVzdA==",
								MediaType: mt,
							},
						},
					}},
				},
			},
		}
		result, reasons := ValidateImagesForAPI(msgs)
		if len(reasons) != 0 {
			t.Errorf("media type %s: expected no reasons, got %v", mt, reasons)
		}
		if len(result[0].Content) != 1 {
			t.Errorf("media type %s: expected 1 content block, got %d", mt, len(result[0].Content))
		}
	}
}

func TestIsValidImageBlockNil(t *testing.T) {
	valid, reason := isValidImageBlock(nil)
	if valid {
		t.Error("expected nil image block to be invalid")
	}
	if reason != "image block is nil" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

// ============================================================================
// Tests for P1-6d: StripImagesFromErrorToolResults
// ============================================================================

func TestStripImagesFromErrorToolResultsEmpty(t *testing.T) {
	result := StripImagesFromErrorToolResults(nil)
	if len(result) != 0 {
		t.Errorf("expected empty for nil input, got %d", len(result))
	}
}

func TestStripImagesFromErrorToolResultsNonError(t *testing.T) {
	// Non-error tool_result should keep images
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "result"}},
						{OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/img.png"},
							},
						}},
					},
				}},
			},
		},
	}
	result := StripImagesFromErrorToolResults(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content[0].OfToolResult.Content) != 2 {
		t.Errorf("expected 2 content blocks in non-error tool_result, got %d", len(result[0].Content[0].OfToolResult.Content))
	}
}

func TestStripImagesFromErrorToolResultsErrorWithImage(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					IsError:   param.Opt[bool]{Value: true},
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "Error occurred"}},
						{OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/img.png"},
							},
						}},
					},
				}},
			},
		},
	}
	result := StripImagesFromErrorToolResults(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	tr := result[0].Content[0].OfToolResult
	if len(tr.Content) != 1 {
		t.Fatalf("expected 1 content block (text only), got %d", len(tr.Content))
	}
	if tr.Content[0].OfText == nil {
		t.Error("expected text block to remain")
	}
	if tr.Content[0].OfText.Text != "Error occurred" {
		t.Errorf("expected 'Error occurred', got %q", tr.Content[0].OfText.Text)
	}
}

func TestStripImagesFromErrorToolResultsErrorWithDocument(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					IsError:   param.Opt[bool]{Value: true},
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "Error"}},
						{OfDocument: &anthropic.DocumentBlockParam{
							Source: anthropic.DocumentBlockParamSourceUnion{
								OfBase64: &anthropic.Base64PDFSourceParam{Data: "dGVzdA=="},
							},
						}},
					},
				}},
			},
		},
	}
	result := StripImagesFromErrorToolResults(msgs)
	tr := result[0].Content[0].OfToolResult
	if len(tr.Content) != 1 {
		t.Fatalf("expected 1 content block (text only), got %d", len(tr.Content))
	}
	if tr.Content[0].OfText == nil {
		t.Error("expected text block to remain after document stripped")
	}
}

func TestStripImagesFromErrorToolResultsPreservesNonErrorInSameMessage(t *testing.T) {
	// Message with both error and non-error tool_results
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					IsError:   param.Opt[bool]{Value: true},
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "Error"}},
						{OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/err.png"},
							},
						}},
					},
				}},
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-2",
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "Success"}},
						{OfImage: &anthropic.ImageBlockParam{
							Source: anthropic.ImageBlockParamSourceUnion{
								OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/ok.png"},
							},
						}},
					},
				}},
			},
		},
	}
	result := StripImagesFromErrorToolResults(msgs)
	// First tool_result (error) should have image stripped
	tr1 := result[0].Content[0].OfToolResult
	if len(tr1.Content) != 1 {
		t.Errorf("expected 1 content block in error tool_result, got %d", len(tr1.Content))
	}
	// Second tool_result (non-error) should keep image
	tr2 := result[0].Content[1].OfToolResult
	if len(tr2.Content) != 2 {
		t.Errorf("expected 2 content blocks in non-error tool_result, got %d", len(tr2.Content))
	}
}

func TestStripImagesFromErrorToolResultsNoErrorToolResults(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hello"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hi"}},
			},
		},
	}
	result := StripImagesFromErrorToolResults(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
}

// ============================================================================
// Integration tests for NormalizeAPIMessages with new pipeline stages
// ============================================================================

func TestNormalizeAPIMessagesStripsVirtualThenAlternates(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[virtual]"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hello"}},
			},
		},
	}
	result := NormalizeAPIMessages(msgs)
	// Virtual message stripped → first is assistant → prepend synthetic user
	// Result: synthetic user, assistant
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != anthropic.MessageParamRoleUser {
		t.Error("expected first to be synthetic user")
	}
	if result[1].Role != anthropic.MessageParamRoleAssistant {
		t.Error("expected second to be assistant")
	}
}

func TestNormalizeAPIMessagesReordersAttachments(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "See image:"}},
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "dGVzdA==",
							MediaType: anthropic.Base64ImageSourceMediaTypeImagePNG,
						},
					},
				}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "I see it"}},
			},
		},
	}
	result := NormalizeAPIMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	// Image should be reordered to first position in user message
	if result[0].Content[0].OfImage == nil {
		t.Error("expected image block first after NormalizeAPIMessages reorder")
	}
	if result[0].Content[1].OfText == nil {
		t.Error("expected text block second after NormalizeAPIMessages reorder")
	}
}

func TestNormalizeAPIMessagesValidatesAndRemovesBadImages(t *testing.T) {
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfImage: &anthropic.ImageBlockParam{}}, // invalid: no source
				{OfText: &anthropic.TextBlockParam{Text: "Check this"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "I can't see it"}},
			},
		},
	}
	result := NormalizeAPIMessages(msgs)
	// Invalid image should be removed
	userContent := result[0].Content
	if len(userContent) != 1 {
		t.Fatalf("expected 1 content block (text only), got %d", len(userContent))
	}
	if userContent[0].OfText == nil {
		t.Error("expected text block to remain")
	}
}

func TestNormalizeAPIMessagesStripsImagesFromErrorToolResults(t *testing.T) {
	assistantMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    "tool-1",
				Name:  "read_file",
				Input: map[string]any{},
			}},
		},
	}
	userMsg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: "tool-1",
				IsError:   param.Opt[bool]{Value: true},
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: "Error: file not found"}},
					{OfImage: &anthropic.ImageBlockParam{
						Source: anthropic.ImageBlockParamSourceUnion{
							OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/screenshot.png"},
						},
					}},
				},
			}},
		},
	}
	result := NormalizeAPIMessages([]anthropic.MessageParam{assistantMsg, userMsg})
	// Find the user message with tool_result
	for _, msg := range result {
		if msg.Role == anthropic.MessageParamRoleUser {
			for _, block := range msg.Content {
				if block.OfToolResult != nil {
					for _, c := range block.OfToolResult.Content {
						if c.OfImage != nil {
							t.Error("error tool_result should have images stripped")
						}
					}
				}
			}
		}
	}
}

func TestNormalizeAPIMessagesFullPipelineOrder(t *testing.T) {
	// Test that all pipeline stages work together correctly
	msgs := []anthropic.MessageParam{
		// Virtual message (should be stripped first)
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "[virtual]"}},
			},
		},
		// User message with misplaced attachment and valid image
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "See image:"}},
				{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      "dGVzdA==",
							MediaType: anthropic.Base64ImageSourceMediaTypeImageJPEG,
						},
					},
				}},
			},
		},
		// Assistant response
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "I see it"}},
			},
		},
	}
	result := NormalizeAPIMessages(msgs)

	// After virtual stripping: [user, assistant]
	// After role alternation: same (already alternating after virtual removed)
	// After attachment reorder: image comes before text in first user message
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	// First message: user with image then text
	if result[0].Role != anthropic.MessageParamRoleUser {
		t.Error("expected first message to be user")
	}
	if result[0].Content[0].OfImage == nil {
		t.Error("expected image block first (reordered)")
	}
	if result[0].Content[1].OfText == nil {
		t.Error("expected text block second")
	}

	// Second message: assistant
	if result[1].Role != anthropic.MessageParamRoleAssistant {
		t.Error("expected second message to be assistant")
	}
}

// ─── Upstream Quality: Idempotency ──────────────────────────────────────────

func TestNormalizeWhitespaceIdempotent(t *testing.T) {
	// normalizeWhitespace(normalizeWhitespace(x)) == normalizeWhitespace(x)
	// Invariant: normalization should be idempotent
	inputs := []string{
		"hello   \nworld  ",
		"a\n\n\n\nb",
		"line1\n\n\n\nline2\n\n\n\nline3",
		"a  \n\n\nb  \n\n",
		"\n\n\n\n",
		"   \t   \n\n\t  \n",
		"no trailing spaces",
		"",
	}
	for _, in := range inputs {
		first := normalizeWhitespace(in)
		second := normalizeWhitespace(first)
		if second != first {
			t.Errorf("normalizeWhitespace not idempotent for %q: first=%q, second=%q", in, first, second)
		}
	}
}

func TestNormalizeJSONBytesIdempotent(t *testing.T) {
	// NormalizeJSONBytes should be idempotent
	inputs := []string{
		`{"z":1,"a":2,"m":3}`,
		`{"outer":{"z":1,"a":2}}`,
		`{"a":1,"b":2}`,
		`{}`,
		`[1,2,{"z":3,"a":4}]`,
	}
	for _, in := range inputs {
		first := NormalizeJSONBytes([]byte(in))
		second := NormalizeJSONBytes(first)
		if string(second) != string(first) {
			t.Errorf("NormalizeJSONBytes not idempotent for %q: first=%q, second=%q", in, string(first), string(second))
		}
	}
}

func TestNormalizeAPIMessagesIdempotent(t *testing.T) {
	// NormalizeAPIMessages should be idempotent: normalize(normalize(msgs)) == normalize(msgs)
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hello"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    "tool-1",
					Name:  "read_file",
					Input: map[string]any{"path": "/tmp/test.go", "offset": 10},
				}},
			},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool-1",
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: "result\n\n\n\nmore"}},
					},
				}},
			},
		},
	}

	first := NormalizeAPIMessages(msgs)
	second := NormalizeAPIMessages(first)

	if len(first) != len(second) {
		t.Fatalf("idempotency violation: message count changed from %d to %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Role != second[i].Role {
			t.Errorf("idempotency violation: message %d role changed from %q to %q", i, first[i].Role, second[i].Role)
		}
	}
}

// ─── Upstream Quality: NormalizeWhitespace Edge Cases ───────────────────────

func TestNormalizeWhitespaceUnicodePreservation(t *testing.T) {
	// normalizeWhitespace is about collapsing blank lines and trimming trailing
	// spaces/tabs. It does NOT strip unicode or BOM characters. Unicode text
	// should be preserved as-is (matching upstream's sanitization principle that
	// normalizeWhitespace doesn't strip unicode).
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"CJK characters preserved", "你好\n\n\n世界", "你好\n\n世界"},
		{"emoji preserved", "Hello 🌍\n\n\nWorld  ", "Hello 🌍\n\nWorld"},
		{"accented chars preserved", "café\n\n\trésumé  ", "café\n\n\trésumé"}, // leading tab on line preserved, trailing space trimmed
		{"unicode whitespace not trimmed (only space/tab)", "hello\xc2\xa0\n", "hello\xc2\xa0"}, // non-breaking space not trimmed
		{"BOM preserved (not stripped by normalizeWhitespace)", "\uFEFFhello\n\nworld", "\uFEFFhello\n\nworld"},
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

func TestNormalizeWhitespaceControlCharacters(t *testing.T) {
	// normalizeWhitespace only trims space and tab from line endings (\t).
	// Other control characters like \r (carriage return) are NOT trimmed
	// because TrimRight(line, " \t") only targets space and tab.
	// This matches upstream's behavior where normalizeWhitespace is purely
	// a whitespace collapser, not a general sanitization function.

	// \r is not trimmed (only space and tab are)
	input := "line1\r\n\n\nline2"
	got := normalizeWhitespace(input)
	// \r survives because TrimRight(line, " \t") doesn't strip it
	want := "line1\r\n\nline2"
	if got != want {
		t.Errorf("normalizeWhitespace with \\r: got %q, want %q", got, want)
	}

	// Tab at end of line IS trimmed
	input2 := "line1\t\n\n\nline2\t"
	got2 := normalizeWhitespace(input2)
	want2 := "line1\n\nline2"
	if got2 != want2 {
		t.Errorf("normalizeWhitespace with trailing tabs: got %q, want %q", got2, want2)
	}
}

func TestNormalizeWhitespaceLargeBlankBlocks(t *testing.T) {
	// Verify that 10, 50, 100 blank lines are all collapsed to exactly 1.
	for _, n := range []int{10, 50, 100} {
		// Create n blank lines between two lines of text
		lines := "a" + strings.Repeat("\n", n+1) + "b" // n+1 = n blank lines
		got := normalizeWhitespace(lines)
		want := "a\n\nb"
		if got != want {
			t.Errorf("normalizeWhitespace with %d blank lines: got %q, want %q", n, got, want)
		}
	}
}

func TestNormalizeWhitespaceSingleLineNoTrailing(t *testing.T) {
	// A single line with no trailing newline should remain unchanged.
	inputs := []string{
		"hello",
		"a   ",
		"no trailing spaces",
	}
	for _, in := range inputs {
		got := normalizeWhitespace(in)
		want := strings.TrimRight(in, " \t")
		if got != want {
			t.Errorf("normalizeWhitespace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAPIMessagesIdempotentRepeated(t *testing.T) {
	// Verify that running NormalizeAPIMessages many times doesn't keep
	// changing the output (strong idempotency).
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hello"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "Hi"}},
			},
		},
	}

	first := NormalizeAPIMessages(msgs)
	for i := 0; i < 10; i++ {
		next := NormalizeAPIMessages(first)
		if len(next) != len(first) {
			t.Fatalf("NormalizeAPIMessages changed length on iteration %d", i)
		}
	}
}

// ============================================================================
// Phase 4-6 New Feature Tests
// ============================================================================

// ─── Phase 4: system-reminder smooshing ───────────────────────────────────────

func TestSmooshSystemReminders(t *testing.T) {
	t.Run("folds system-reminder into preceding tool_result", func(t *testing.T) {
		// After ReorderContentForAPI, tool_results are hoisted to the front.
		// So the order becomes: tool_result, system-reminder text
		// smooshSystemReminders should fold the system-reminder into tool_result
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfToolResult: &anthropic.ToolResultBlockParam{
						ToolUseID: "tool1",
						Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "result"}}},
					}},
					{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>file changed</system-reminder>"}},
				},
			},
		}

		result := smooshSystemReminders(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		content := result[0].Content
		if len(content) != 1 {
			t.Fatalf("expected 1 content block after smoosh, got %d", len(content))
		}
		if content[0].OfToolResult == nil {
			t.Fatal("expected tool_result block")
		}
		blocks := content[0].OfToolResult.Content
		if len(blocks) != 2 {
			t.Fatalf("expected 2 content blocks in tool_result, got %d", len(blocks))
		}
		if blocks[1].OfText == nil || blocks[1].OfText.Text != "<system-reminder>file changed</system-reminder>" {
			t.Errorf("expected system-reminder text in tool_result, got %v", blocks[1])
		}
	})

	t.Run("no adjacent tool_result keeps standalone", func(t *testing.T) {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>no tool_result</system-reminder>"}},
				},
			},
		}

		result := smooshSystemReminders(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		// Single text block — no smoosh possible, but it should still be there
		if result[0].Content[0].OfText == nil {
			t.Error("expected text block to remain")
		}
	})
}

// ─── Phase 5: beta header memoization ─────────────────────────────────────────

func TestBetaHeaderMemoizationNormalize(t *testing.T) {
	ClearBetaHeaderCache()

	// First call computes
	betas1 := BuildBetaHeaders("test-memo-model")
	// Second call returns cached copy
	betas2 := BuildBetaHeaders("test-memo-model")

	if len(betas1) != len(betas2) {
		t.Errorf("cached result length mismatch: first=%d, second=%d", len(betas1), len(betas2))
	}
	for i := range betas1 {
		if betas1[i] != betas2[i] {
			t.Errorf("cached result mismatch at %d: first=%q, second=%q", i, betas1[i], betas2[i])
		}
	}

	// Clear cache should force recomputation
	ClearBetaHeaderCache()
	betas3 := BuildBetaHeaders("different-memo-model")
	if len(betas3) != len(betas1) {
		t.Errorf("after cache clear, different model should still produce same length: got %d, expected %d", len(betas3), len(betas1))
	}
}

// ─── Phase 6: assistant message content ordering ──────────────────────────────

func TestReorderContentForAPIAssistantMessages(t *testing.T) {
	t.Run("reorders assistant message with tool_use before text", func(t *testing.T) {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					{OfToolUse: &anthropic.ToolUseBlockParam{Name: "read_file", Input: json.RawMessage("{}")}},
					{OfText: &anthropic.TextBlockParam{Text: "Let me read the file"}},
				},
			},
		}

		result := ReorderContentForAPI(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		content := result[0].Content
		// Should be reordered: text first, then tool_use
		if content[0].OfText == nil {
			t.Error("expected text block first after reorder")
		}
		if content[1].OfToolUse == nil {
			t.Error("expected tool_use block second after reorder")
		}
	})

	t.Run("already ordered assistant message unchanged", func(t *testing.T) {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: "thinking"}},
					{OfToolUse: &anthropic.ToolUseBlockParam{Name: "read_file", Input: json.RawMessage("{}")}},
				},
			},
		}

		result := ReorderContentForAPI(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		// Should remain: text first, then tool_use
		if result[0].Content[0].OfText == nil {
			t.Error("expected text block first")
		}
		if result[0].Content[1].OfToolUse == nil {
			t.Error("expected tool_use block second")
		}
	})
}

// ─── Phase 6: empty text block stripping ──────────────────────────────────────

func TestStripEmptyTextBlocks(t *testing.T) {
	t.Run("strips empty text blocks", func(t *testing.T) {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: ""}},
					{OfText: &anthropic.TextBlockParam{Text: "   "}},
					{OfText: &anthropic.TextBlockParam{Text: "real content"}},
				},
			},
		}

		result := stripEmptyTextBlocks(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if len(result[0].Content) != 1 {
			t.Errorf("expected 1 content block after stripping empty, got %d", len(result[0].Content))
		}
		if result[0].Content[0].OfText.Text != "real content" {
			t.Errorf("expected 'real content', got %q", result[0].Content[0].OfText.Text)
		}
	})

	t.Run("all empty text blocks keeps placeholder", func(t *testing.T) {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleAssistant,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: ""}},
					{OfText: &anthropic.TextBlockParam{Text: "   "}},
				},
			},
		}

		result := stripEmptyTextBlocks(msgs)
		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if len(result[0].Content) != 1 {
			t.Errorf("expected 1 placeholder block, got %d", len(result[0].Content))
		}
		if result[0].Content[0].OfText.Text != "[empty]" {
			t.Errorf("expected '[empty]' placeholder, got %q", result[0].Content[0].OfText.Text)
		}
	})

	t.Run("no empty blocks unchanged", func(t *testing.T) {
		msgs := []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: "hello"}},
					{OfText: &anthropic.TextBlockParam{Text: "world"}},
				},
			},
		}

		result := stripEmptyTextBlocks(msgs)
		if len(result[0].Content) != 2 {
			t.Errorf("expected 2 content blocks unchanged, got %d", len(result[0].Content))
		}
	})
}

// ─── Phase 4-6: normalize idempotency ─────────────────────────────────────────

func TestNormalizeIdempotentPhase456(t *testing.T) {
	// Test that NormalizeAPIMessages is idempotent after Phase 4-6 changes
	msgs := []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "tool1",
					Content:   []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: "result"}}},
				}},
				{OfText: &anthropic.TextBlockParam{Text: "<system-reminder>file changed</system-reminder>"}},
				{OfText: &anthropic.TextBlockParam{Text: "   "}},
				{OfText: &anthropic.TextBlockParam{Text: "user text"}},
			},
		},
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				{OfToolUse: &anthropic.ToolUseBlockParam{Name: "read_file", Input: json.RawMessage("{}")}},
				{OfText: &anthropic.TextBlockParam{Text: "reading file"}},
				{OfText: &anthropic.TextBlockParam{Text: ""}},
			},
		},
	}

	first := NormalizeAPIMessages(msgs)
	for i := 0; i < 10; i++ {
		next := NormalizeAPIMessages(first)
		if len(next) != len(first) {
			t.Fatalf("NormalizeAPIMessages changed length on iteration %d", i)
		}
		// Check content block counts are stable
		for j := range next {
			if len(next[j].Content) != len(first[j].Content) {
				t.Fatalf("NormalizeAPIMessages changed content block count on msg %d iteration %d", j, i)
			}
		}
	}
}