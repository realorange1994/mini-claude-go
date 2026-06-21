package main

import (
	"testing"
)

func TestMemoryPathGuard_IsMemoryPath(t *testing.T) {
	g := NewMemoryPathGuard("/project")

	tests := []struct {
		path     string
		expected bool
	}{
		{"/project/.claude/memory/global.md", true},
		{"/project/.claude/session_memory.md", true},
		{"/project/.claude/checkpoints/cp-1.json", true},
		{"/project/main.go", false},
		{"/project/src/utils.go", false},
	}

	for _, tt := range tests {
		result := g.IsMemoryPath(tt.path)
		if result != tt.expected {
			t.Errorf("IsMemoryPath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestMemoryPathGuard_IsCheckpointWriterAllowed(t *testing.T) {
	g := NewMemoryPathGuard("/project")

	tests := []struct {
		path     string
		expected bool
	}{
		{"/project/.claude/memory/MEMORY.md", true},
		{"/project/.claude/checkpoints/checkpoint.md", true},
		{"/project/.claude/checkpoints/notes.md", true},
		{"/project/.claude/memory/tasks/T1/progress.md", true},
		{"/project/.claude/memory/pinned.md", false},
	}

	for _, tt := range tests {
		result := g.IsCheckpointWriterAllowed(tt.path)
		if result != tt.expected {
			t.Errorf("IsCheckpointWriterAllowed(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestMemoryPathGuard_IsReservedForCheckpointWriter(t *testing.T) {
	g := NewMemoryPathGuard("/project")

	tests := []struct {
		path     string
		expected bool
	}{
		{"/project/.claude/memory/tasks/T1/progress.md", true},
		{"/project/.claude/memory/MEMORY.md", false},
	}

	for _, tt := range tests {
		result := g.IsReservedForCheckpointWriter(tt.path)
		if result != tt.expected {
			t.Errorf("IsReservedForCheckpointWriter(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestMemoryPathGuard_IsMainAgentAllowed(t *testing.T) {
	g := NewMemoryPathGuard("/project")

	tests := []struct {
		path     string
		expected bool
	}{
		{"/project/.claude/memory/MEMORY.md", true},
		{"/project/.claude/memory/tasks/T1/progress.md", false},
		{"/project/main.go", true},
	}

	for _, tt := range tests {
		result := g.IsMainAgentAllowed(tt.path)
		if result != tt.expected {
			t.Errorf("IsMainAgentAllowed(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestMemoryPathGuard_IsSubagentAllowed(t *testing.T) {
	g := NewMemoryPathGuard("/project")

	tests := []struct {
		path     string
		agentID  string
		expected bool
	}{
		{"/project/.claude/memory/tasks/T1/progress.md", "T1", true},
		{"/project/.claude/memory/tasks/T2/progress.md", "T1", false},
		{"/project/.claude/memory/MEMORY.md", "T1", false},
	}

	for _, tt := range tests {
		result := g.IsSubagentAllowed(tt.path, tt.agentID)
		if result != tt.expected {
			t.Errorf("IsSubagentAllowed(%q, %q) = %v, want %v", tt.path, tt.agentID, result, tt.expected)
		}
	}
}

func TestMemoryPathGuard_ValidateMemoryWrite(t *testing.T) {
	g := NewMemoryPathGuard("/project")

	// Checkpoint writer can write MEMORY.md
	err := g.ValidateMemoryWrite("/project/.claude/memory/MEMORY.md", "checkpoint-writer", "")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Main agent cannot write tasks
	err = g.ValidateMemoryWrite("/project/.claude/memory/tasks/T1/progress.md", "main", "")
	if err == nil {
		t.Error("expected error for main agent writing tasks")
	}

	// Subagent can write own task
	err = g.ValidateMemoryWrite("/project/.claude/memory/tasks/T1/progress.md", "subagent", "T1")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Subagent cannot write other task
	err = g.ValidateMemoryWrite("/project/.claude/memory/tasks/T2/progress.md", "subagent", "T1")
	if err == nil {
		t.Error("expected error for subagent writing other task")
	}
}

func TestFormatMemoryPathError(t *testing.T) {
	tests := []struct {
		path      string
		agentType string
		agentID   string
		contains  string
	}{
		{"/path", "checkpoint-writer", "", "checkpoint writer"},
		{"/path", "main", "", "reserved"},
		{"/path", "subagent", "T1", "subagent T1"},
	}

	for _, tt := range tests {
		result := FormatMemoryPathError(tt.path, tt.agentType, tt.agentID)
		if result == "" {
			t.Error("expected non-empty error message")
		}
	}
}
