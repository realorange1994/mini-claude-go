package main

import (
	"errors"
	"testing"
)

func TestClassifyAssistantStep_Final(t *testing.T) {
	result := ClassifyAssistantStep("Here is your answer", nil, "", nil)
	if result.Category != StepFinal {
		t.Errorf("expected final, got %s", result.Category)
	}
	if !result.HasText {
		t.Error("expected HasText=true")
	}
}

func TestClassifyAssistantStep_Continue(t *testing.T) {
	toolCalls := []map[string]any{{"name": "bash"}}
	result := ClassifyAssistantStep("", toolCalls, "", nil)
	if result.Category != StepContinue {
		t.Errorf("expected continue, got %s", result.Category)
	}
	if !result.HasTools {
		t.Error("expected HasTools=true")
	}
}

func TestClassifyAssistantStep_ThinkOnly(t *testing.T) {
	result := ClassifyAssistantStep("", nil, "thinking about the problem", nil)
	if result.Category != StepThinkOnly {
		t.Errorf("expected think-only, got %s", result.Category)
	}
	if !result.HasThinking {
		t.Error("expected HasThinking=true")
	}
}

func TestClassifyAssistantStep_Invalid(t *testing.T) {
	result := ClassifyAssistantStep("", nil, "", nil)
	if result.Category != StepInvalid {
		t.Errorf("expected invalid, got %s", result.Category)
	}
}

func TestClassifyAssistantStep_Failed(t *testing.T) {
	err := errors.New("connection timeout")
	result := ClassifyAssistantStep("", nil, "", err)
	if result.Category != StepFailed {
		t.Errorf("expected failed, got %s", result.Category)
	}
	if result.ErrorMsg != "connection timeout" {
		t.Errorf("expected error message, got %q", result.ErrorMsg)
	}
}

func TestClassifyAssistantStep_WithThinkingAndText(t *testing.T) {
	result := ClassifyAssistantStep("answer", nil, "reasoning", nil)
	if result.Category != StepFinal {
		t.Errorf("expected final, got %s", result.Category)
	}
	if !result.HasThinking {
		t.Error("expected HasThinking=true")
	}
}

func TestShouldContinue(t *testing.T) {
	tests := []struct {
		category StepCategory
		expected bool
	}{
		{StepContinue, true},
		{StepThinkOnly, true},
		{StepFinal, false},
		{StepFiltered, false},
		{StepInvalid, false},
		{StepFailed, false},
	}

	for _, tt := range tests {
		result := ShouldContinue(StepClassification{Category: tt.category})
		if result != tt.expected {
			t.Errorf("ShouldContinue(%s) = %v, want %v", tt.category, result, tt.expected)
		}
	}
}
