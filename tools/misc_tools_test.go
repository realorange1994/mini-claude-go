package tools

import (
	"strings"
	"testing"
	"time"
)

// ─── TaskOutputTool ─────────────────────────────────────────────────────────

func TestTaskOutputToolName(t *testing.T) {
	tool := &TaskOutputTool{}
	if tool.Name() != "task_output" {
		t.Errorf("expected 'task_output', got %q", tool.Name())
	}
}

func TestTaskOutputToolSchema(t *testing.T) {
	tool := &TaskOutputTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "task_id" {
		t.Errorf("expected required=[task_id], got %v", required)
	}
}

func TestTaskOutputToolPermissions(t *testing.T) {
	tool := &TaskOutputTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTaskOutputToolExecuteNilCallback(t *testing.T) {
	tool := &TaskOutputTool{}
	result := tool.Execute(map[string]any{"task_id": "1"})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestTaskOutputToolExecuteMissingTaskID(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(id string, block bool, timeout time.Duration) (string, string) {
			return "", ""
		},
	}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing task_id should return error")
	}
}

func TestTaskOutputToolExecuteWithResult(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(id string, block bool, timeout time.Duration) (string, string) {
			return "task output", ""
		},
	}
	result := tool.Execute(map[string]any{"task_id": "abc"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "task output" {
		t.Errorf("expected 'task output', got %q", result.Output)
	}
}

func TestTaskOutputToolExecuteWithErr(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(id string, block bool, timeout time.Duration) (string, string) {
			return "", "task failed"
		},
	}
	result := tool.Execute(map[string]any{"task_id": "abc"})
	if !result.IsError {
		t.Error("error text should result in error")
	}
}

func TestTaskOutputToolExecuteNoOutputYet(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(id string, block bool, timeout time.Duration) (string, string) {
			return "", ""
		},
	}
	result := tool.Execute(map[string]any{"task_id": "abc"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "no output available yet") {
		t.Errorf("should mention no output yet, got %q", result.Output)
	}
}

func TestTaskOutputToolExecuteTimeoutClamp(t *testing.T) {
	var gotTimeout time.Duration
	tool := &TaskOutputTool{
		GetOutputFunc: func(id string, block bool, timeout time.Duration) (string, string) {
			gotTimeout = timeout
			return "done", ""
		},
	}
	tool.Execute(map[string]any{"task_id": "abc", "timeout": float64(999999)})
	if gotTimeout != 600000*time.Millisecond {
		t.Errorf("timeout should be clamped to 600s, got %v", gotTimeout)
	}
}

// ─── EnterPlanModeTool ──────────────────────────────────────────────────────

func TestEnterPlanModeToolName(t *testing.T) {
	tool := &EnterPlanModeTool{}
	if tool.Name() != "EnterPlanMode" {
		t.Errorf("expected 'EnterPlanMode', got %q", tool.Name())
	}
}

func TestEnterPlanModeToolSchema(t *testing.T) {
	tool := &EnterPlanModeTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["reason"]; !ok {
		t.Error("schema should have reason property")
	}
}

func TestEnterPlanModeToolPermissions(t *testing.T) {
	tool := &EnterPlanModeTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestEnterPlanModeToolExecuteNormal(t *testing.T) {
	var setMode string
	tool := &EnterPlanModeTool{
		GetMode: func() string { return "normal" },
		SetMode: func(m string) { setMode = m },
	}
	result := tool.Execute(map[string]any{"reason": "refactoring"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if setMode != "plan" {
		t.Errorf("expected mode 'plan', got %q", setMode)
	}
	if !strings.Contains(result.Output, "refactoring") {
		t.Error("result should contain the reason")
	}
}

func TestEnterPlanModeToolExecuteAlreadyInPlan(t *testing.T) {
	tool := &EnterPlanModeTool{
		GetMode: func() string { return "plan" },
		SetMode: func(m string) {},
	}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Already in plan mode") {
		t.Errorf("should indicate already in plan mode, got %q", result.Output)
	}
}

// ─── ExitPlanModeTool ───────────────────────────────────────────────────────

func TestExitPlanModeToolName(t *testing.T) {
	tool := &ExitPlanModeTool{}
	if tool.Name() != "ExitPlanMode" {
		t.Errorf("expected 'ExitPlanMode', got %q", tool.Name())
	}
}

func TestExitPlanModeToolPermissions(t *testing.T) {
	tool := &ExitPlanModeTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestExitPlanModeToolExecuteNotInPlan(t *testing.T) {
	tool := &ExitPlanModeTool{
		GetMode: func() string { return "auto" },
	}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Not in plan mode") {
		t.Errorf("should indicate not in plan mode, got %q", result.Output)
	}
}

func TestExitPlanModeToolExecuteApproved(t *testing.T) {
	var setMode string
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return "plan" },
		SetMode:        func(m string) { setMode = m },
		GetPrePlanMode: func() string { return "auto" },
	}
	result := tool.Execute(map[string]any{"approved": true, "summary": "Implement feature"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if setMode != "auto" {
		t.Errorf("expected mode 'auto', got %q", setMode)
	}
	if !strings.Contains(result.Output, "Implement feature") {
		t.Error("result should contain the summary")
	}
}

func TestExitPlanModeToolExecuteNotApproved(t *testing.T) {
	tool := &ExitPlanModeTool{
		GetMode: func() string { return "plan" },
		SetMode: func(m string) {},
	}
	result := tool.Execute(map[string]any{"approved": false})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Plan not yet approved") {
		t.Errorf("should indicate not approved, got %q", result.Output)
	}
}

func TestExitPlanModeToolExecuteDefaultPrePlan(t *testing.T) {
	var setMode string
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return "plan" },
		SetMode:        func(m string) { setMode = m },
		GetPrePlanMode: func() string { return "" },
	}
	result := tool.Execute(map[string]any{"approved": true})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if setMode != "auto" {
		t.Errorf("empty pre-plan should default to 'auto', got %q", setMode)
	}
}

// ─── parseNumber (ask_user_question.go) ────────────────────────────────────

func TestParseNumberValid(t *testing.T) {
	n, err := parseNumber("42")
	if err != nil || n != 42 {
		t.Errorf("expected 42, got %d, err %v", n, err)
	}
}

func TestParseNumberZero(t *testing.T) {
	n, err := parseNumber("0")
	if err != nil || n != 0 {
		t.Errorf("expected 0, got %d, err %v", n, err)
	}
}

func TestParseNumberEmpty(t *testing.T) {
	// Empty string returns (0, nil) — the loop simply doesn't execute
	n, err := parseNumber("")
	if err != nil {
		t.Error("empty string should not error, it returns 0")
	}
	if n != 0 {
		t.Errorf("expected 0 for empty string, got %d", n)
	}
}

func TestParseNumberNonDigit(t *testing.T) {
	_, err := parseNumber("abc")
	if err == nil {
		t.Error("non-digit should return error")
	}
}

func TestParseNumberMixed(t *testing.T) {
	_, err := parseNumber("1a")
	if err == nil {
		t.Error("mixed digits/letters should return error")
	}
}

func TestParseNumberMultiDigit(t *testing.T) {
	n, err := parseNumber("123")
	if err != nil || n != 123 {
		t.Errorf("expected 123, got %d, err %v", n, err)
	}
}
