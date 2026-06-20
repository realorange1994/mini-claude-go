package main

import (
	"strings"
	"testing"
)

func TestPruneLevel(t *testing.T) {
	tests := []struct {
		pressure int
		expected int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 2},
	}

	for _, tt := range tests {
		result := PruneLevel(tt.pressure)
		if result != tt.expected {
			t.Errorf("PruneLevel(%d) = %d, want %d", tt.pressure, result, tt.expected)
		}
	}
}

func TestPruneOldToolOutputs_NoPruning(t *testing.T) {
	outputs := []ToolOutput{
		{ToolName: "bash", Output: "short output", TurnIndex: 0},
		{ToolName: "bash", Output: "another output", TurnIndex: 1},
	}

	result := PruneOldToolOutputs(outputs, 0)
	if result.SoftTrimmed != 0 || result.HardCleared != 0 {
		t.Error("expected no pruning at pressure 0")
	}
}

func TestPruneOldToolOutputs_SoftTrim(t *testing.T) {
	// Create enough outputs to exceed PruneProtect (40K tokens = 160K chars)
	var outputs []ToolOutput
	for i := 0; i < 40; i++ {
		outputs = append(outputs, ToolOutput{
			ToolName:  "bash",
			Output:    strings.Repeat("x", 5000), // ~1250 tokens each
			TurnIndex: i,
		})
	}
	// Add recent protected turns
	for i := 40; i < 45; i++ {
		outputs = append(outputs, ToolOutput{
			ToolName:  "bash",
			Output:    "recent output",
			TurnIndex: i,
		})
	}

	result := PruneOldToolOutputs(outputs, 1)
	if result.SoftTrimmed == 0 {
		t.Error("expected soft-trimming at pressure 1")
	}
}

func TestPruneOldToolOutputs_HardClear(t *testing.T) {
	// Create enough outputs to exceed PruneProtect (40K tokens = 160K chars)
	var outputs []ToolOutput
	for i := 0; i < 40; i++ {
		outputs = append(outputs, ToolOutput{
			ToolName:  "bash",
			Output:    strings.Repeat("x", 5000), // ~1250 tokens each
			TurnIndex: i,
		})
	}
	// Add recent protected turns
	for i := 40; i < 45; i++ {
		outputs = append(outputs, ToolOutput{
			ToolName:  "bash",
			Output:    "recent output",
			TurnIndex: i,
		})
	}

	result := PruneOldToolOutputs(outputs, 2)
	if result.HardCleared == 0 {
		t.Error("expected hard-clearing at pressure 2")
	}
}

func TestPruneOldToolOutputs_ProtectedTools(t *testing.T) {
	largeOutput := strings.Repeat("x", 5000)
	outputs := []ToolOutput{
		{ToolName: "skill", Output: largeOutput, TurnIndex: 0},
		{ToolName: "bash", Output: "recent 1", TurnIndex: 1},
		{ToolName: "bash", Output: "recent 2", TurnIndex: 2},
		{ToolName: "bash", Output: "recent 3", TurnIndex: 3},
		{ToolName: "bash", Output: "recent 4", TurnIndex: 4},
		{ToolName: "bash", Output: "recent 5", TurnIndex: 5},
	}

	result := PruneOldToolOutputs(outputs, 2)
	// skill tool should be protected
	if result.SoftTrimmed != 0 && result.HardCleared != 0 {
		t.Error("expected skill tool to be protected")
	}
}

func TestPruneOldToolOutputs_AlreadyCompacted(t *testing.T) {
	outputs := []ToolOutput{
		{ToolName: "bash", Output: strings.Repeat("x", 5000), TurnIndex: 0, Compacted: true},
		{ToolName: "bash", Output: "recent 1", TurnIndex: 1},
		{ToolName: "bash", Output: "recent 2", TurnIndex: 2},
		{ToolName: "bash", Output: "recent 3", TurnIndex: 3},
	}

	result := PruneOldToolOutputs(outputs, 2)
	if result.HardCleared != 0 {
		t.Error("expected no hard-clearing for already compacted")
	}
}

func TestSoftTrimOutput_Small(t *testing.T) {
	output := "short"
	result := softTrimOutput(output)
	if result != "short" {
		t.Error("expected no trimming for small output")
	}
}

func TestSoftTrimOutput_Large(t *testing.T) {
	output := strings.Repeat("x", 5000)
	result := softTrimOutput(output)
	if !strings.Contains(result, "trimmed") {
		t.Error("expected trimming marker")
	}
	if !strings.Contains(result, "5000") {
		t.Error("expected original size in marker")
	}
}

func TestIsProtectedTool(t *testing.T) {
	if !isProtectedTool("skill") {
		t.Error("expected skill to be protected")
	}
	if !isProtectedTool("search_skills") {
		t.Error("expected search_skills to be protected")
	}
	if isProtectedTool("bash") {
		t.Error("expected bash to NOT be protected")
	}
}

func TestEstimateTokensPrune(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},
		{"hello", 2},
	}

	for _, tt := range tests {
		result := estimateTokensPrune(tt.input)
		if result != tt.expected {
			t.Errorf("estimateTokensPrune(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}
