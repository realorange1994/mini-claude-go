package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewAgentTaskStore(t *testing.T) {
	ts := NewAgentTaskStore()
	if ts == nil {
		t.Fatal("NewAgentTaskStore returned nil")
	}
	if ts.tasks == nil {
		t.Error("tasks map is nil")
	}
}

func TestAgentTaskStoreCreateAndGet(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test description", "explore", "test prompt", "claude-sonnet-4-20250514")
	if task == nil {
		t.Fatal("Create returned nil")
	}
	if task.Status != TaskPending {
		t.Errorf("expected status pending, got %s", task.Status)
	}
	if task.Type != "local_agent" {
		t.Errorf("expected type 'local_agent', got %s", task.Type)
	}
	if task.Description != "test description" {
		t.Errorf("expected description 'test description', got %s", task.Description)
	}
	if task.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got %s", task.Model)
	}
	if task.SubagentType != "explore" {
		t.Errorf("expected subagentType 'explore', got %s", task.SubagentType)
	}
	if task.Prompt != "test prompt" {
		t.Errorf("expected prompt 'test prompt', got %s", task.Prompt)
	}

	// Verify Get returns the same task
	got := ts.Get(task.ID)
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, got.ID)
	}
}

func TestAgentTaskStoreCreateWithID(t *testing.T) {
	ts := NewAgentTaskStore()
	taskID := "test-task-001"
	task := ts.CreateWithID(taskID, "test desc", "plan", "test prompt", "model-v1")
	if task == nil {
		t.Fatal("CreateWithID returned nil")
	}
	if task.ID != taskID {
		t.Errorf("expected ID %s, got %s", taskID, task.ID)
	}

	got := ts.Get(taskID)
	if got == nil {
		t.Fatal("Get returned nil for created task")
	}
	if got.ID != taskID {
		t.Errorf("expected Get(%s) to return task with ID %s, got %s", taskID, taskID, got.ID)
	}
}

func TestAgentTaskStoreStartCompleteFailKill(t *testing.T) {
	ts := NewAgentTaskStore()

	// Create task
	task := ts.Create("test", "", "", "")
	taskID := task.ID

	// Start with cancel func
	_, cancel := context.WithCancel(context.Background())
	ts.Start(taskID, cancel)
	task = ts.Get(taskID)
	if task.Status != TaskRunning {
		t.Errorf("expected status running after Start, got %s", task.Status)
	}

	// Complete
	ts.Complete(taskID)
	task = ts.Get(taskID)
	if task.Status != TaskCompleted {
		t.Errorf("expected status completed, got %s", task.Status)
	}

	// Test Fail
	task2 := ts.Create("test2", "", "", "")
	ts.Fail(task2.ID, fmt.Errorf("some error"))
	task2 = ts.Get(task2.ID)
	if task2.Status != TaskFailed {
		t.Errorf("expected status failed, got %s", task2.Status)
	}

	// Test Kill
	task3 := ts.Create("test3", "", "", "")
	ts.Start(task3.ID, func() {})
	ts.Kill(task3.ID)
	task3 = ts.Get(task3.ID)
	if task3.Status != TaskKilled {
		t.Errorf("expected status killed, got %s", task3.Status)
	}
}

func TestAgentTaskIsTerminal(t *testing.T) {
	ts := NewAgentTaskStore()

	task := ts.Create("test", "", "", "")
	if task.IsTerminal() {
		t.Error("new task should not be terminal")
	}

	ts.Complete(task.ID)
	task = ts.Get(task.ID)
	if !task.IsTerminal() {
		t.Error("completed task should be terminal")
	}

	task2 := ts.Create("test2", "", "", "")
	ts.Fail(task2.ID, fmt.Errorf("err"))
	task2 = ts.Get(task2.ID)
	if !task2.IsTerminal() {
		t.Error("failed task should be terminal")
	}

	task3 := ts.Create("test3", "", "", "")
	ts.Start(task3.ID, func() {})
	ts.Kill(task3.ID)
	task3 = ts.Get(task3.ID)
	if !task3.IsTerminal() {
		t.Error("killed task should be terminal")
	}
}

func TestAgentTaskWriteOutput(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")

	// Write some output
	task.WriteOutput("line1\n")
	task.WriteOutput("line2\n")
	task.WriteOutput("line3\n")

	output := task.GetOutput()
	if output != "line1\nline2\nline3\n" {
		t.Errorf("unexpected output: %q", output)
	}
}

