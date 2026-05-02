package tools

import (
	"strings"
	"testing"
)

func setupTestStore() *AgentTaskStore {
	ts := NewAgentTaskStore()
	// Create a few tasks
	ts.Create("explore repo structure", "explore", "Look at the code", "claude-sonnet-4-20250514")
	ts.Create("fix bug #42", "fix", "Fix the login bug", "claude-sonnet-4-20250514")
	return ts
}

func TestAgentListTool(t *testing.T) {
	ts := setupTestStore()
	tool := &AgentListTool{Store: ts}

	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "explore repo structure") {
		t.Errorf("expected 'explore repo structure' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "2 agent(s)") {
		t.Errorf("expected '2 agent(s)' in output, got: %s", result.Output)
	}
}

func TestAgentListToolWithStatusFilter(t *testing.T) {
	ts := setupTestStore()
	tool := &AgentListTool{Store: ts}

	// Filter by running (none)
	result := tool.Execute(map[string]any{"status": "running"})
	if strings.Contains(result.Output, "explore") {
		t.Errorf("should not show pending tasks when filtering for running, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No agents") {
		t.Errorf("expected 'No agents' message, got: %s", result.Output)
	}
}

func TestAgentListToolNilStore(t *testing.T) {
	tool := &AgentListTool{Store: nil}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("expected error for nil store")
	}
}

func TestAgentGetTool(t *testing.T) {
	ts := setupTestStore()
	tool := &AgentGetTool{Store: ts}

	tasks := ts.List()
	taskID := tasks[0].ID

	result := tool.Execute(map[string]any{"agent_id": taskID})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, taskID) {
		t.Errorf("expected task ID %s in output, got: %s", taskID, result.Output)
	}
	if !strings.Contains(result.Output, "pending") {
		t.Errorf("expected 'pending' status in output, got: %s", result.Output)
	}
}

func TestAgentGetToolNotFound(t *testing.T) {
	ts := setupTestStore()
	tool := &AgentGetTool{Store: ts}

	result := tool.Execute(map[string]any{"agent_id": "nonexistent"})
	if !result.IsError {
		t.Error("expected error for nonexistent agent")
	}
}

func TestAgentGetToolNoAgentID(t *testing.T) {
	ts := setupTestStore()
	tool := &AgentGetTool{Store: ts}

	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("expected error when agent_id is missing")
	}
}

func TestAgentGetToolWithOutput(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")
	task.WriteOutput("Hello from agent\nLine 2\n")

	tool := &AgentGetTool{Store: ts}
	result := tool.Execute(map[string]any{"agent_id": task.ID})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Hello from agent") {
		t.Errorf("expected agent output in result, got: %s", result.Output)
	}
}

func TestAgentKillTool(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")
	ts.Start(task.ID, func() {})

	tool := &AgentKillTool{Store: ts}
	result := tool.Execute(map[string]any{"agent_id": task.ID})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "killed") {
		t.Errorf("expected 'killed' in output, got: %s", result.Output)
	}

	// Verify task is killed
	got := ts.Get(task.ID)
	if got.Status != TaskKilled {
		t.Errorf("expected TaskKilled, got %s", got.Status)
	}
}

func TestAgentKillToolNotRunning(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")
	ts.Complete(task.ID) // Complete the task so it's terminal

	tool := &AgentKillTool{Store: ts}
	result := tool.Execute(map[string]any{"agent_id": task.ID})
	if !result.IsError {
		t.Error("expected error when killing a completed agent")
	}
}

func TestAgentKillToolNotFound(t *testing.T) {
	ts := NewAgentTaskStore()
	tool := &AgentKillTool{Store: ts}
	result := tool.Execute(map[string]any{"agent_id": "nonexistent"})
	if !result.IsError {
		t.Error("expected error for nonexistent agent")
	}
}

func TestAgentToolInputSchema(t *testing.T) {
	ts := NewAgentTaskStore()

	listTool := &AgentListTool{Store: ts}
	if listTool.Name() != "agent_list" {
		t.Errorf("expected name 'agent_list', got %s", listTool.Name())
	}
	schema := listTool.InputSchema()
	if schema == nil {
		t.Error("InputSchema should not be nil")
	}

	getTool := &AgentGetTool{Store: ts}
	if getTool.Name() != "agent_get" {
		t.Errorf("expected name 'agent_get', got %s", getTool.Name())
	}

	killTool := &AgentKillTool{Store: ts}
	if killTool.Name() != "agent_kill" {
		t.Errorf("expected name 'agent_kill', got %s", killTool.Name())
	}
}

func TestAgentListToolEmptyStore(t *testing.T) {
	ts := NewAgentTaskStore()
	tool := &AgentListTool{Store: ts}

	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No agents") {
		t.Errorf("expected 'No agents' message, got: %s", result.Output)
	}
}

func TestAgentGetToolTailParameter(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")

	// Write 100 lines
	for i := 0; i < 100; i++ {
		task.WriteOutput("This is line that is reasonably long to test tail truncation behavior in the agent get tool output display feature number " + strings.Repeat("x", 20) + "\n")
	}

	tool := &AgentGetTool{Store: ts}
	result := tool.Execute(map[string]any{"agent_id": task.ID, "tail": float64(10)})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "earlier lines omitted") {
		t.Errorf("expected truncation message, got: %s", result.Output[:200])
	}
}
