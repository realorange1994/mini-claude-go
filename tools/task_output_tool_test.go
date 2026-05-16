package tools

import (
	"strings"
	"testing"
	"time"
)

func TestTaskOutputToolProgressMode(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(agentID string, block bool, timeout time.Duration) (string, string) {
			return "full output", ""
		},
		GetProgressFunc: func(agentID string, lastN int) (string, string) {
			return "Task: test123\nStatus: running\nTotal lines: 42\nTotal bytes: 1024\nCompleted: no (still running)\n\n--- Last 100 lines ---\nlast line", ""
		},
	}

	result := tool.Execute(map[string]any{
		"task_id":  "test123",
		"progress": true,
	})
	if result.IsError {
		t.Errorf("progress mode should not return error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "running") {
		t.Error("progress output should contain status")
	}
	if !strings.Contains(result.Output, "Last 100 lines") {
		t.Error("progress output should contain last N lines header")
	}
}

func TestTaskOutputToolProgressNoCallback(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(agentID string, block bool, timeout time.Duration) (string, string) {
			return "output", ""
		},
	}

	result := tool.Execute(map[string]any{
		"task_id":  "test123",
		"progress": true,
	})
	if result.IsError {
		t.Error("should not error when progress callback missing")
	}
	if !strings.Contains(result.Output, "progress tracking not available") {
		t.Errorf("should mention progress not available, got: %s", result.Output)
	}
}

func TestTaskOutputToolStandardMode(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(agentID string, block bool, timeout time.Duration) (string, string) {
			return "Agent: test\nStatus: completed\nOutput:\nhello world", ""
		},
	}

	result := tool.Execute(map[string]any{
		"task_id": "test123",
	})
	if result.IsError {
		t.Error("standard mode should not return error")
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Error("standard mode should contain output")
	}
}

func TestTaskOutputToolMissingID(t *testing.T) {
	tool := &TaskOutputTool{
		GetOutputFunc: func(agentID string, block bool, timeout time.Duration) (string, string) {
			return "", ""
		},
	}

	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("should error when task_id missing")
	}
}

func TestTaskOutputToolNoCallback(t *testing.T) {
	tool := &TaskOutputTool{}

	result := tool.Execute(map[string]any{
		"task_id": "test123",
	})
	if !result.IsError {
		t.Error("should error when GetOutputFunc nil")
	}
}

func TestTaskOutputToolProgressCustomLastN(t *testing.T) {
	called := false
	tool := &TaskOutputTool{
		GetOutputFunc: func(agentID string, block bool, timeout time.Duration) (string, string) {
			return "", ""
		},
		GetProgressFunc: func(agentID string, lastN int) (string, string) {
			called = true
			if lastN != 50 {
				t.Errorf("expected lastN=50, got %d", lastN)
			}
			return "progress output", ""
		},
	}

	tool.Execute(map[string]any{
		"task_id":  "test123",
		"progress": true,
		"last_n":   float64(50),
	})

	if !called {
		t.Error("expected GetProgressFunc to be called")
	}
}
