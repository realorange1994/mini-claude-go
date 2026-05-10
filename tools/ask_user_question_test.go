package tools

import (
	"strings"
	"testing"
)

// ─── AskUserQuestionTool interface ───────────────────────────────────────────

func TestAskUserQuestionToolName(t *testing.T) {
	tool := &AskUserQuestionTool{}
	if tool.Name() != "AskUserQuestion" {
		t.Errorf("expected 'AskUserQuestion', got %q", tool.Name())
	}
}

func TestAskUserQuestionToolSchema(t *testing.T) {
	tool := &AskUserQuestionTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "questions" {
		t.Errorf("expected required=[questions], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["questions"]; !ok {
		t.Error("schema should have questions property")
	}
}

func TestAskUserQuestionToolPermissions(t *testing.T) {
	tool := &AskUserQuestionTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestAskUserQuestionToolExecuteNoQuestions(t *testing.T) {
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing questions should return error")
	}
}

func TestAskUserQuestionToolExecuteNotArray(t *testing.T) {
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{"questions": "not an array"})
	if !result.IsError {
		t.Error("non-array questions should return error")
	}
}

func TestAskUserQuestionToolExecuteEmptyArray(t *testing.T) {
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{"questions": []any{}})
	if !result.IsError {
		t.Error("empty questions array should return error")
	}
}

func TestAskUserQuestionToolExecuteTooManyQuestions(t *testing.T) {
	questions := make([]any, 5)
	for i := range questions {
		questions[i] = map[string]any{
			"question": "Q?",
			"header":   "H",
			"options": []any{
				map[string]any{"label": "A", "description": "desc a"},
				map[string]any{"label": "B", "description": "desc b"},
			},
		}
	}
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{"questions": questions})
	if !result.IsError {
		t.Error("more than 4 questions should return error")
	}
	if !strings.Contains(result.Output, "at most 4") {
		t.Errorf("should mention at most 4, got %q", result.Output)
	}
}

func TestAskUserQuestionToolExecuteTooFewOptions(t *testing.T) {
	questions := []any{
		map[string]any{
			"question": "Q?",
			"header":   "H",
			"options": []any{
				map[string]any{"label": "A", "description": "only one"},
			},
		},
	}
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{"questions": questions})
	if !result.IsError {
		t.Error("fewer than 2 options should return error")
	}
}

func TestAskUserQuestionToolExecuteTooManyOptions(t *testing.T) {
	options := make([]any, 5)
	for i := range options {
		options[i] = map[string]any{"label": "A", "description": "opt"}
	}
	questions := []any{
		map[string]any{
			"question": "Q?",
			"header":   "H",
			"options":  options,
		},
	}
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{"questions": questions})
	if !result.IsError {
		t.Error("more than 4 options should return error")
	}
	if !strings.Contains(result.Output, "at most 4") {
		t.Errorf("should mention at most 4, got %q", result.Output)
	}
}

func TestAskUserQuestionToolExecuteNoOptionsKey(t *testing.T) {
	questions := []any{
		map[string]any{
			"question": "Q?",
			"header":   "H",
		},
	}
	tool := &AskUserQuestionTool{}
	result := tool.Execute(map[string]any{"questions": questions})
	if !result.IsError {
		t.Error("missing options should return error")
	}
}

// Note: Execute with valid questions requires stdin interaction,
// which is not suitable for automated testing. The validation
// logic above covers the error paths; the happy path is
// effectively tested by the parseNumber tests in misc_tools_test.go.
