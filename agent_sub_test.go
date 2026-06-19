package main

import (
	"strings"
	"testing"
	"time"
)

func TestCompletionGate_CheckCompletion_Success(t *testing.T) {
	g := NewCompletionGate()

	tests := []struct {
		output   string
		complete bool
	}{
		{"**Status**: success\nTask completed.", true},
		{"**Status**: partial\nSome work done.", true},
		{"Task completed successfully.", true},
		{"Done.", true},
		{"**Status**: blocked\nCannot proceed.", false},
		{"**Status**: failed\nError occurred.", false},
		{"I still need to fix the bug.", false},
		{"Some output without markers", true}, // default: assume complete
	}

	for _, tt := range tests {
		complete, _ := g.CheckCompletion(tt.output, "test task")
		if complete != tt.complete {
			t.Errorf("CheckCompletion(%q) = %v, want %v", tt.output, complete, tt.complete)
		}
	}
}

func TestCompletionGate_BuildNudgeMessage(t *testing.T) {
	g := NewCompletionGate()
	msg := g.BuildNudgeMessage("task is incomplete", "fix the bug")

	if msg == "" {
		t.Error("expected non-empty nudge message")
	}
	if !strings.Contains(msg, "fix the bug") {
		t.Error("expected message to contain task description")
	}
}

func TestValidateOutput_ValidJSON(t *testing.T) {
	output := `Here is the result: {"status": "success", "count": 42}`
	schema := map[string]any{
		"required": []string{"status"},
	}

	valid, result, err := ValidateOutput(output, schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Error("expected valid output")
	}
	if result["status"] != "success" {
		t.Errorf("expected status 'success', got '%v'", result["status"])
	}
}

func TestValidateOutput_MissingRequiredField(t *testing.T) {
	output := `{"count": 42}`
	schema := map[string]any{
		"required": []string{"status"},
	}

	valid, _, err := ValidateOutput(output, schema)
	if valid {
		t.Error("expected invalid output")
	}
	if err == nil {
		t.Error("expected error for missing field")
	}
}

func TestValidateOutput_NoJSON(t *testing.T) {
	output := "No JSON here"
	schema := map[string]any{}

	valid, _, err := ValidateOutput(output, schema)
	if valid {
		t.Error("expected invalid output")
	}
	if err == nil {
		t.Error("expected error for missing JSON")
	}
}

func TestAgentHealthCheck(t *testing.T) {
	h := NewAgentHealthCheck(100 * time.Millisecond)

	// Record activity
	h.RecordActivity("agent-1")

	// Should not be stuck immediately
	if h.IsStuck("agent-1") {
		t.Error("agent should not be stuck immediately")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should be stuck now
	if !h.IsStuck("agent-1") {
		t.Error("agent should be stuck after timeout")
	}

	// Unknown agent should not be stuck
	if h.IsStuck("agent-unknown") {
		t.Error("unknown agent should not be stuck")
	}
}

func TestAgentHealthCheck_RemoveAgent(t *testing.T) {
	h := NewAgentHealthCheck(100 * time.Millisecond)

	h.RecordActivity("agent-1")
	h.RemoveAgent("agent-1")

	// Should not be tracked anymore
	if h.IsStuck("agent-1") {
		t.Error("removed agent should not be tracked")
	}
}
