package tools

import (
	"context"
	"strings"
	"testing"
)

// ─── EnterPlanModeTool ──────────────────────────────────────────────────────

func TestEnterPlanModeName(t *testing.T) {
	tool := &EnterPlanModeTool{}
	if tool.Name() != "EnterPlanMode" {
		t.Errorf("Name() = %q, want EnterPlanMode", tool.Name())
	}
}

func TestEnterPlanModeDescription(t *testing.T) {
	tool := &EnterPlanModeTool{}
	desc := tool.Description()
	if !strings.Contains(desc, "plan mode") {
		t.Error("description should mention plan mode")
	}
	if !strings.Contains(desc, "ExitPlanMode") {
		t.Error("description should reference ExitPlanMode")
	}
}

func TestEnterPlanModeInputSchema(t *testing.T) {
	tool := &EnterPlanModeTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}
	if _, ok := props["reason"]; !ok {
		t.Error("schema should have 'reason' property")
	}
}

func TestEnterPlanModeCheckPermissions(t *testing.T) {
	tool := &EnterPlanModeTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("EnterPlanMode should passthrough permissions, got: %v", result)
	}
}

func TestEnterPlanModeExecute(t *testing.T) {
	var mode string
	tool := &EnterPlanModeTool{
		GetMode: func() string { return mode },
		SetMode: func(m string) { mode = m },
	}

	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Entered plan mode") {
		t.Errorf("expected 'Entered plan mode', got: %s", result.Output)
	}
	if mode != "plan" {
		t.Errorf("mode = %q, want 'plan'", mode)
	}
}

func TestEnterPlanModeExecuteWithReason(t *testing.T) {
	var mode string
	tool := &EnterPlanModeTool{
		GetMode: func() string { return mode },
		SetMode: func(m string) { mode = m },
	}

	result := tool.Execute(map[string]any{"reason": "Implement new feature"})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Implement new feature") {
		t.Errorf("expected reason in output, got: %s", result.Output)
	}
}

func TestEnterPlanModeAlreadyInPlanMode(t *testing.T) {
	tool := &EnterPlanModeTool{
		GetMode: func() string { return "plan" },
		SetMode: func(m string) { t.Fatal("SetMode should not be called") },
	}

	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Already in plan mode") {
		t.Errorf("expected 'Already in plan mode', got: %s", result.Output)
	}
}

func TestEnterPlanModeCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := &EnterPlanModeTool{
		GetMode: func() string { return "auto" },
		SetMode: func(m string) {},
	}

	result := tool.ExecuteContext(ctx, map[string]any{})
	if !result.IsError {
		t.Error("expected error for cancelled context")
	}
	if !strings.Contains(result.Output, "timed out") && !strings.Contains(result.Output, "cancelled") {
		t.Errorf("expected timeout/cancelled error, got: %s", result.Output)
	}
}

// ─── ExitPlanModeTool ───────────────────────────────────────────────────────

func TestExitPlanModeName(t *testing.T) {
	tool := &ExitPlanModeTool{}
	if tool.Name() != "ExitPlanMode" {
		t.Errorf("Name() = %q, want ExitPlanMode", tool.Name())
	}
}

func TestExitPlanModeDescription(t *testing.T) {
	tool := &ExitPlanModeTool{}
	desc := tool.Description()
	if !strings.Contains(desc, "plan mode") {
		t.Error("description should mention plan mode")
	}
}

func TestExitPlanModeInputSchema(t *testing.T) {
	tool := &ExitPlanModeTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}
	for _, key := range []string{"approved", "summary"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema should have %q property", key)
		}
	}
}

func TestExitPlanModeCheckPermissions(t *testing.T) {
	tool := &ExitPlanModeTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("ExitPlanMode should passthrough permissions, got: %v", result)
	}
}

func TestExitPlanModeExecute(t *testing.T) {
	mode := "plan"
	prePlan := "normal"
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return mode },
		SetMode:        func(m string) { mode = m },
		GetPrePlanMode: func() string { return prePlan },
	}

	result := tool.Execute(map[string]any{"approved": true})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Exited plan mode") {
		t.Errorf("expected 'Exited plan mode', got: %s", result.Output)
	}
	if mode != "normal" {
		t.Errorf("mode = %q, want 'normal'", mode)
	}
}

func TestExitPlanModeNotInPlanMode(t *testing.T) {
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return "auto" },
		SetMode:        func(m string) { t.Fatal("SetMode should not be called") },
		GetPrePlanMode: func() string { return "auto" },
	}

	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Not in plan mode") {
		t.Errorf("expected 'Not in plan mode', got: %s", result.Output)
	}
}

func TestExitPlanModeApprovedFalse(t *testing.T) {
	mode := "plan"
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return mode },
		SetMode:        func(m string) { t.Fatal("SetMode should not be called") },
		GetPrePlanMode: func() string { return "auto" },
	}

	result := tool.Execute(map[string]any{"approved": false})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "not yet approved") {
		t.Errorf("expected 'not yet approved', got: %s", result.Output)
	}
}

func TestExitPlanModeWithSummary(t *testing.T) {
	mode := "plan"
	prePlan := "auto"
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return mode },
		SetMode:        func(m string) { mode = m },
		GetPrePlanMode: func() string { return prePlan },
	}

	result := tool.Execute(map[string]any{"approved": true, "summary": "Refactor auth module"})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Refactor auth module") {
		t.Errorf("expected summary in output, got: %s", result.Output)
	}
}

func TestExitPlanModeFallbackPrePlan(t *testing.T) {
	mode := "plan"
	prePlan := "" // empty — should fallback to "auto"
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return mode },
		SetMode:        func(m string) { mode = m },
		GetPrePlanMode: func() string { return prePlan },
	}

	result := tool.Execute(map[string]any{"approved": true})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if mode != "auto" {
		t.Errorf("mode = %q, want 'auto' (fallback)", mode)
	}
}

func TestExitPlanModePrePlanIsPlan(t *testing.T) {
	// Edge case: prePlan is "plan" — should also fallback to "auto"
	mode := "plan"
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return mode },
		SetMode:        func(m string) { mode = m },
		GetPrePlanMode: func() string { return "plan" },
	}

	result := tool.Execute(map[string]any{"approved": true})
	if result.IsError {
		t.Fatalf("Execute error: %s", result.Output)
	}
	if mode != "auto" {
		t.Errorf("mode = %q, want 'auto' (prePlan was 'plan')", mode)
	}
}

func TestExitPlanModeCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := &ExitPlanModeTool{
		GetMode:        func() string { return "plan" },
		SetMode:        func(m string) {},
		GetPrePlanMode: func() string { return "auto" },
	}

	result := tool.ExecuteContext(ctx, map[string]any{})
	if !result.IsError {
		t.Error("expected error for cancelled context")
	}
	if !strings.Contains(result.Output, "timed out") && !strings.Contains(result.Output, "cancelled") {
		t.Errorf("expected timeout/cancelled error, got: %s", result.Output)
	}
}
