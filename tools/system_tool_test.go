package tools

import (
	"strings"
	"testing"
)

func TestSystemInfo(t *testing.T) {
	tool := &SystemTool{}
	result := tool.Execute(map[string]any{"operation": "info"})
	if result.IsError {
		t.Fatalf("system info failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "OS:") {
		t.Errorf("expected 'OS:' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Memory:") {
		t.Errorf("expected 'Memory:' in output, got: %s", result.Output)
	}
}

func TestSystemUname(t *testing.T) {
	tool := &SystemTool{}
	result := tool.Execute(map[string]any{"operation": "uname"})
	if result.IsError {
		t.Fatalf("system uname failed: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("expected non-empty output from uname")
	}
}

func TestSystemWho(t *testing.T) {
	tool := &SystemTool{}
	result := tool.Execute(map[string]any{"operation": "who"})
	if result.IsError {
		t.Fatalf("system who failed: %s", result.Output)
	}
	// Should return something (at least current user or "No users logged in")
	if result.Output == "" {
		t.Error("expected non-empty output from who")
	}
}

func TestSystemUnknownOperation(t *testing.T) {
	tool := &SystemTool{}
	result := tool.Execute(map[string]any{"operation": "invalid"})
	if !result.IsError {
		t.Error("expected error for unknown operation")
	}
}
