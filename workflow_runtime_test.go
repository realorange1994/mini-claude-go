package main

import (
	"testing"
)

func TestWorkflowRuntime_New(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)
	if r == nil {
		t.Error("expected non-nil runtime")
	}
}

func TestWorkflowRuntime_Define(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	steps := []WorkflowStep{
		{ID: "step1", Name: "Step 1", Type: "agent"},
	}

	workflow := r.Define("test-workflow", "A test workflow", steps)
	if workflow == nil {
		t.Fatal("expected non-nil workflow")
	}
	if workflow.Name != "test-workflow" {
		t.Errorf("expected 'test-workflow', got %q", workflow.Name)
	}
	if len(workflow.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(workflow.Steps))
	}
}

func TestWorkflowRuntime_Run(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	steps := []WorkflowStep{
		{ID: "step1", Name: "Step 1", Type: "agent"},
	}

	workflow := r.Define("test-workflow", "A test workflow", steps)
	result, err := r.Run(workflow.ID)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if result.Status != WorkflowCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestWorkflowRuntime_Run_NotFound(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	_, err := r.Run("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workflow")
	}
}

func TestWorkflowRuntime_Cancel(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	steps := []WorkflowStep{
		{ID: "step1", Name: "Step 1", Type: "agent"},
	}

	workflow := r.Define("test-workflow", "A test workflow", steps)

	err := r.Cancel(workflow.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	if workflow.Status != WorkflowCancelled {
		t.Errorf("expected cancelled, got %s", workflow.Status)
	}
}

func TestWorkflowRuntime_GetWorkflow(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	steps := []WorkflowStep{
		{ID: "step1", Name: "Step 1", Type: "agent"},
	}

	workflow := r.Define("test-workflow", "A test workflow", steps)

	result := r.GetWorkflow(workflow.ID)
	if result == nil {
		t.Fatal("expected non-nil workflow")
	}
	if result.ID != workflow.ID {
		t.Errorf("expected %s, got %s", workflow.ID, result.ID)
	}
}

func TestWorkflowRuntime_ListWorkflows(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	r.Define("workflow1", "First workflow", nil)

	workflows := r.ListWorkflows()
	if len(workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(workflows))
	}
}

func TestWorkflowRuntime_SaveLoadWorkflow(t *testing.T) {
	dir := t.TempDir()
	r := NewWorkflowRuntime(dir)

	steps := []WorkflowStep{
		{ID: "step1", Name: "Step 1", Type: "agent"},
	}

	workflow := r.Define("test-workflow", "A test workflow", steps)

	// Save
	err := r.SaveWorkflow(workflow.ID)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Create new runtime and load
	r2 := NewWorkflowRuntime(dir)
	err = r2.LoadWorkflow(workflow.ID)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	loaded := r2.GetWorkflow(workflow.ID)
	if loaded == nil {
		t.Fatal("expected loaded workflow")
	}
	if loaded.Name != "test-workflow" {
		t.Errorf("expected 'test-workflow', got %q", loaded.Name)
	}
}

func TestFormatWorkflowResult(t *testing.T) {
	result := &WorkflowResult{
		WorkflowID: "test",
		Status:     WorkflowCompleted,
		Steps: []WorkflowStep{
			{Name: "Step 1", Type: "agent", Status: WorkflowCompleted},
		},
	}

	output := FormatWorkflowResult(result)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatWorkflowResult_Nil(t *testing.T) {
	output := FormatWorkflowResult(nil)
	if output != "No workflow result." {
		t.Errorf("expected 'No workflow result.', got %q", output)
	}
}

func TestFormatWorkflowList(t *testing.T) {
	workflows := []*Workflow{
		{ID: "test", Name: "Test Workflow", Description: "A test"},
	}

	output := FormatWorkflowList(workflows)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatWorkflowList_Empty(t *testing.T) {
	output := FormatWorkflowList(nil)
	if output != "No workflows found." {
		t.Errorf("expected 'No workflows found.', got %q", output)
	}
}
