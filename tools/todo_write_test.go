package tools

import (
	"testing"
)

// ─── TodoList ────────────────────────────────────────────────────────────────

func TestNewTodoList(t *testing.T) {
	list := NewTodoList()
	if len(list.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(list.Items))
	}
}

func TestTodoListUpdate(t *testing.T) {
	list := NewTodoList()
	items := []TodoItem{
		{Content: "Task 1", Status: TodoPending},
		{Content: "Task 2", Status: TodoInProgress},
	}
	list.Update(items)
	if len(list.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list.Items))
	}
	if list.Items[0].Content != "Task 1" {
		t.Errorf("expected 'Task 1', got %q", list.Items[0].Content)
	}
}

func TestTodoListBuildReminderEmpty(t *testing.T) {
	list := NewTodoList()
	if list.BuildReminder() != "" {
		t.Error("empty todo list should have empty reminder")
	}
}

func TestTodoListBuildReminderWithItems(t *testing.T) {
	list := NewTodoList()
	list.Update([]TodoItem{
		{Content: "Do work", Status: TodoInProgress, ActiveForm: "Doing work"},
	})
	reminder := list.BuildReminder()
	if reminder == "" {
		t.Error("non-empty todo list should have non-empty reminder")
	}
}

func TestTodoListStatusIcons(t *testing.T) {
	list := NewTodoList()
	list.Update([]TodoItem{
		{Content: "Pending", Status: TodoPending},
		{Content: "In Progress", Status: TodoInProgress},
		{Content: "Done", Status: TodoCompleted},
	})
	reminder := list.BuildReminder()
	// Check different status icons
	if reminder == "" {
		t.Error("reminder should not be empty")
	}
}

func TestTodoListBuildIdleReminder(t *testing.T) {
	list := NewTodoList()
	reminder := list.BuildIdleReminder()
	if reminder == "" {
		t.Error("idle reminder should not be empty")
	}
}

func TestTodoListIncrementTurn(t *testing.T) {
	list := NewTodoList()
	// First few turns should not trigger reminder
	for i := 0; i < 9; i++ {
		if list.IncrementTurn() {
			t.Errorf("should not remind on turn %d", i+1)
		}
	}
	// 10th turn should trigger
	if !list.IncrementTurn() {
		t.Error("should remind on turn 10")
	}
}

func TestTodoListIncrementTurnResetsCounter(t *testing.T) {
	list := NewTodoList()
	// Reach reminder threshold
	for i := 0; i < 10; i++ {
		list.IncrementTurn()
	}
	// Should not remind again immediately
	for i := 0; i < 9; i++ {
		if list.IncrementTurn() {
			t.Errorf("should not remind immediately after reminder, turn %d", i+1)
		}
	}
	// After another 10 turns, should remind again
	if !list.IncrementTurn() {
		t.Error("should remind after second 10 turns")
	}
}

func TestTodoListUpdateResetsCounter(t *testing.T) {
	list := NewTodoList()
	// Increment a few times
	for i := 0; i < 5; i++ {
		list.IncrementTurn()
	}
	// Update the list
	list.Update([]TodoItem{{Content: "New task", Status: TodoPending}})
	// Counter should have been reset, so we need 10 more turns to remind
	for i := 0; i < 10; i++ {
		if list.IncrementTurn() && i < 9 {
			t.Errorf("should not remind until turn 10 after update, reminded on turn %d", i+1)
		}
	}
}

// ─── TodoWriteTool ───────────────────────────────────────────────────────────

func TestTodoWriteToolExecute(t *testing.T) {
	list := NewTodoList()
	tool := TodoWriteTool{TodoList: list}

	result := tool.Execute(map[string]any{
		"todos": []any{
			map[string]any{"content": "Task A", "status": "pending"},
			map[string]any{"content": "Task B", "status": "in_progress"},
		},
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if len(list.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(list.Items))
	}
}

func TestTodoWriteToolMissingTodos(t *testing.T) {
	list := NewTodoList()
	tool := TodoWriteTool{TodoList: list}

	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing todos should return error")
	}
}

func TestTodoWriteToolInvalidTodos(t *testing.T) {
	list := NewTodoList()
	tool := TodoWriteTool{TodoList: list}

	result := tool.Execute(map[string]any{
		"todos": []any{
			"not a map",
			map[string]any{"content": "Valid", "status": "pending"},
		},
	})
	// Should still succeed with valid items, skipping invalid ones
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if len(list.Items) != 1 {
		t.Errorf("expected 1 valid item, got %d", len(list.Items))
	}
}

// ─── TodoStatus constants ───────────────────────────────────────────────────

func TestTodoStatusConstants(t *testing.T) {
	if TodoPending != "pending" {
		t.Errorf("TodoPending = %q, want 'pending'", TodoPending)
	}
	if TodoInProgress != "in_progress" {
		t.Errorf("TodoInProgress = %q, want 'in_progress'", TodoInProgress)
	}
	if TodoCompleted != "completed" {
		t.Errorf("TodoCompleted = %q, want 'completed'", TodoCompleted)
	}
}
