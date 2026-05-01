package tools

import (
	"strings"
	"testing"
)

func TestBriefToolName(t *testing.T) {
	tool := &BriefTool{}
	if tool.Name() != "brief" {
		t.Errorf("expected name 'brief', got: %s", tool.Name())
	}
}

func TestBriefToolDescription(t *testing.T) {
	tool := &BriefTool{}
	desc := tool.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
	if !strings.Contains(desc, "communication") {
		t.Error("description should mention communication")
	}
}

func TestBriefToolInputSchema(t *testing.T) {
	tool := &BriefTool{}
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Errorf("expected type object, got: %v", schema["type"])
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("expected required field")
	}
	found := false
	for _, r := range required {
		if r == "task" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'task' in required params, got: %v", required)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["task"]; !ok {
		t.Error("expected 'task' property in schema")
	}
}

func TestBriefToolCheckPermissions(t *testing.T) {
	tool := &BriefTool{}
	result := tool.CheckPermissions(map[string]any{"task": "test"})
	if result != "" {
		t.Errorf("expected no permission denial, got: %s", result)
	}
}

func TestBriefToolExecuteSuccess(t *testing.T) {
	tool := &BriefTool{}
	result := tool.Execute(map[string]any{"task": "implement a new feature"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// Verify key principles are present
	expectedPhrases := []string{
		"concise and direct",
		"Skip filler words",
		"Don't restate",
		"decisions needing user input",
		"implement a new feature",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(result.Output, phrase) {
			t.Errorf("expected %q in output", phrase)
		}
	}
}

func TestBriefToolExecuteMissingTask(t *testing.T) {
	tool := &BriefTool{}
	result := tool.Execute(map[string]any{})

	if !result.IsError {
		t.Fatal("expected error when task is missing")
	}
	if !strings.Contains(result.Output, "task parameter is required") {
		t.Errorf("expected 'task parameter is required' in error, got: %s", result.Output)
	}
}

func TestBriefToolExecuteEmptyTask(t *testing.T) {
	tool := &BriefTool{}
	result := tool.Execute(map[string]any{"task": ""})

	if !result.IsError {
		t.Fatal("expected error when task is empty")
	}
}