func TestAgentTaskOutputCap(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")

	// First, write some initial content so there's something to truncate
	task.WriteOutput(strings.Repeat("a", 1000))

	// Now write more than 50KB to exceed the cap
	large := make([]byte, maxOutputBuffer+1000)
	for i := range large {
		large[i] = 'x'
	}
	task.WriteOutput(string(large))

	output := task.GetOutput()
	if len(output) > maxOutputBuffer {
		t.Errorf("output exceeded maxOutputBuffer: %d > %d", len(output), maxOutputBuffer)
	}
	// Should contain truncation marker
	if !containsString(output, "chars truncated") {
		t.Error("output should contain truncation marker")
	}
}

func TestAgentTaskStoreList(t *testing.T) {
	ts := NewAgentTaskStore()
	task1 := ts.Create("task1", "", "", "")
	task1.StartTime = time.Now().Add(-3 * time.Second)
	task2 := ts.Create("task2", "", "", "")
	task2.StartTime = time.Now().Add(-2 * time.Second)
	task3 := ts.Create("task3", "", "", "")
	task3.StartTime = time.Now().Add(-1 * time.Second)

	tasks := ts.List()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
	// Should be sorted newest first
	if !tasks[0].StartTime.After(tasks[1].StartTime) {
		t.Error("tasks should be sorted newest first")
	}
}

func TestAgentTaskStoreListByStatus(t *testing.T) {
	ts := NewAgentTaskStore()
	ts.Create("t1", "", "", "") // pending
	ts.Create("t2", "", "", "") // pending

	tasks := ts.ListByStatus(TaskPending)
	if len(tasks) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(tasks))
	}

	tasks2 := ts.ListByStatus(TaskRunning)
	if len(tasks2) != 0 {
		t.Errorf("expected 0 running tasks, got %d", len(tasks2))
	}
}

func TestAgentTaskStoreConcurrentAccess(t *testing.T) {
	ts := NewAgentTaskStore()

	var wg sync.WaitGroup
	ids := make([]string, 100)

	// Concurrently create tasks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := ts.Create(fmt.Sprintf("task%d", idx), "", "", "")
			ids[idx] = task.ID
		}(i)
	}
	wg.Wait()

	// Concurrently read tasks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := ts.Get(ids[idx])
			if task == nil {
				t.Errorf("task %d not found", idx)
			}
		}(i)
	}
	wg.Wait()

	// Concurrently write output
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := ts.Get(ids[idx])
			if task != nil {
				task.WriteOutput(fmt.Sprintf("output%d", idx))
			}
		}(i)
	}
	wg.Wait()
}

func TestAgentTaskSetStatus(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")

	task.SetStatus(TaskRunning)
	if task.Status != TaskRunning {
		t.Errorf("expected TaskRunning, got %s", task.Status)
	}
	if task.Status != TaskRunning {
		t.Errorf("task.GetStatus mismatch")
	}
}

func TestAgentTaskSetToolsInfo(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")

	task.SetToolsInfo(42, 1234)
	if task.ToolsUsed != 42 {
		t.Errorf("expected ToolsUsed=42, got %d", task.ToolsUsed)
	}
	if task.DurationMs != 1234 {
		t.Errorf("expected DurationMs=1234, got %d", task.DurationMs)
	}
}

func TestAgentTaskStoreUpdateTranscriptPath(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")

	ts.UpdateTranscriptPath(task.ID, "/path/to/transcript.jsonl")
	task = ts.Get(task.ID)
	if task.TranscriptPath != "/path/to/transcript.jsonl" {
		t.Errorf("expected transcript path '/path/to/transcript.jsonl', got %s", task.TranscriptPath)
	}
}

func TestAgentTaskStoreDelete(t *testing.T) {
	ts := NewAgentTaskStore()
	task := ts.Create("test", "", "", "")
	taskID := task.ID

	ts.Delete(taskID)
	if ts.Get(taskID) != nil {
		t.Error("Get should return nil after Delete")
	}
	if ts.Count() != 0 {
		t.Errorf("expected 0 tasks after Delete, got %d", ts.Count())
	}
}

func TestAgentTaskStoreCount(t *testing.T) {
	ts := NewAgentTaskStore()
	if ts.Count() != 0 {
		t.Errorf("expected 0 tasks, got %d", ts.Count())
	}

	task1 := ts.Create("t1", "", "", "")
	ts.Create("t2", "", "", "")
	if ts.Count() != 2 {
		t.Errorf("expected 2 tasks, got %d", ts.Count())
	}

	ts.Delete(task1.ID)
	if ts.Count() != 1 {
		t.Errorf("expected 1 task after delete, got %d", ts.Count())
	}
}

func TestAgentTaskStatusString(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   string
	}{
		{TaskPending, "pending"},
		{TaskRunning, "running"},
		{TaskCompleted, "completed"},
		{TaskFailed, "failed"},
		{TaskKilled, "killed"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("%s.String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
