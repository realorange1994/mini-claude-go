package tools

import (
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeInfo(t *testing.T) {
	tool := &RuntimeInfoTool{}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	checks := []string{
		"Go Version:",
		"GOOS:",
		"GOARCH:",
		"NumCPU:",
		"NumGoroutine:",
		"Working Directory:",
		"Memory Alloc:",
	}

	for _, want := range checks {
		if !strings.Contains(result.Output, want) {
			t.Errorf("expected %q in output: %s", want, result.Output)
		}
	}

	if !strings.Contains(result.Output, runtime.GOOS) {
		t.Errorf("expected actual GOOS %q in output: %s", runtime.GOOS, result.Output)
	}
}

func TestRuntimeInfoSchema(t *testing.T) {
	tool := &RuntimeInfoTool{}
	schema := tool.InputSchema()
	if schema["type"] != "object" {
		t.Errorf("expected type object, got: %v", schema["type"])
	}
	props := schema["properties"].(map[string]any)
	if len(props) != 0 {
		t.Errorf("expected no properties, got: %v", props)
	}
}

func TestRuntimeInfoPermissions(t *testing.T) {
	tool := &RuntimeInfoTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result != "" {
		t.Errorf("expected no permission denial, got: %s", result)
	}
}
