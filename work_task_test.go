package main

import (
	"testing"
)

func TestWorkTaskStore_CreateTask(t *testing.T) {
	store := NewWorkTaskStore()

	id := store.CreateTask("Fix bug", "Fix the authentication bug", "Fixing bug", nil)
	if id == "" {
		t.Fatal("expected non-empty task ID")
	}
	if id != "1" {
		t.Fatalf("expected ID '1', got '%s'", id)
	}

	task := store.GetTask(id)
	if task == nil {
		t.Fatal("task not found")
	}
	if task.Subject != "Fix bug" {
		t.Errorf("expected subject 'Fix bug', got '%s'", task.Subject)
	}
	if task.Description != "Fix the authentication bug" {
		t.Errorf("expected description 'Fix the authentication bug', got '%s'", task.Description)
	}
	if task.Status != WorkTaskPending {
		t.Errorf("expected status pending, got '%s'", task.Status)
	}
	if task.ActiveForm != "Fixing bug" {
		t.Errorf("expected active form 'Fixing bug', got '%s'", task.ActiveForm)
	}
}

func TestWorkTaskStore_CreateTask_IncrementingIDs(t *testing.T) {
	store := NewWorkTaskStore()

	id1 := store.CreateTask("Task 1", "Description 1", "", nil)
	id2 := store.CreateTask("Task 2", "Description 2", "", nil)

	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}
}

func TestWorkTaskStore_CreateTask_WithMetadata(t *testing.T) {
	store := NewWorkTaskStore()

	metadata := map[string]any{"priority": "high", "tags": []string{"bugfix"}}
	id := store.CreateTask("Fix bug", "Fix it", "", metadata)

	task := store.GetTask(id)
	if task == nil {
		t.Fatal("task not found")
	}
	if task.Metadata["priority"] != "high" {
		t.Errorf("expected metadata priority 'high', got '%v'", task.Metadata["priority"])
	}
}

