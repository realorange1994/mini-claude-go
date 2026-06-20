package main

import (
	"strings"
	"testing"
)

func TestCheckpointValidator_ValidContent(t *testing.T) {
	v := NewCheckpointValidator()

	content := `# Session Checkpoint

Topic: Implement authentication module

### Execution context
Working on auth module

### Live resources
- E:\Git\project\

### Session metadata
- Session started: 2026-06-20`

	errors := v.Validate(content)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %d", len(errors))
	}
}

func TestCheckpointValidator_TopicMissing(t *testing.T) {
	v := NewCheckpointValidator()

	content := `# Session Checkpoint

### Execution context
Working on auth module

### Live resources
- E:\Git\project\

### Session metadata
- Session started: 2026-06-20`

	errors := v.Validate(content)
	found := false
	for _, err := range errors {
		if err.RuleID == "topic-missing" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected topic-missing error")
	}
}

func TestCheckpointValidator_TopicTooLong(t *testing.T) {
	v := NewCheckpointValidator()

	topic := strings.Repeat("x", 100)
	content := "Topic: " + topic + "\n\n### Execution context\n### Live resources\n### Session metadata"

	errors := v.Validate(content)
	found := false
	for _, err := range errors {
		if err.RuleID == "topic-too-long" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected topic-too-long warning")
	}
}

func TestCheckpointValidator_SubsectionMissing(t *testing.T) {
	v := NewCheckpointValidator()

	content := `Topic: Test

### Execution context
Working on auth module`

	errors := v.Validate(content)
	found := false
	for _, err := range errors {
		if err.RuleID == "subsection-missing" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected subsection-missing error")
	}
}

func TestCheckpointValidator_BudgetExceeded(t *testing.T) {
	v := NewCheckpointValidator()

	// Create content that exceeds budget
	largeContent := "Topic: Test\n\n### Execution context\n### Live resources\n### Session metadata\n"
	largeContent += strings.Repeat("x", 50000) // ~12500 tokens

	errors := v.Validate(largeContent)
	found := false
	for _, err := range errors {
		if err.RuleID == "budget-exceeded" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected budget-exceeded error")
	}
}

func TestBuildReflectionMessage(t *testing.T) {
	errors := []*ValidationError{
		{RuleID: "topic-missing", Severity: "error", Message: "Missing topic"},
		{RuleID: "subsection-missing", Severity: "error", Message: "Missing section"},
	}

	msg := BuildReflectionMessage(errors)

	if !strings.Contains(msg, "topic-missing") {
		t.Error("expected message to contain rule ID")
	}
	if !strings.Contains(msg, "Missing topic") {
		t.Error("expected message to contain error message")
	}
}

func TestEstimateTokensCV(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},
		{"hello", 2},
	}

	for _, tt := range tests {
		result := estimateTokensCV(tt.input)
		if result != tt.expected {
			t.Errorf("estimateTokensCV(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}
