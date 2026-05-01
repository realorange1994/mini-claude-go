package main

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewTaskStore(t *testing.T) {
	ts := NewTaskStore()
	if ts == nil {
		t.Fatal("NewTaskStore returned nil")
	}
	if len(ts.AllTasks()) != 0 {
		t.Fatalf("expected empty store, got %d tasks", len(ts.AllTasks()))
	}
}

func TestTaskStoreCreateAndGet(t *testing.T) {
	ts := NewTaskStore()
	task := ts.CreateTask("agent-1", "test task", "claude-3", "general")

	if task == nil {
		t.Fatal("CreateTask returned nil")
	}
	if task.ID != "agent-1" {
		t.Errorf("expected ID=agent-1, got %s", task.ID)
	}
	if task.Status != TaskStatusPending {
		t.Errorf("expected Status=Pending, got %d", task.Status)
	}
	if task.Description != "test task" {
		t.Errorf("expected Description=test task, got %s", task.Description)
	}
	if task.Model != "claude-3" {
		t.Errorf("expected Model=claude-3, got %s", task.Model)
	}
	if task.SubagentType != "general" {
		t.Errorf("expected SubagentType=general, got %s", task.SubagentType)
	}

	got := ts.GetTask("agent-1")
	if got == nil {
		t.Fatal("GetTask returned nil for existing task")
	}
	if got.ID != "agent-1" {
		t.Errorf("GetTask: expected ID=agent-1, got %s", got.ID)
	}

	missing := ts.GetTask("nonexistent")
	if missing != nil {
		t.Error("GetTask should return nil for nonexistent task")
	}
}

func TestTaskStoreCompleteTask(t *testing.T) {
	ts := NewTaskStore()
	ts.CreateTask("agent-1", "test", "model", "type")

	ts.CompleteTask("agent-1", "task result", 5, 1234)

	task := ts.GetTask("agent-1")
	if task.Status != TaskStatusCompleted {
		t.Errorf("expected Status=Completed, got %d", task.Status)
	}
	if task.Result != "task result" {
		t.Errorf("expected Result=task result, got %s", task.Result)
	}
	if task.ToolsUsed != 5 {
		t.Errorf("expected ToolsUsed=5, got %d", task.ToolsUsed)
	}
	if task.DurationMs != 1234 {
		t.Errorf("expected DurationMs=1234, got %d", task.DurationMs)
	}
	if task.EndTime.IsZero() {
		t.Error("EndTime should be set after CompleteTask")
	}
}

func TestTaskStoreFailTask(t *testing.T) {
	ts := NewTaskStore()
	ts.CreateTask("agent-1", "test", "model", "type")

	ts.FailTask("agent-1", "something went wrong")

	task := ts.GetTask("agent-1")
	if task.Status != TaskStatusFailed {
		t.Errorf("expected Status=Failed, got %d", task.Status)
	}
	if task.Error != "something went wrong" {
		t.Errorf("expected Error=something went wrong, got %s", task.Error)
	}
	if task.EndTime.IsZero() {
		t.Error("EndTime should be set after FailTask")
	}
}

func TestTaskStoreCompleteNonexistent(t *testing.T) {
	ts := NewTaskStore()
	// Should not panic
	ts.CompleteTask("nonexistent", "result", 0, 0)
	ts.FailTask("nonexistent", "error")
}

func TestTaskStateIsTerminal(t *testing.T) {
	ts := NewTaskStore()
	task := ts.CreateTask("agent-1", "test", "model", "type")

	if task.IsTerminal() {
		t.Error("Pending task should not be terminal")
	}

	task.Status = TaskStatusRunning
	if task.IsTerminal() {
		t.Error("Running task should not be terminal")
	}

	task.Status = TaskStatusCompleted
	if !task.IsTerminal() {
		t.Error("Completed task should be terminal")
	}

	task.Status = TaskStatusFailed
	if !task.IsTerminal() {
		t.Error("Failed task should be terminal")
	}

	task.Status = TaskStatusKilled
	if !task.IsTerminal() {
		t.Error("Killed task should be terminal")
	}
}

func TestTaskStoreAllTasks(t *testing.T) {
	ts := NewTaskStore()
	ts.CreateTask("agent-3", "third", "model", "type")
	time.Sleep(time.Millisecond)
	ts.CreateTask("agent-1", "first", "model", "type")
	time.Sleep(time.Millisecond)
	ts.CreateTask("agent-2", "second", "model", "type")

	all := ts.AllTasks()
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(all))
	}
	// Should be sorted by StartTime (oldest first)
	if all[0].ID != "agent-3" {
		t.Errorf("expected first task ID=agent-3, got %s", all[0].ID)
	}
	if all[1].ID != "agent-1" {
		t.Errorf("expected second task ID=agent-1, got %s", all[1].ID)
	}
	if all[2].ID != "agent-2" {
		t.Errorf("expected third task ID=agent-2, got %s", all[2].ID)
	}
}

func TestTaskStoreConcurrentAccess(t *testing.T) {
	ts := NewTaskStore()
	var wg sync.WaitGroup

	// Concurrent creates
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("agent-%d", i)
			ts.CreateTask(id, "concurrent task", "model", "type")
		}(i)
	}
	wg.Wait()

	all := ts.AllTasks()
	if len(all) != 100 {
		t.Fatalf("expected 100 tasks, got %d", len(all))
	}

	// Concurrent reads and writes
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			ts.GetTask(fmt.Sprintf("agent-%d", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			ts.CompleteTask(fmt.Sprintf("agent-%d", i), "done", 1, 100)
		}(i)
		go func(i int) {
			defer wg.Done()
			ts.AllTasks()
		}(i)
	}
	wg.Wait()

	// Verify all tasks are completed
	for _, task := range ts.AllTasks() {
		if task.Status != TaskStatusCompleted {
			t.Errorf("task %s: expected Completed, got %d", task.ID, task.Status)
		}
	}
}