func TestWorkTaskStore_ListTasks(t *testing.T) {
	store := NewWorkTaskStore()

	store.CreateTask("Task 1", "Desc 1", "", nil)
	store.CreateTask("Task 2", "Desc 2", "", nil)
	store.CreateTask("Task 3", "Desc 3", "", nil)

	tasks := store.ListTasks()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Verify ordering (by ID)
	if tasks[0].ID != "1" || tasks[1].ID != "2" || tasks[2].ID != "3" {
		t.Errorf("unexpected ordering: %s, %s, %s", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
}

func TestWorkTaskStore_ListTasks_ExcludesDeleted(t *testing.T) {
	store := NewWorkTaskStore()

	store.CreateTask("Task 1", "Desc 1", "", nil)
	id2 := store.CreateTask("Task 2", "Desc 2", "", nil)
	store.CreateTask("Task 3", "Desc 3", "", nil)

	// Delete task 2
	store.UpdateTask(id2, map[string]any{"status": string(WorkTaskDeleted)})

	tasks := store.ListTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks (deleted excluded), got %d", len(tasks))
	}
}

func TestWorkTaskStore_UpdateTask_Status(t *testing.T) {
	store := NewWorkTaskStore()

	id := store.CreateTask("Fix bug", "Fix it", "", nil)

	err := store.UpdateTask(id, map[string]any{"status": string(WorkTaskInProgress)})
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Status != WorkTaskInProgress {
		t.Errorf("expected status in_progress, got '%s'", task.Status)
	}

	err = store.UpdateTask(id, map[string]any{"status": string(WorkTaskCompleted)})
	if err != nil {
		t.Fatal(err)
	}

	task = store.GetTask(id)
	if task.Status != WorkTaskCompleted {
		t.Errorf("expected status completed, got '%s'", task.Status)
	}
}

func TestWorkTaskStore_UpdateTask_InvalidStatus(t *testing.T) {
	store := NewWorkTaskStore()

	id := store.CreateTask("Fix bug", "Fix it", "", nil)

	err := store.UpdateTask(id, map[string]any{"status": "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestWorkTaskStore_UpdateTask_SubjectAndDescription(t *testing.T) {
	store := NewWorkTaskStore()

	id := store.CreateTask("Old Subject", "Old Description", "", nil)

	err := store.UpdateTask(id, map[string]any{
		"subject":     "New Subject",
		"description": "New Description",
	})
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Subject != "New Subject" {
		t.Errorf("expected subject 'New Subject', got '%s'", task.Subject)
	}
	if task.Description != "New Description" {
		t.Errorf("expected description 'New Description', got '%s'", task.Description)
	}
}

func TestWorkTaskStore_UpdateTask_Owner(t *testing.T) {
	store := NewWorkTaskStore()

	id := store.CreateTask("Fix bug", "Fix it", "", nil)

	err := store.UpdateTask(id, map[string]any{"owner": "agent-1"})
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Owner != "agent-1" {
		t.Errorf("expected owner 'agent-1', got '%s'", task.Owner)
	}
}

func TestWorkTaskStore_UpdateTask_Blocks(t *testing.T) {
	store := NewWorkTaskStore()

	id1 := store.CreateTask("Task 1", "Desc 1", "", nil)
	id2 := store.CreateTask("Task 2", "Desc 2", "", nil)

	// Task 1 blocks task 2
	err := store.UpdateTask(id1, map[string]any{"addBlocks": []any{id2}})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask(id1)
	task2 := store.GetTask(id2)

	if len(task1.Blocks) != 1 || task1.Blocks[0] != id2 {
		t.Errorf("expected task1 to block task2, got blocks: %v", task1.Blocks)
	}
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != id1 {
		t.Errorf("expected task2 to be blocked by task1, got blockedBy: %v", task2.BlockedBy)
	}
}

func TestWorkTaskStore_UpdateTask_DuplicateBlocks(t *testing.T) {
	store := NewWorkTaskStore()

	id1 := store.CreateTask("Task 1", "Desc 1", "", nil)
	id2 := store.CreateTask("Task 2", "Desc 2", "", nil)

	// Add same block twice
	store.UpdateTask(id1, map[string]any{"addBlocks": []any{id2}})
	store.UpdateTask(id1, map[string]any{"addBlocks": []any{id2}})

	task1 := store.GetTask(id1)
	if len(task1.Blocks) != 1 {
		t.Errorf("expected 1 block (no duplicates), got %d", len(task1.Blocks))
	}
}

func TestWorkTaskStore_UpdateTask_DeletedCleansReferences(t *testing.T) {
	store := NewWorkTaskStore()

	id1 := store.CreateTask("Task 1", "Desc 1", "", nil)
	id2 := store.CreateTask("Task 2", "Desc 2", "", nil)

	// Task 1 blocks task 2
	store.UpdateTask(id1, map[string]any{"addBlocks": []any{id2}})

	// Delete task 1
	store.UpdateTask(id1, map[string]any{"status": string(WorkTaskDeleted)})

	// Task 2 should have its BlockedBy cleaned
	task2 := store.GetTask(id2)
	if len(task2.BlockedBy) != 0 {
		t.Errorf("expected task2.BlockedBy to be empty after deletion, got %v", task2.BlockedBy)
	}
}

func TestWorkTaskStore_UpdateTask_NotFound(t *testing.T) {
	store := NewWorkTaskStore()

	err := store.UpdateTask("999", map[string]any{"subject": "New"})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestWorkTaskStore_UpdateTask_Metadata(t *testing.T) {
	store := NewWorkTaskStore()

	id := store.CreateTask("Fix bug", "Fix it", "", map[string]any{"priority": "high"})

	// Add metadata key
	err := store.UpdateTask(id, map[string]any{
		"metadata": map[string]any{"assignee": "john"},
	})
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Metadata["priority"] != "high" {
		t.Errorf("expected existing metadata preserved, got %v", task.Metadata)
	}
	if task.Metadata["assignee"] != "john" {
		t.Errorf("expected new metadata added, got %v", task.Metadata)
	}

	// Delete metadata key
	err = store.UpdateTask(id, map[string]any{
		"metadata": map[string]any{"priority": nil},
	})
	if err != nil {
		t.Fatal(err)
	}

	task = store.GetTask(id)
	if _, exists := task.Metadata["priority"]; exists {
		t.Error("expected metadata key 'priority' to be deleted")
	}
}

func TestWorkTaskStore_UpdateTask_MultipleBlocksAndBlockedBy(t *testing.T) {
	store := NewWorkTaskStore()

	id1 := store.CreateTask("Task 1", "Desc 1", "", nil)
	id2 := store.CreateTask("Task 2", "Desc 2", "", nil)
	id3 := store.CreateTask("Task 3", "Desc 3", "", nil)

	// Task 1 blocks both task 2 and task 3
	err := store.UpdateTask(id1, map[string]any{"addBlocks": []any{id2, id3}})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask(id1)
	if len(task1.Blocks) != 2 {
		t.Errorf("expected task1 to block 2 tasks, got %d", len(task1.Blocks))
	}

	task2 := store.GetTask(id2)
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != id1 {
		t.Errorf("expected task2 blocked by task1, got %v", task2.BlockedBy)
	}

	task3 := store.GetTask(id3)
	if len(task3.BlockedBy) != 1 || task3.BlockedBy[0] != id1 {
		t.Errorf("expected task3 blocked by task1, got %v", task3.BlockedBy)
	}
}
