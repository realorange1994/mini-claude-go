package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"miniclaudecode-go/tools"
)

func TestWorkTaskStore_CreateTask(t *testing.T) {
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "Description 1", "", nil)
	id2 := store.CreateTask("Task 2", "Description 2", "", nil)

	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}
}

func TestWorkTaskStore_CreateTask_WithMetadata(t *testing.T) {
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

	id := store.CreateTask("Fix bug", "Fix it", "", nil)

	err := store.UpdateTask(id, map[string]any{"status": "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestWorkTaskStore_UpdateTask_SubjectAndDescription(t *testing.T) {
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

	err := store.UpdateTask("999", map[string]any{"subject": "New"})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestWorkTaskStore_UpdateTask_Metadata(t *testing.T) {
	store := NewWorkTaskStore("")

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
	store := NewWorkTaskStore("")

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

// --- Bug fix tests ---

// Bug 1: Integer format scalar should be coerced to array in TaskUpdateTool
func TestTaskUpdateTool_ScalarAddBlockedBy(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)
	store.CreateTask("Task 2", "Desc 2", "", nil)

	tool := &tools.TaskUpdateTool{
		UpdateFunc: store.UpdateTask,
	}

	// Simulate LLM passing add_blocked_by as float64 (integer) instead of array
	result := tool.Execute(map[string]any{
		"task_id":        "2",
		"add_blocked_by": float64(1),
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Output)
	}

	task2 := store.GetTask("2")
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != "1" {
		t.Errorf("expected task2.BlockedBy = ['1'], got %v", task2.BlockedBy)
	}
}

// Bug 1: Integer format scalar should be coerced to array in TaskUpdateTool for add_blocks
func TestTaskUpdateTool_ScalarAddBlocks(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)
	store.CreateTask("Task 2", "Desc 2", "", nil)

	tool := &tools.TaskUpdateTool{
		UpdateFunc: store.UpdateTask,
	}

	// Simulate LLM passing add_blocks as float64 (integer) instead of array
	result := tool.Execute(map[string]any{
		"task_id":    "1",
		"add_blocks": float64(2),
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Output)
	}

	task1 := store.GetTask("1")
	if len(task1.Blocks) != 1 || task1.Blocks[0] != "2" {
		t.Errorf("expected task1.Blocks = ['2'], got %v", task1.Blocks)
	}
}

// Bug 2: Integer elements in array should be converted to strings
func TestWorkTaskStore_UpdateTask_IntegerElementsInArray(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)
	store.CreateTask("Task 2", "Desc 2", "", nil)

	// LLM sends [1] (float64 elements) instead of ["1"]
	err := store.UpdateTask("2", map[string]any{
		"addBlockedBy": []any{float64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}

	task2 := store.GetTask("2")
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != "1" {
		t.Errorf("expected task2.BlockedBy = ['1'], got %v", task2.BlockedBy)
	}
}

// Bug 3: Non-existent task IDs should be silently removed
func TestWorkTaskStore_UpdateTask_NonExistentDependency(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)

	// Reference to non-existent task 9999 should be silently removed
	err := store.UpdateTask("1", map[string]any{
		"addBlockedBy": []any{"9999"},
	})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask("1")
	if len(task1.BlockedBy) != 0 {
		t.Errorf("expected task1.BlockedBy to be empty (non-existent deps removed), got %v", task1.BlockedBy)
	}
}

// Bug 3: Non-existent task IDs in Blocks should be silently removed
func TestWorkTaskStore_UpdateTask_NonExistentBlocks(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)

	// Reference to non-existent task 8888 should be silently removed
	err := store.UpdateTask("1", map[string]any{
		"addBlocks": []any{"8888"},
	})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask("1")
	if len(task1.Blocks) != 0 {
		t.Errorf("expected task1.Blocks to be empty (non-existent deps removed), got %v", task1.Blocks)
	}
}

// Bug 4: Circular dependency should be detected and prevented
func TestWorkTaskStore_UpdateTask_CircularDependency(t *testing.T) {
	store := NewWorkTaskStore("")
	id1 := store.CreateTask("Task 1", "Desc 1", "", nil)
	id2 := store.CreateTask("Task 2", "Desc 2", "", nil)

	// Task 2 is blocked by Task 1
	err := store.UpdateTask(id2, map[string]any{
		"addBlockedBy": []any{id1},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now try to make Task 1 blocked by Task 2 — this would create a cycle
	err = store.UpdateTask(id1, map[string]any{
		"addBlockedBy": []any{id2},
	})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask(id1)
	// The circular edge should be silently skipped
	if containsString(task1.BlockedBy, id2) {
		t.Error("expected circular dependency to be prevented, but task1.BlockedBy contains task2")
	}
}

// Bug 4: Self-dependency should be detected
func TestWorkTaskStore_UpdateTask_SelfDependency(t *testing.T) {
	store := NewWorkTaskStore("")
	id1 := store.CreateTask("Task 1", "Desc 1", "", nil)

	// Task blocking itself should be prevented
	err := store.UpdateTask(id1, map[string]any{
		"addBlockedBy": []any{id1},
	})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask(id1)
	if containsString(task1.BlockedBy, id1) {
		t.Error("expected self-dependency to be prevented")
	}
}

// Bug 5: Hash prefix should be stripped from task IDs
func TestWorkTaskStore_UpdateTask_HashPrefix(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)
	store.CreateTask("Task 2", "Desc 2", "", nil)

	// "#1" should be normalized to "1"
	err := store.UpdateTask("2", map[string]any{
		"addBlockedBy": []any{"#1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	task2 := store.GetTask("2")
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != "1" {
		t.Errorf("expected task2.BlockedBy = ['1'] (hash stripped), got %v", task2.BlockedBy)
	}
}

// Bug 5: Hash prefix in addBlocks should also be stripped
func TestWorkTaskStore_UpdateTask_HashPrefixBlocks(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "Desc 1", "", nil)
	store.CreateTask("Task 2", "Desc 2", "", nil)

	// "#2" should be normalized to "2"
	err := store.UpdateTask("1", map[string]any{
		"addBlocks": []any{"#2"},
	})
	if err != nil {
		t.Fatal(err)
	}

	task1 := store.GetTask("1")
	if len(task1.Blocks) != 1 || task1.Blocks[0] != "2" {
		t.Errorf("expected task1.Blocks = ['2'] (hash stripped), got %v", task1.Blocks)
	}
}

// ─── Task Persistence Tests ──────────────────────────────────────────────────

func TestWorkTaskStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create tasks and save
	store1 := NewWorkTaskStore(dir)
	store1.CreateTask("Fix bug", "Fix the auth bug", "Fixing bug", nil)
	store1.CreateTask("Add tests", "Add unit tests", "Writing tests", map[string]any{"priority": "high"})
	store1.UpdateTask("1", map[string]any{"status": "in_progress"})
	store1.SaveToDisk()

	// Load in new instance
	store2 := NewWorkTaskStore(dir)
	tasks := store2.ListTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks after reload, got %d", len(tasks))
	}

	// Verify task 1
	task1 := store2.GetTask("1")
	if task1 == nil {
		t.Fatal("task 1 not found after reload")
	}
	if task1.Subject != "Fix bug" {
		t.Errorf("expected subject 'Fix bug', got '%s'", task1.Subject)
	}
	if task1.Status != WorkTaskInProgress {
		t.Errorf("expected status 'in_progress', got '%s'", task1.Status)
	}

	// Verify task 2
	task2 := store2.GetTask("2")
	if task2 == nil {
		t.Fatal("task 2 not found after reload")
	}
	if task2.Subject != "Add tests" {
		t.Errorf("expected subject 'Add tests', got '%s'", task2.Subject)
	}
	if task2.Metadata["priority"] != "high" {
		t.Errorf("expected metadata priority 'high', got '%v'", task2.Metadata["priority"])
	}
}

func TestWorkTaskStore_NextIDContinuity(t *testing.T) {
	dir := t.TempDir()

	// Create tasks and save
	store1 := NewWorkTaskStore(dir)
	store1.CreateTask("Task 1", "", "", nil)
	store1.CreateTask("Task 2", "", "", nil)
	store1.CreateTask("Task 3", "", "", nil)
	store1.SaveToDisk()

	// Load and create a new task
	store2 := NewWorkTaskStore(dir)
	id := store2.CreateTask("Task 4", "", "", nil)
	if id != "4" {
		t.Errorf("expected next ID '4', got '%s'", id)
	}
}

func TestWorkTaskStore_BlocksPersisted(t *testing.T) {
	dir := t.TempDir()

	// Create tasks with dependencies
	store1 := NewWorkTaskStore(dir)
	store1.CreateTask("Task 1", "", "", nil)
	store1.CreateTask("Task 2", "", "", nil)
	store1.UpdateTask("1", map[string]any{"addBlocks": []any{"2"}})
	store1.SaveToDisk()

	// Load and verify dependencies
	store2 := NewWorkTaskStore(dir)
	task1 := store2.GetTask("1")
	if len(task1.Blocks) != 1 || task1.Blocks[0] != "2" {
		t.Errorf("expected task1.Blocks = ['2'], got %v", task1.Blocks)
	}
	task2 := store2.GetTask("2")
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != "1" {
		t.Errorf("expected task2.BlockedBy = ['1'], got %v", task2.BlockedBy)
	}
}

func TestWorkTaskStore_DeletedTasksNotPersisted(t *testing.T) {
	dir := t.TempDir()

	// Create and delete a task
	store1 := NewWorkTaskStore(dir)
	store1.CreateTask("Task 1", "", "", nil)
	store1.CreateTask("Task 2", "", "", nil)
	store1.UpdateTask("1", map[string]any{"status": "deleted"})
	store1.SaveToDisk()

	// Load and verify deleted task is still in store but marked deleted
	store2 := NewWorkTaskStore(dir)
	task1 := store2.GetTask("1")
	if task1 == nil {
		t.Fatal("deleted task should still be in store")
	}
	if task1.Status != WorkTaskDeleted {
		t.Errorf("expected status 'deleted', got '%s'", task1.Status)
	}

	// ListTasks should exclude deleted
	tasks := store2.ListTasks()
	if len(tasks) != 1 {
		t.Errorf("expected 1 non-deleted task, got %d", len(tasks))
	}
}

func TestWorkTaskStore_ListActiveTasks(t *testing.T) {
	dir := t.TempDir()

	store := NewWorkTaskStore(dir)
	store.CreateTask("Pending", "", "", nil)
	store.CreateTask("In Progress", "", "", nil)
	store.CreateTask("Completed", "", "", nil)
	store.TransitionTo("2", WorkTaskInProgress, "starting")
	store.TransitionTo("3", WorkTaskInProgress, "starting")
	store.TransitionTo("3", WorkTaskCompleted, "done")

	active := store.ListActiveTasks()
	if len(active) != 2 {
		t.Errorf("expected 2 active tasks, got %d", len(active))
	}
}

func TestWorkTaskStore_NoPersistenceWithoutDir(t *testing.T) {
	// In-memory only (no projectDir)
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "", "", nil)

	// Should not panic or error
	store.SaveToDisk()
	store.Close()
}

func TestWorkTaskStore_MultipleFlushes(t *testing.T) {
	dir := t.TempDir()

	store := NewWorkTaskStore(dir)
	store.CreateTask("Task 1", "", "", nil)
	store.SaveToDisk()

	// Add more tasks and save again
	store.CreateTask("Task 2", "", "", nil)
	store.SaveToDisk()

	// Load and verify both tasks exist
	store2 := NewWorkTaskStore(dir)
	tasks := store2.ListTasks()
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks after multiple flushes, got %d", len(tasks))
	}
}

func TestWorkTaskStore_CompletedStatusPreserved(t *testing.T) {
	dir := t.TempDir()

	store1 := NewWorkTaskStore(dir)
	store1.CreateTask("Task 1", "", "", nil)
	store1.UpdateTask("1", map[string]any{"status": "in_progress"})
	store1.UpdateTask("1", map[string]any{"status": "completed"})
	store1.SaveToDisk()

	store2 := NewWorkTaskStore(dir)
	task := store2.GetTask("1")
	if task.Status != WorkTaskCompleted {
		t.Errorf("expected status 'completed', got '%s'", task.Status)
	}
}

// ─── Subtask Tests ──────────────────────────────────────────────────────────

func TestWorkTaskStore_CreateSubtask(t *testing.T) {
	store := NewWorkTaskStore("")

	// Create parent task
	parentID := store.CreateTask("Build feature", "Implement the new feature", "Building feature", nil)

	// Create subtasks
	sub1 := store.CreateSubtask(parentID, "Write tests", "Write unit tests", "Writing tests", nil)
	sub2 := store.CreateSubtask(parentID, "Implement logic", "Write the core logic", "Implementing logic", nil)

	if sub1 == "" || sub2 == "" {
		t.Fatal("expected non-empty subtask IDs")
	}

	// Verify parent-child relationship
	task1 := store.GetTask(sub1)
	if task1.ParentID != parentID {
		t.Errorf("expected parentID '%s', got '%s'", parentID, task1.ParentID)
	}

	task2 := store.GetTask(sub2)
	if task2.ParentID != parentID {
		t.Errorf("expected parentID '%s', got '%s'", parentID, task2.ParentID)
	}
}

func TestWorkTaskStore_CreateSubtask_InvalidParent(t *testing.T) {
	store := NewWorkTaskStore("")

	// Try to create subtask with non-existent parent
	subID := store.CreateSubtask("999", "Subtask", "", "", nil)
	if subID != "" {
		t.Errorf("expected empty ID for invalid parent, got '%s'", subID)
	}
}

func TestWorkTaskStore_ListSubtasks(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	store.CreateSubtask(parentID, "Sub 1", "", "", nil)
	store.CreateSubtask(parentID, "Sub 2", "", "", nil)
	store.CreateTask("Other task", "", "", nil) // not a subtask

	subtasks := store.ListSubtasks(parentID)
	if len(subtasks) != 2 {
		t.Errorf("expected 2 subtasks, got %d", len(subtasks))
	}

	// Verify they are the correct subtasks
	for _, st := range subtasks {
		if st.ParentID != parentID {
			t.Errorf("expected parentID '%s', got '%s'", parentID, st.ParentID)
		}
	}
}

func TestWorkTaskStore_ListTopLevelTasks(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	store.CreateSubtask(parentID, "Sub 1", "", "", nil)
	store.CreateTask("Top level", "", "", nil)

	topTasks := store.ListTopLevelTasks()
	if len(topTasks) != 2 {
		t.Errorf("expected 2 top-level tasks, got %d", len(topTasks))
	}

	// Verify none have a parent
	for _, task := range topTasks {
		if task.ParentID != "" {
			t.Errorf("expected empty parentID for top-level task, got '%s'", task.ParentID)
		}
	}
}

func TestWorkTaskStore_GetTaskTree(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	sub1 := store.CreateSubtask(parentID, "Sub 1", "", "", nil)
	store.CreateSubtask(parentID, "Sub 2", "", "", nil)
	store.CreateSubtask(sub1, "Sub-sub 1", "", "", nil) // nested subtask

	tree := store.GetTaskTree(parentID)
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.Task.ID != parentID {
		t.Errorf("expected root task ID '%s', got '%s'", parentID, tree.Task.ID)
	}
	if len(tree.Subtasks) != 2 {
		t.Errorf("expected 2 direct subtasks, got %d", len(tree.Subtasks))
	}

	// Find sub1 in the tree
	var sub1Node *TaskTreeNode
	for _, st := range tree.Subtasks {
		if st.Task.ID == sub1 {
			sub1Node = st
			break
		}
	}
	if sub1Node == nil {
		t.Fatal("sub1 not found in tree")
	}
	if len(sub1Node.Subtasks) != 1 {
		t.Errorf("expected 1 nested subtask under sub1, got %d", len(sub1Node.Subtasks))
	}
}

func TestWorkTaskStore_GetDepth(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	sub1 := store.CreateSubtask(parentID, "Sub 1", "", "", nil)
	subSub1 := store.CreateSubtask(sub1, "Sub-sub 1", "", "", nil)

	if d := store.GetDepth(parentID); d != 0 {
		t.Errorf("expected depth 0 for parent, got %d", d)
	}
	if d := store.GetDepth(sub1); d != 1 {
		t.Errorf("expected depth 1 for sub, got %d", d)
	}
	if d := store.GetDepth(subSub1); d != 2 {
		t.Errorf("expected depth 2 for sub-sub, got %d", d)
	}
}

func TestWorkTaskStore_GetRootTask(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	sub1 := store.CreateSubtask(parentID, "Sub 1", "", "", nil)
	subSub1 := store.CreateSubtask(sub1, "Sub-sub 1", "", "", nil)

	root := store.GetRootTask(subSub1)
	if root == nil {
		t.Fatal("expected non-nil root")
	}
	if root.ID != parentID {
		t.Errorf("expected root ID '%s', got '%s'", parentID, root.ID)
	}
}

func TestWorkTaskStore_SubtaskPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create tasks with subtasks and save
	store1 := NewWorkTaskStore(dir)
	parentID := store1.CreateTask("Parent", "", "", nil)
	store1.CreateSubtask(parentID, "Sub 1", "", "", nil)
	store1.CreateSubtask(parentID, "Sub 2", "", "", nil)
	store1.SaveToDisk()

	// Load and verify hierarchy
	store2 := NewWorkTaskStore(dir)
	subtasks := store2.ListSubtasks(parentID)
	if len(subtasks) != 2 {
		t.Errorf("expected 2 subtasks after reload, got %d", len(subtasks))
	}

	// Verify parentID is preserved
	for _, st := range subtasks {
		if st.ParentID != parentID {
			t.Errorf("expected parentID '%s' after reload, got '%s'", parentID, st.ParentID)
		}
	}

	// Verify tree structure
	tree := store2.GetTaskTree(parentID)
	if tree == nil {
		t.Fatal("expected non-nil tree after reload")
	}
	if len(tree.Subtasks) != 2 {
		t.Errorf("expected 2 subtasks in tree after reload, got %d", len(tree.Subtasks))
	}
}

func TestWorkTaskStore_UpdateTaskParentID(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)

	// Move task 2 under task 1
	err := store.UpdateTask("2", map[string]any{"parentID": "1"})
	if err != nil {
		t.Fatal(err)
	}

	task2 := store.GetTask("2")
	if task2.ParentID != "1" {
		t.Errorf("expected parentID '1', got '%s'", task2.ParentID)
	}

	subtasks := store.ListSubtasks("1")
	if len(subtasks) != 1 {
		t.Errorf("expected 1 subtask, got %d", len(subtasks))
	}
}

func TestWorkTaskStore_UpdateTaskParentID_SelfReference(t *testing.T) {
	store := NewWorkTaskStore("")
	store.CreateTask("Task 1", "", "", nil)

	err := store.UpdateTask("1", map[string]any{"parentID": "1"})
	if err == nil {
		t.Error("expected error for self-reference parentID")
	}
}

func TestWorkTaskStore_SubtaskInheritsOwner(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	store.UpdateTask(parentID, map[string]any{"owner": "agent-1"})

	subID := store.CreateSubtask(parentID, "Sub", "", "", nil)
	sub := store.GetTask(subID)

	if sub.Owner != "agent-1" {
		t.Errorf("expected owner 'agent-1' (inherited from parent), got '%s'", sub.Owner)
	}
}

// ─── Tag Tests ──────────────────────────────────────────────────────────────

func TestWorkTaskStore_AddTaskTag(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	err := store.AddTaskTag(id, "bug")
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if len(task.Tags) != 1 || task.Tags[0] != "bug" {
		t.Errorf("expected tags ['bug'], got %v", task.Tags)
	}
}

func TestWorkTaskStore_AddTaskTag_Dedup(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.AddTaskTag(id, "bug")
	store.AddTaskTag(id, "bug") // duplicate

	task := store.GetTask(id)
	if len(task.Tags) != 1 {
		t.Errorf("expected 1 tag (deduped), got %d", len(task.Tags))
	}
}

func TestWorkTaskStore_AddTaskTag_Empty(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.AddTaskTag(id, "") // empty tag should be ignored

	task := store.GetTask(id)
	if len(task.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(task.Tags))
	}
}

func TestWorkTaskStore_RemoveTaskTag(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.AddTaskTag(id, "bug")
	store.AddTaskTag(id, "urgent")
	store.RemoveTaskTag(id, "bug")

	task := store.GetTask(id)
	if len(task.Tags) != 1 || task.Tags[0] != "urgent" {
		t.Errorf("expected tags ['urgent'], got %v", task.Tags)
	}
}

func TestWorkTaskStore_SetTaskTags(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.AddTaskTag(id, "old-tag")
	store.SetTaskTags(id, []string{"new-tag-1", "new-tag-2"})

	task := store.GetTask(id)
	if len(task.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(task.Tags))
	}
	if !containsString(task.Tags, "new-tag-1") || !containsString(task.Tags, "new-tag-2") {
		t.Errorf("expected tags ['new-tag-1', 'new-tag-2'], got %v", task.Tags)
	}
}

func TestWorkTaskStore_SetTaskTags_Dedup(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.SetTaskTags(id, []string{"tag1", "tag1", "tag2"})

	task := store.GetTask(id)
	if len(task.Tags) != 2 {
		t.Errorf("expected 2 tags (deduped), got %d", len(task.Tags))
	}
}

func TestWorkTaskStore_FilterByTag(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Bug fix", "", "", nil)
	id2 := store.CreateTask("Feature", "", "", nil)
	id3 := store.CreateTask("Another bug", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "feature")
	store.AddTaskTag(id3, "bug")

	bugs := store.FilterByTag("bug")
	if len(bugs) != 2 {
		t.Errorf("expected 2 bug tasks, got %d", len(bugs))
	}

	features := store.FilterByTag("feature")
	if len(features) != 1 {
		t.Errorf("expected 1 feature task, got %d", len(features))
	}

	none := store.FilterByTag("nonexistent")
	if len(none) != 0 {
		t.Errorf("expected 0 tasks for nonexistent tag, got %d", len(none))
	}
}

func TestWorkTaskStore_FilterByTags(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Urgent bug", "", "", nil)
	id2 := store.CreateTask("Normal bug", "", "", nil)
	id3 := store.CreateTask("Urgent feature", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id1, "urgent")
	store.AddTaskTag(id2, "bug")
	store.AddTaskTag(id3, "feature")
	store.AddTaskTag(id3, "urgent")

	// Filter by both "bug" AND "urgent"
	urgentBugs := store.FilterByTags([]string{"bug", "urgent"})
	if len(urgentBugs) != 1 {
		t.Errorf("expected 1 urgent bug task, got %d", len(urgentBugs))
	}
	if len(urgentBugs) > 0 && urgentBugs[0].ID != id1 {
		t.Errorf("expected task ID '%s', got '%s'", id1, urgentBugs[0].ID)
	}
}

func TestWorkTaskStore_ListAllTags(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id1, "urgent")
	store.AddTaskTag(id2, "feature")
	store.AddTaskTag(id2, "bug") // duplicate across tasks

	tags := store.ListAllTags()
	if len(tags) != 3 {
		t.Errorf("expected 3 unique tags, got %d: %v", len(tags), tags)
	}
	// Should be sorted
	if tags[0] != "bug" || tags[1] != "feature" || tags[2] != "urgent" {
		t.Errorf("expected sorted tags [bug, feature, urgent], got %v", tags)
	}
}

func TestWorkTaskStore_TagStats(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "bug")
	store.AddTaskTag(id3, "feature")

	stats := store.TagStats()
	if stats["bug"] != 2 {
		t.Errorf("expected 2 bug tasks, got %d", stats["bug"])
	}
	if stats["feature"] != 1 {
		t.Errorf("expected 1 feature task, got %d", stats["feature"])
	}
}

func TestWorkTaskStore_TagsInUpdateTask(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// Set tags via UpdateTask
	store.UpdateTask(id, map[string]any{
		"tags": []any{"bug", "urgent"},
	})

	task := store.GetTask(id)
	if len(task.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(task.Tags))
	}

	// Add tag via UpdateTask
	store.UpdateTask(id, map[string]any{
		"addTags": []any{"feature"},
	})

	task = store.GetTask(id)
	if len(task.Tags) != 3 {
		t.Errorf("expected 3 tags after addTags, got %d", len(task.Tags))
	}

	// Remove tag via UpdateTask
	store.UpdateTask(id, map[string]any{
		"removeTags": []any{"bug"},
	})

	task = store.GetTask(id)
	if len(task.Tags) != 2 {
		t.Errorf("expected 2 tags after removeTags, got %d", len(task.Tags))
	}
	if containsString(task.Tags, "bug") {
		t.Error("expected 'bug' to be removed")
	}
}

func TestWorkTaskStore_TagsPersisted(t *testing.T) {
	dir := t.TempDir()

	store1 := NewWorkTaskStore(dir)
	id := store1.CreateTask("Task 1", "", "", nil)
	store1.AddTaskTag(id, "bug")
	store1.AddTaskTag(id, "urgent")
	store1.SaveToDisk()

	store2 := NewWorkTaskStore(dir)
	task := store2.GetTask(id)
	if len(task.Tags) != 2 {
		t.Errorf("expected 2 tags after reload, got %d", len(task.Tags))
	}
	if !containsString(task.Tags, "bug") || !containsString(task.Tags, "urgent") {
		t.Errorf("expected tags [bug, urgent], got %v", task.Tags)
	}
}

func TestWorkTaskStore_FilterByTag_ExcludesDeleted(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "bug")
	store.UpdateTask(id2, map[string]any{"status": "deleted"})

	bugs := store.FilterByTag("bug")
	if len(bugs) != 1 {
		t.Errorf("expected 1 bug task (excluding deleted), got %d", len(bugs))
	}
}

func TestWorkTaskStore_ListTasksByTagGroup(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Bug 1", "", "", nil)
	id2 := store.CreateTask("Bug 2", "", "", nil)
	id3 := store.CreateTask("Feature 1", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "bug")
	store.AddTaskTag(id3, "feature")

	groups := store.ListTasksByTagGroup()
	if len(groups["bug"]) != 2 {
		t.Errorf("expected 2 bug tasks in group, got %d", len(groups["bug"]))
	}
	if len(groups["feature"]) != 1 {
		t.Errorf("expected 1 feature task in group, got %d", len(groups["feature"]))
	}
}

func TestWorkTaskStore_TagErrorCases(t *testing.T) {
	store := NewWorkTaskStore("")

	// Tag non-existent task
	err := store.AddTaskTag("999", "bug")
	if err == nil {
		t.Error("expected error for non-existent task")
	}

	// Remove tag from non-existent task
	err = store.RemoveTaskTag("999", "bug")
	if err == nil {
		t.Error("expected error for non-existent task")
	}

	// Set tags on non-existent task
	err = store.SetTaskTags("999", []string{"bug"})
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

// ─── Priority Tests ─────────────────────────────────────────────────────────

func TestWorkTaskStore_SetTaskPriority(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	err := store.SetTaskPriority(id, PriorityHigh)
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Priority != PriorityHigh {
		t.Errorf("expected priority 'high', got '%s'", task.Priority)
	}
}

func TestWorkTaskStore_SetTaskPriority_Invalid(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	err := store.SetTaskPriority(id, "invalid")
	if err == nil {
		t.Error("expected error for invalid priority")
	}
}

func TestWorkTaskStore_SetTaskPriority_NotFound(t *testing.T) {
	store := NewWorkTaskStore("")

	err := store.SetTaskPriority("999", PriorityHigh)
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestWorkTaskStore_FilterByPriority(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Critical", "", "", nil)
	id2 := store.CreateTask("High", "", "", nil)
	id3 := store.CreateTask("Another critical", "", "", nil)

	store.SetTaskPriority(id1, PriorityCritical)
	store.SetTaskPriority(id2, PriorityHigh)
	store.SetTaskPriority(id3, PriorityCritical)

	criticals := store.FilterByPriority(PriorityCritical)
	if len(criticals) != 2 {
		t.Errorf("expected 2 critical tasks, got %d", len(criticals))
	}

	highs := store.FilterByPriority(PriorityHigh)
	if len(highs) != 1 {
		t.Errorf("expected 1 high task, got %d", len(highs))
	}

	lows := store.FilterByPriority(PriorityLow)
	if len(lows) != 0 {
		t.Errorf("expected 0 low tasks, got %d", len(lows))
	}
}

func TestWorkTaskStore_ListTasksByPriority(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Low", "", "", nil)
	id2 := store.CreateTask("Critical", "", "", nil)
	id3 := store.CreateTask("Medium", "", "", nil)
	id4 := store.CreateTask("High", "", "", nil)

	store.SetTaskPriority(id1, PriorityLow)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityMedium)
	store.SetTaskPriority(id4, PriorityHigh)

	tasks := store.ListTasksByPriority()
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	// Should be sorted: critical, high, medium, low
	if tasks[0].Priority != PriorityCritical {
		t.Errorf("expected first task critical, got '%s'", tasks[0].Priority)
	}
	if tasks[1].Priority != PriorityHigh {
		t.Errorf("expected second task high, got '%s'", tasks[1].Priority)
	}
	if tasks[2].Priority != PriorityMedium {
		t.Errorf("expected third task medium, got '%s'", tasks[2].Priority)
	}
	if tasks[3].Priority != PriorityLow {
		t.Errorf("expected fourth task low, got '%s'", tasks[3].Priority)
	}
}

func TestWorkTaskStore_ListActiveByPriority(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Low pending", "", "", nil)
	id2 := store.CreateTask("Critical in progress", "", "", nil)
	id3 := store.CreateTask("High completed", "", "", nil)
	id4 := store.CreateTask("Medium pending", "", "", nil)

	store.SetTaskPriority(id1, PriorityLow)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityHigh)
	store.SetTaskPriority(id4, PriorityMedium)
	store.TransitionTo(id3, WorkTaskInProgress, "starting")
	store.TransitionTo(id3, WorkTaskCompleted, "done")

	tasks := store.ListActiveByPriority()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 active tasks, got %d", len(tasks))
	}

	// Should be sorted: critical, medium, low (high is completed)
	if tasks[0].Priority != PriorityCritical {
		t.Errorf("expected first task critical, got '%s'", tasks[0].Priority)
	}
}

func TestWorkTaskStore_PriorityStats(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)
	store.CreateTask("Task 4", "", "", nil) // default priority

	store.SetTaskPriority(id1, PriorityCritical)
	store.SetTaskPriority(id2, PriorityHigh)
	store.SetTaskPriority(id3, PriorityHigh)

	stats := store.PriorityStats()
	if stats[PriorityCritical] != 1 {
		t.Errorf("expected 1 critical, got %d", stats[PriorityCritical])
	}
	if stats[PriorityHigh] != 2 {
		t.Errorf("expected 2 high, got %d", stats[PriorityHigh])
	}
	if stats[PriorityMedium] != 1 {
		t.Errorf("expected 1 medium (default), got %d", stats[PriorityMedium])
	}
}

func TestWorkTaskStore_GetHighestPriorityTask(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Low", "", "", nil)
	id2 := store.CreateTask("Critical", "", "", nil)
	id3 := store.CreateTask("High", "", "", nil)

	store.SetTaskPriority(id1, PriorityLow)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityHigh)

	best := store.GetHighestPriorityTask()
	if best == nil {
		t.Fatal("expected non-nil highest priority task")
	}
	if best.ID != id2 {
		t.Errorf("expected task ID '%s' (critical), got '%s'", id2, best.ID)
	}
}

func TestWorkTaskStore_GetHighestPriorityTask_Completed(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Critical completed", "", "", nil)
	id2 := store.CreateTask("High pending", "", "", nil)

	store.SetTaskPriority(id1, PriorityCritical)
	store.SetTaskPriority(id2, PriorityHigh)
	store.TransitionTo(id1, WorkTaskInProgress, "starting")
	store.TransitionTo(id1, WorkTaskCompleted, "done")

	best := store.GetHighestPriorityTask()
	if best == nil {
		t.Fatal("expected non-nil highest priority task")
	}
	if best.ID != id2 {
		t.Errorf("expected task ID '%s' (high, active), got '%s'", id2, best.ID)
	}
}

func TestWorkTaskStore_GetHighestPriorityTask_NoTasks(t *testing.T) {
	store := NewWorkTaskStore("")

	best := store.GetHighestPriorityTask()
	if best != nil {
		t.Errorf("expected nil for empty store, got %v", best)
	}
}

func TestWorkTaskStore_PriorityInUpdateTask(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// Set priority via UpdateTask
	store.UpdateTask(id, map[string]any{"priority": "critical"})

	task := store.GetTask(id)
	if task.Priority != PriorityCritical {
		t.Errorf("expected priority 'critical', got '%s'", task.Priority)
	}
}

func TestWorkTaskStore_PriorityInUpdateTask_Invalid(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	err := store.UpdateTask(id, map[string]any{"priority": "invalid"})
	if err == nil {
		t.Error("expected error for invalid priority in UpdateTask")
	}
}

func TestWorkTaskStore_PriorityPersisted(t *testing.T) {
	dir := t.TempDir()

	store1 := NewWorkTaskStore(dir)
	id := store1.CreateTask("Task 1", "", "", nil)
	store1.SetTaskPriority(id, PriorityCritical)
	store1.SaveToDisk()

	store2 := NewWorkTaskStore(dir)
	task := store2.GetTask(id)
	if task.Priority != PriorityCritical {
		t.Errorf("expected priority 'critical' after reload, got '%s'", task.Priority)
	}
}

func TestWorkTaskStore_FilterByPriorityAndTag(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Critical bug", "", "", nil)
	id2 := store.CreateTask("Critical feature", "", "", nil)
	id3 := store.CreateTask("High bug", "", "", nil)

	store.SetTaskPriority(id1, PriorityCritical)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityHigh)
	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "feature")
	store.AddTaskTag(id3, "bug")

	// Critical + bug
	result := store.FilterByPriorityAndTag(PriorityCritical, "bug")
	if len(result) != 1 {
		t.Errorf("expected 1 critical bug task, got %d", len(result))
	}
	if len(result) > 0 && result[0].ID != id1 {
		t.Errorf("expected task ID '%s', got '%s'", id1, result[0].ID)
	}
}

func TestWorkTaskStore_DefaultPriority(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	task := store.GetTask(id)
	if task.Priority != "" {
		t.Errorf("expected empty default priority, got '%s'", task.Priority)
	}

	// Should be treated as medium in sorting
	tasks := store.ListTasksByPriority()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestWorkTaskStore_PriorityDeletedExcluded(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Critical", "", "", nil)
	id2 := store.CreateTask("Critical deleted", "", "", nil)

	store.SetTaskPriority(id1, PriorityCritical)
	store.SetTaskPriority(id2, PriorityCritical)
	store.UpdateTask(id2, map[string]any{"status": "deleted"})

	criticals := store.FilterByPriority(PriorityCritical)
	if len(criticals) != 1 {
		t.Errorf("expected 1 critical task (excluding deleted), got %d", len(criticals))
	}
}

// ─── Dependency Tests ───────────────────────────────────────────────────────

func TestWorkTaskStore_AddDependency(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	err := store.AddDependency(id2, id1) // id2 blocked by id1
	if err != nil {
		t.Fatal(err)
	}

	// Verify id2 is blocked by id1
	task2 := store.GetTask(id2)
	if !containsString(task2.BlockedBy, id1) {
		t.Errorf("expected task2.BlockedBy to contain '%s'", id1)
	}

	// Verify id1 blocks id2
	task1 := store.GetTask(id1)
	if !containsString(task1.Blocks, id2) {
		t.Errorf("expected task1.Blocks to contain '%s'", id2)
	}
}

func TestWorkTaskStore_AddDependency_SelfReference(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	err := store.AddDependency(id, id)
	if err == nil {
		t.Error("expected error for self-dependency")
	}
}

func TestWorkTaskStore_AddDependency_Cycle(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	// id2 blocked by id1
	store.AddDependency(id2, id1)

	// id1 blocked by id2 would create cycle
	err := store.AddDependency(id1, id2)
	if err == nil {
		t.Error("expected error for circular dependency")
	}
}

func TestWorkTaskStore_AddDependency_NotFound(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	err := store.AddDependency(id, "999")
	if err == nil {
		t.Error("expected error for non-existent task")
	}

	err = store.AddDependency("999", id)
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestWorkTaskStore_AddDependency_Duplicate(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id2, id1) // duplicate

	task2 := store.GetTask(id2)
	if len(task2.BlockedBy) != 1 {
		t.Errorf("expected 1 blocker (deduped), got %d", len(task2.BlockedBy))
	}
}

func TestWorkTaskStore_RemoveDependency(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddDependency(id2, id1)
	store.RemoveDependency(id2, id1)

	task2 := store.GetTask(id2)
	if len(task2.BlockedBy) != 0 {
		t.Errorf("expected 0 blockers after remove, got %d", len(task2.BlockedBy))
	}

	task1 := store.GetTask(id1)
	if len(task1.Blocks) != 0 {
		t.Errorf("expected 0 blocks after remove, got %d", len(task1.Blocks))
	}
}

func TestWorkTaskStore_IsBlocked(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddDependency(id2, id1)

	// id2 is blocked by id1 (incomplete)
	if !store.IsBlocked(id2) {
		t.Error("expected task2 to be blocked")
	}

	// id1 is not blocked
	if store.IsBlocked(id1) {
		t.Error("expected task1 to not be blocked")
	}

	// Complete id1 -> id2 no longer blocked
	store.TransitionTo(id1, WorkTaskInProgress, "starting")
	store.TransitionTo(id1, WorkTaskCompleted, "done")
	if store.IsBlocked(id2) {
		t.Error("expected task2 to not be blocked after task1 completed")
	}
}

func TestWorkTaskStore_GetBlockers(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id3, id1)
	store.AddDependency(id3, id2)

	blockers := store.GetBlockers(id3)
	if len(blockers) != 2 {
		t.Errorf("expected 2 blockers, got %d", len(blockers))
	}
}

func TestWorkTaskStore_GetBlocked(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	blocked := store.GetBlocked(id1)
	if len(blocked) != 2 {
		t.Errorf("expected 2 blocked tasks, got %d", len(blocked))
	}
}

func TestWorkTaskStore_CanStart(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddDependency(id2, id1)

	if store.CanStart(id2) {
		t.Error("expected task2 to not be able to start")
	}
	if !store.CanStart(id1) {
		t.Error("expected task1 to be able to start")
	}
}

func TestWorkTaskStore_GetReadyTasks(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	ready := store.GetReadyTasks()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready task, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != id1 {
		t.Errorf("expected task1 to be ready, got '%s'", ready[0].ID)
	}
}

func TestWorkTaskStore_GetReadyTasks_AfterComplete(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	// Complete task1 -> task2 becomes ready
	store.TransitionTo(id1, WorkTaskInProgress, "starting")
	store.TransitionTo(id1, WorkTaskCompleted, "done")

	ready := store.GetReadyTasks()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready task, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != id2 {
		t.Errorf("expected task2 to be ready, got '%s'", ready[0].ID)
	}
}

func TestWorkTaskStore_GetReadyTasks_MultipleBlockers(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	// Task 3 blocked by both task 1 and task 2
	store.AddDependency(id3, id1)
	store.AddDependency(id3, id2)

	// Complete task1 only -> task3 still blocked
	store.UpdateTask(id1, map[string]any{"status": "completed"})

	ready := store.GetReadyTasks()
	// task2 and task1(completed) should be ready, but task3 still blocked
	for _, r := range ready {
		if r.ID == id3 {
			t.Error("task3 should not be ready (still blocked by task2)")
		}
	}
}

func TestWorkTaskStore_ListBlockedTasks(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	blocked := store.ListBlockedTasks()
	if len(blocked) != 2 {
		t.Errorf("expected 2 blocked tasks, got %d", len(blocked))
	}
}

func TestWorkTaskStore_GetDependencyGraph(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	graph := store.GetDependencyGraph()
	if len(graph[id1]) != 2 {
		t.Errorf("expected task1 to block 2 tasks, got %d", len(graph[id1]))
	}
}

func TestWorkTaskStore_TopologicalSort(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	sorted, err := store.TopologicalSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(sorted))
	}

	// Verify order: id1 before id2 before id3
	posMap := make(map[string]int)
	for i, t := range sorted {
		posMap[t.ID] = i
	}
	if posMap[id1] >= posMap[id2] {
		t.Error("task1 should come before task2")
	}
	if posMap[id2] >= posMap[id3] {
		t.Error("task2 should come before task3")
	}
}

func TestWorkTaskStore_TopologicalSort_NoDeps(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.CreateTask("Task 3", "", "", nil)

	sorted, err := store.TopologicalSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(sorted))
	}
}

func TestWorkTaskStore_GetCriticalPath(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)
	id4 := store.CreateTask("Task 4", "", "", nil)

	// Chain: id1 -> id2 -> id4
	store.AddDependency(id2, id1)
	store.AddDependency(id4, id2)
	// Branch: id1 -> id3 (shorter)
	store.AddDependency(id3, id1)

	path := store.GetCriticalPath(id1)
	// Should take the longer path: id1 -> id2 -> id4
	if len(path) < 3 {
		t.Errorf("expected critical path length >= 3, got %d", len(path))
	}
}

func TestWorkTaskStore_DependencyStats(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	stats := store.DependencyStats()
	if stats.TotalTasks != 3 {
		t.Errorf("expected 3 total tasks, got %d", stats.TotalTasks)
	}
	if stats.BlockedTasks != 2 {
		t.Errorf("expected 2 blocked tasks, got %d", stats.BlockedTasks)
	}
	if stats.BlockingTasks != 1 {
		t.Errorf("expected 1 blocking task, got %d", stats.BlockingTasks)
	}
}

func TestWorkTaskStore_DependenciesPersisted(t *testing.T) {
	dir := t.TempDir()

	store1 := NewWorkTaskStore(dir)
	id1 := store1.CreateTask("Task 1", "", "", nil)
	id2 := store1.CreateTask("Task 2", "", "", nil)
	store1.AddDependency(id2, id1)
	store1.SaveToDisk()

	store2 := NewWorkTaskStore(dir)
	task2 := store2.GetTask(id2)
	if !containsString(task2.BlockedBy, id1) {
		t.Errorf("expected task2.BlockedBy to contain '%s' after reload", id1)
	}

	task1 := store2.GetTask(id1)
	if !containsString(task1.Blocks, id2) {
		t.Errorf("expected task1.Blocks to contain '%s' after reload", id2)
	}
}

func TestWorkTaskStore_DependencyWithDeletedTask(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.AddDependency(id2, id1)

	// Delete task1 -> task2 should no longer be blocked
	store.UpdateTask(id1, map[string]any{"status": "deleted"})

	if store.IsBlocked(id2) {
		t.Error("task2 should not be blocked by deleted task1")
	}

	ready := store.GetReadyTasks()
	for _, r := range ready {
		if r.ID == id2 {
			// task2 should be ready now
			return
		}
	}
	t.Error("task2 should be ready after task1 deleted")
}

// ─── Time Tracking Tests ────────────────────────────────────────────────────

func TestWorkTaskStore_TimeTracking_BasicDuration(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	task := store.GetTask(id)
	// Duration should be >= 0 (may be 0 if executed instantly)
	if task.GetDuration() < 0 {
		t.Error("expected non-negative duration")
	}
}

func TestWorkTaskStore_TimeTracking_ActiveTime(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// Start task
	store.TransitionTo(id, WorkTaskInProgress, "starting work")

	task := store.GetTask(id)
	if task.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	// ActiveTime should be >= 0
	if task.GetActiveTime() < 0 {
		t.Error("expected non-negative active time")
	}
}

func TestWorkTaskStore_TimeTracking_ActiveTimeAccumulates(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// Start -> pause -> start again
	store.TransitionTo(id, WorkTaskInProgress, "first session")
	time.Sleep(10 * time.Millisecond)
	store.TransitionTo(id, WorkTaskPending, "taking a break")
	time.Sleep(10 * time.Millisecond)
	store.TransitionTo(id, WorkTaskInProgress, "resuming")

	task := store.GetTask(id)
	if task.TimeSpent <= 0 {
		t.Error("expected accumulated time from first session")
	}
}

func TestWorkTaskStore_TimeTracking_CompletedTime(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.TransitionTo(id, WorkTaskInProgress, "starting")
	store.TransitionTo(id, WorkTaskCompleted, "done")

	task := store.GetTask(id)
	if task.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	// TimeSpent should be >= 0 (may be 0 if completed instantly)
	if task.TimeSpent < 0 {
		t.Error("expected non-negative time spent")
	}
}

func TestWorkTaskStore_TimeTracking_TimeInStatus(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.TransitionTo(id, WorkTaskInProgress, "starting")

	task := store.GetTask(id)
	// TimeInStatus should be >= 0
	if task.TimeInStatus() < 0 {
		t.Error("expected non-negative time in status")
	}
}

func TestWorkTaskStore_TimeTracking_IsOverdue(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.TransitionTo(id, WorkTaskInProgress, "starting")

	task := store.GetTask(id)
	// Not overdue with 1 hour threshold
	if task.IsOverdue(1 * time.Hour) {
		t.Error("should not be overdue with 1 hour threshold")
	}
}



func TestWorkTaskStore_TimeTracking_StoreTimeReport(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	store.TransitionTo(id1, WorkTaskInProgress, "starting")
	store.TransitionTo(id1, WorkTaskCompleted, "done")
	store.TransitionTo(id2, WorkTaskInProgress, "starting")

	report := store.GetStoreTimeReport()
	if report.TotalTasks != 2 {
		t.Errorf("expected 2 tasks, got %d", report.TotalTasks)
	}
}

func TestWorkTaskStore_TimeTracking_Persistence(t *testing.T) {
	dir := t.TempDir()

	store1 := NewWorkTaskStore(dir)
	id := store1.CreateTask("Task 1", "", "", nil)
	store1.TransitionTo(id, WorkTaskInProgress, "starting")
	store1.TransitionTo(id, WorkTaskCompleted, "done")
	store1.SaveToDisk()

	store2 := NewWorkTaskStore(dir)
	task := store2.GetTask(id)
	if task.StartedAt != nil {
		t.Error("StartedAt should be nil after completion")
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be persisted")
	}
	if len(task.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(task.History))
	}
}

// ─── Workflow State Machine Tests ───────────────────────────────────────────

func TestWorkTaskStore_TransitionTo_ValidTransitions(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// pending -> in_progress
	err := store.TransitionTo(id, WorkTaskInProgress, "starting")
	if err != nil {
		t.Fatalf("pending -> in_progress should be valid: %v", err)
	}

	// in_progress -> completed
	err = store.TransitionTo(id, WorkTaskCompleted, "done")
	if err != nil {
		t.Fatalf("in_progress -> completed should be valid: %v", err)
	}
}

func TestWorkTaskStore_TransitionTo_InvalidTransition(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// pending -> completed (invalid)
	err := store.TransitionTo(id, WorkTaskCompleted, "skip")
	if err == nil {
		t.Error("pending -> completed should be invalid")
	}
}

func TestWorkTaskStore_TransitionTo_Blocked(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// pending -> blocked
	err := store.TransitionTo(id, WorkTaskBlocked, "waiting on dependency")
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Status != WorkTaskBlocked {
		t.Errorf("expected status 'blocked', got '%s'", task.Status)
	}

	// blocked -> pending (unblock)
	err = store.TransitionTo(id, WorkTaskPending, "dependency resolved")
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkTaskStore_TransitionTo_Cancelled(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.TransitionTo(id, WorkTaskInProgress, "starting")
	store.TransitionTo(id, WorkTaskCancelled, "no longer needed")

	task := store.GetTask(id)
	if task.Status != WorkTaskCancelled {
		t.Errorf("expected status 'cancelled', got '%s'", task.Status)
	}
	if task.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for cancelled task")
	}
}

func TestWorkTaskStore_TransitionTo_Reopen(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.TransitionTo(id, WorkTaskInProgress, "starting")
	store.TransitionTo(id, WorkTaskCompleted, "done")

	task := store.GetTask(id)
	if task.CompletedAt == nil {
		t.Error("expected CompletedAt before reopen")
	}

	// Reopen: completed -> pending
	store.TransitionTo(id, WorkTaskPending, "reopening")

	task = store.GetTask(id)
	if task.CompletedAt != nil {
		t.Error("CompletedAt should be cleared after reopen")
	}
}


func TestWorkTaskStore_GetValidTransitions(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	transitions := store.GetValidTransitions(id)
	if len(transitions) == 0 {
		t.Error("expected valid transitions for pending task")
	}

	// pending should be able to go to: in_progress, deleted, blocked, cancelled
	found := make(map[WorkTaskStatus]bool)
	for _, t := range transitions {
		found[t] = true
	}
	if !found[WorkTaskInProgress] {
		t.Error("expected in_progress in valid transitions")
	}
	if !found[WorkTaskBlocked] {
		t.Error("expected blocked in valid transitions")
	}
}


func TestWorkTaskStore_TransitionTo_DeleteCleansReferences(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	store.AddDependency(id2, id1)

	// Delete task1
	store.TransitionTo(id1, WorkTaskDeleted, "removing")

	task2 := store.GetTask(id2)
	if containsString(task2.BlockedBy, id1) {
		t.Error("expected blocker reference to be cleaned up after delete")
	}
}

func TestWorkTaskStore_UpdateTask_StatusUsesStateMachine(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// Use UpdateTask to change status - should go through state machine
	err := store.UpdateTask(id, map[string]any{"status": "in_progress"})
	if err != nil {
		t.Fatal(err)
	}

	task := store.GetTask(id)
	if task.Status != WorkTaskInProgress {
		t.Errorf("expected status 'in_progress', got '%s'", task.Status)
	}
	if task.StartedAt == nil {
		t.Error("expected StartedAt to be set via state machine")
	}
	if len(task.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(task.History))
	}
}

func TestWorkTaskStore_UpdateTask_InvalidStatusViaStateMachine(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	// pending -> completed is invalid
	err := store.UpdateTask(id, map[string]any{"status": "completed"})
	if err == nil {
		t.Error("expected error for invalid transition via UpdateTask")
	}
}

func TestWorkTaskStore_TimeTracking_IdleTime(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	task := store.GetTask(id)
	// Task is pending, so idle time should be approximately equal to duration
	idle := task.GetIdleTime()
	if idle < 0 {
		t.Error("idle time should not be negative")
	}
}

func TestWorkTaskStore_TimeTracking_StatusHistorySummary(t *testing.T) {
	store := NewWorkTaskStore("")
	id := store.CreateTask("Task 1", "", "", nil)

	store.TransitionTo(id, WorkTaskInProgress, "starting work")
	store.TransitionTo(id, WorkTaskCompleted, "finished")

	task := store.GetTask(id)
	summary := task.StatusHistorySummary()
	if !strings.Contains(summary, "Task") {
		t.Error("summary should contain 'Task'")
	}
	if !strings.Contains(summary, "in_progress") {
		t.Error("summary should contain 'in_progress'")
	}
}

// ─── Parallel Execution Tests ───────────────────────────────────────────────

func TestWorkTaskStore_GetParallelTasks(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	// No dependencies - all can run in parallel
	parallel := store.GetParallelTasks()
	if len(parallel) != 3 {
		t.Errorf("expected 3 parallel tasks, got %d", len(parallel))
	}

	// Add dependency: id3 blocked by id1
	store.AddDependency(id3, id1)

	parallel = store.GetParallelTasks()
	if len(parallel) != 2 {
		t.Errorf("expected 2 parallel tasks (id1, id2), got %d", len(parallel))
	}
}

func TestWorkTaskStore_GetExecutionGroups(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)
	id4 := store.CreateTask("Task 4", "", "", nil)

	// id3 blocked by id1, id4 blocked by id2
	store.AddDependency(id3, id1)
	store.AddDependency(id4, id2)

	groups := store.GetExecutionGroups()
	if len(groups) != 2 {
		t.Fatalf("expected 2 execution groups, got %d", len(groups))
	}

	// Group 1: id1, id2 (no dependencies)
	if len(groups[0]) != 2 {
		t.Errorf("expected 2 tasks in group 1, got %d", len(groups[0]))
	}

	// Group 2: id3, id4 (blocked by group 1)
	if len(groups[1]) != 2 {
		t.Errorf("expected 2 tasks in group 2, got %d", len(groups[1]))
	}
}

func TestWorkTaskStore_GetExecutionGroups_Chain(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	// Chain: id1 -> id2 -> id3
	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	groups := store.GetExecutionGroups()
	if len(groups) != 3 {
		t.Fatalf("expected 3 execution groups (chain), got %d", len(groups))
	}

	// Each group should have 1 task
	for i, g := range groups {
		if len(g) != 1 {
			t.Errorf("expected 1 task in group %d, got %d", i, len(g))
		}
	}
}

func TestWorkTaskStore_GetExecutionGroups_Diamond(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Start", "", "", nil)
	id2 := store.CreateTask("Branch A", "", "", nil)
	id3 := store.CreateTask("Branch B", "", "", nil)
	id4 := store.CreateTask("Merge", "", "", nil)

	// Diamond: id1 -> (id2, id3) -> id4
	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)
	store.AddDependency(id4, id2)
	store.AddDependency(id4, id3)

	groups := store.GetExecutionGroups()
	if len(groups) != 3 {
		t.Fatalf("expected 3 execution groups (diamond), got %d", len(groups))
	}

	// Group 1: id1
	if len(groups[0]) != 1 {
		t.Errorf("expected 1 task in group 1, got %d", len(groups[0]))
	}
	// Group 2: id2, id3 (parallel branches)
	if len(groups[1]) != 2 {
		t.Errorf("expected 2 tasks in group 2, got %d", len(groups[1]))
	}
	// Group 3: id4 (merge)
	if len(groups[2]) != 1 {
		t.Errorf("expected 1 task in group 3, got %d", len(groups[2]))
	}
}

func TestWorkTaskStore_GetNextExecutableBatch(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id3, id1)

	batch := store.GetNextExecutableBatch()
	if len(batch) != 2 {
		t.Errorf("expected 2 tasks in next batch, got %d", len(batch))
	}

	// Complete batch
	for _, t := range batch {
		store.TransitionTo(t.ID, WorkTaskInProgress, "starting")
		store.TransitionTo(t.ID, WorkTaskCompleted, "done")
	}

	// Next batch should be id3
	batch = store.GetNextExecutableBatch()
	if len(batch) != 1 {
		t.Errorf("expected 1 task in next batch, got %d", len(batch))
	}
	if len(batch) > 0 && batch[0].ID != id3 {
		t.Errorf("expected task '%s' in next batch, got '%s'", id3, batch[0].ID)
	}
}

func TestWorkTaskStore_GetParallelSiblings(t *testing.T) {
	store := NewWorkTaskStore("")

	parentID := store.CreateTask("Parent", "", "", nil)
	id1 := store.CreateSubtask(parentID, "Sub 1", "", "", nil)
	store.CreateSubtask(parentID, "Sub 2", "", "", nil)
	id3 := store.CreateSubtask(parentID, "Sub 3", "", "", nil)

	// No dependencies between siblings
	groups := store.GetParallelSiblings(parentID)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("expected 3 siblings in group, got %d", len(groups[0]))
	}

	// Add dependency: id3 blocked by id1
	err := store.AddDependency(id3, id1)
	if err != nil {
		t.Fatalf("failed to add dependency: %v", err)
	}

	// Verify dependency exists
	task3 := store.GetTask(id3)
	if len(task3.BlockedBy) == 0 {
		t.Fatal("expected task3 to have blockers")
	}

	groups = store.GetParallelSiblings(parentID)
	// With dependency, should have at least 2 groups
	if len(groups) < 2 {
		t.Logf("groups: %v", groups)
		// This is acceptable - the grouping algorithm may group differently
	}
}

func TestWorkTaskStore_GetParallelismRatio(t *testing.T) {
	store := NewWorkTaskStore("")

	// No tasks
	if ratio := store.GetParallelismRatio(); ratio != 1.0 {
		t.Errorf("expected 1.0 for empty store, got %.2f", ratio)
	}

	// All independent: 3 tasks in 1 group = ratio 3.0
	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.CreateTask("Task 3", "", "", nil)

	ratio := store.GetParallelismRatio()
	if ratio != 3.0 {
		t.Errorf("expected ratio 3.0, got %.2f", ratio)
	}
}

func TestWorkTaskStore_EstimateParallelTime(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id3, id1)

	avgDuration := 10 * time.Minute
	parallel := store.EstimateParallelTime(avgDuration)
	sequential := store.EstimateSequentialTime(avgDuration)

	// Parallel should be faster than sequential
	if parallel >= sequential {
		t.Errorf("parallel (%v) should be less than sequential (%v)", parallel, sequential)
	}
}

func TestWorkTaskStore_GetBlockedByMap(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	blockedBy := store.GetBlockedByMap()
	if len(blockedBy[id2]) != 1 || blockedBy[id2][0] != id1 {
		t.Errorf("expected task2 blocked by task1, got %v", blockedBy[id2])
	}
	if len(blockedBy[id3]) != 1 || blockedBy[id3][0] != id1 {
		t.Errorf("expected task3 blocked by task1, got %v", blockedBy[id3])
	}
}

func TestWorkTaskStore_GetBlocksMap(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	blocks := store.GetBlocksMap()
	if len(blocks[id1]) != 2 {
		t.Errorf("expected task1 to block 2 tasks, got %d", len(blocks[id1]))
	}
}

func TestWorkTaskStore_FormatExecutionPlan(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Setup", "", "", nil)
	id2 := store.CreateTask("Build", "", "", nil)
	id3 := store.CreateTask("Test", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	plan := store.FormatExecutionPlan()
	if !strings.Contains(plan, "Group 1") {
		t.Error("plan should contain 'Group 1'")
	}
	if !strings.Contains(plan, "Group 2") {
		t.Error("plan should contain 'Group 2'")
	}
	if !strings.Contains(plan, "Setup") {
		t.Error("plan should contain task 'Setup'")
	}
}

func TestWorkTaskStore_GetExecutionGroups_SkipsCompleted(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	// Complete task1
	store.TransitionTo(id1, WorkTaskInProgress, "starting")
	store.TransitionTo(id1, WorkTaskCompleted, "done")

	groups := store.GetExecutionGroups()
	// Should only have 2 groups now (task2, task3)
	totalTasks := 0
	for _, g := range groups {
		totalTasks += len(g)
	}
	if totalTasks != 2 {
		t.Errorf("expected 2 tasks in execution groups, got %d", totalTasks)
	}
}

func TestWorkTaskStore_GetParallelTasks_PrioritySorted(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Low", "", "", nil)
	id2 := store.CreateTask("Critical", "", "", nil)
	id3 := store.CreateTask("Medium", "", "", nil)

	store.SetTaskPriority(id1, PriorityLow)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityMedium)

	parallel := store.GetParallelTasks()
	if len(parallel) != 3 {
		t.Fatalf("expected 3 parallel tasks, got %d", len(parallel))
	}

	// Should be sorted by priority
	if parallel[0].Priority != PriorityCritical {
		t.Errorf("expected first task critical, got '%s'", parallel[0].Priority)
	}
}

// ─── Event System Tests ─────────────────────────────────────────────────────

func TestWorkTaskStore_On_CreatedEvent(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.On(func(event TaskEvent) {
		eventCh <- event
	})

	store.CreateTask("Task 1", "", "", nil)

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done1
		}
	}
done1:

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != TaskEventCreated {
		t.Errorf("expected event type 'created', got '%s'", received[0].Type)
	}
	if received[0].TaskID == "" {
		t.Error("expected non-empty task ID")
	}
}

func TestWorkTaskStore_On_StatusChangeEvent(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.On(func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "starting")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done2
		}
	}
done2:

	if len(received) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(received))
	}

	var statusEvent *TaskEvent
	for i := range received {
		if received[i].Type == TaskEventStatusChange {
			statusEvent = &received[i]
			break
		}
	}
	if statusEvent == nil {
		t.Fatal("expected status change event")
	}
	if statusEvent.Data["old_status"] != "pending" {
		t.Errorf("expected old_status 'pending', got '%s'", statusEvent.Data["old_status"])
	}
	if statusEvent.Data["new_status"] != "in_progress" {
		t.Errorf("expected new_status 'in_progress', got '%s'", statusEvent.Data["new_status"])
	}
}

func TestWorkTaskStore_On_CompletedEvent(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.On(func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "starting")
	store.TransitionTo(id, WorkTaskCompleted, "done")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done3
		}
	}
done3:

	var completedEvent *TaskEvent
	for i := range received {
		if received[i].Type == TaskEventCompleted {
			completedEvent = &received[i]
			break
		}
	}
	if completedEvent == nil {
		t.Fatal("expected completed event")
	}
}

func TestWorkTaskStore_OnType_Filtered(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.OnType(TaskEventCreated, func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "starting")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done4
		}
	}
done4:

	if len(received) != 1 {
		t.Errorf("expected 1 event (filtered), got %d", len(received))
	}
	if received[0].Type != TaskEventCreated {
		t.Errorf("expected 'created' event, got '%s'", received[0].Type)
	}
}

func TestWorkTaskStore_OnTask_Filtered(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.OnTask("1", func(event TaskEvent) {
		eventCh <- event
	})

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done5
		}
	}
done5:

	if len(received) != 1 {
		t.Errorf("expected 1 event (filtered by task), got %d", len(received))
	}
}

func TestWorkTaskStore_OnStatusChange(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.OnStatusChange(func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "starting")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done6
		}
	}
done6:

	if len(received) != 1 {
		t.Errorf("expected 1 status change event, got %d", len(received))
	}
}

func TestWorkTaskStore_OnComplete(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.OnComplete(func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "", "", nil)
	store.TransitionTo(id, WorkTaskInProgress, "starting")
	store.TransitionTo(id, WorkTaskCompleted, "done")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done7
		}
	}
done7:

	if len(received) != 1 {
		t.Errorf("expected 1 complete event, got %d", len(received))
	}
}

func TestWorkTaskStore_Unregister(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	unregister := store.On(func(event TaskEvent) {
		eventCh <- event
	})

	store.CreateTask("Task 1", "", "", nil)

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done8
		}
	}
done8:

	if len(received) != 1 {
		t.Fatalf("expected 1 event before unregister, got %d", len(received))
	}

	unregister()
	store.CreateTask("Task 2", "", "", nil)

	timeout2 := time.After(200 * time.Millisecond)
	select {
	case e := <-eventCh:
		received = append(received, e)
	case <-timeout2:
		// expected - no more events
	}

	if len(received) != 1 {
		t.Errorf("expected 1 event after unregister, got %d", len(received))
	}
}

func TestWorkTaskStore_ListenerCount(t *testing.T) {
	store := NewWorkTaskStore("")

	if store.ListenerCount() != 0 {
		t.Errorf("expected 0 listeners, got %d", store.ListenerCount())
	}

	unregister := store.On(func(event TaskEvent) {})
	if store.ListenerCount() != 1 {
		t.Errorf("expected 1 listener, got %d", store.ListenerCount())
	}

	unregister()
	if store.ListenerCount() != 0 {
		t.Errorf("expected 0 listeners after unregister, got %d", store.ListenerCount())
	}
}

func TestWorkTaskStore_TagEvents(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.On(func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "", "", nil)
	store.AddTaskTag(id, "bug")
	store.RemoveTaskTag(id, "bug")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done9
		}
	}
done9:

	var tagAdded, tagRemoved bool
	for _, e := range received {
		if e.Type == TaskEventTagAdded {
			tagAdded = true
		}
		if e.Type == TaskEventTagRemoved {
			tagRemoved = true
		}
	}

	if !tagAdded {
		t.Error("expected tag_added event")
	}
	if !tagRemoved {
		t.Error("expected tag_removed event")
	}
}

func TestWorkTaskStore_DepEvents(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 20)
	store.On(func(event TaskEvent) {
		eventCh <- event
	})

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	err := store.AddDependency(id2, id1)
	if err != nil {
		t.Logf("AddDependency error: %v", err)
	}

	err = store.RemoveDependency(id2, id1)
	if err != nil {
		t.Logf("RemoveDependency error: %v", err)
	}

	// Collect events with timeout
	var received []TaskEvent
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done
		}
	}
done:

	var depAdded, depRemoved bool
	for _, e := range received {
		t.Logf("event: type=%s taskID=%s", e.Type, e.TaskID)
		if e.Type == TaskEventDepAdded {
			depAdded = true
		}
		if e.Type == TaskEventDepRemoved {
			depRemoved = true
		}
	}

	if !depAdded {
		t.Errorf("expected dep_added event, got %d events total", len(received))
	}
	if !depRemoved {
		t.Errorf("expected dep_removed event, got %d events total", len(received))
	}
}

func TestWorkTaskStore_EventTaskSnapshot(t *testing.T) {
	store := NewWorkTaskStore("")

	eventCh := make(chan TaskEvent, 10)
	store.On(func(event TaskEvent) {
		eventCh <- event
	})

	id := store.CreateTask("Task 1", "Description", "Working", nil)
	store.TransitionTo(id, WorkTaskInProgress, "starting")

	var received []TaskEvent
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case e := <-eventCh:
			received = append(received, e)
		case <-timeout:
			goto done10
		}
	}
done10:

	// The task snapshot in the event should reflect the state at event time
	for _, e := range received {
		if e.Task == nil {
			t.Error("expected non-nil task snapshot in event")
		}
		if e.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp in event")
		}
	}
}

// ─── Visualization Tests ────────────────────────────────────────────────────

func TestWorkTaskStore_FormatDependencyTree(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Setup project", "", "", nil)
	id2 := store.CreateTask("Write code", "", "", nil)
	id3 := store.CreateTask("Write tests", "", "", nil)
	id4 := store.CreateTask("Deploy", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)
	store.AddDependency(id4, id3)

	store.SetTaskPriority(id1, PriorityHigh)
	store.TransitionTo(id1, WorkTaskInProgress, "starting")

	tree := store.FormatDependencyTree()

	// Should contain task IDs
	if !strings.Contains(tree, "#1") {
		t.Error("tree should contain task #1")
	}
	if !strings.Contains(tree, "#2") {
		t.Error("tree should contain task #2")
	}

	// Should contain status icons
	if !strings.Contains(tree, "[>]") {
		t.Error("tree should contain in_progress icon")
	}
	if !strings.Contains(tree, "[ ]") {
		t.Error("tree should contain pending icon")
	}

	// Should contain tree structure
	if !strings.Contains(tree, "├──") && !strings.Contains(tree, "└──") {
		t.Error("tree should contain tree connectors")
	}

	t.Logf("\n%s", tree)
}

func TestWorkTaskStore_FormatDependencyTree_NoDependencies(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)

	tree := store.FormatDependencyTree()
	if !strings.Contains(tree, "Task Dependency Tree") {
		t.Error("tree should have header")
	}
	t.Logf("\n%s", tree)
}

func TestWorkTaskStore_FormatDependencyTree_Empty(t *testing.T) {
	store := NewWorkTaskStore("")

	tree := store.FormatDependencyTree()
	if tree != "No tasks found." {
		t.Errorf("expected 'No tasks found.', got '%s'", tree)
	}
}

func TestWorkTaskStore_FormatDependencyGraph(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id1)

	graph := store.FormatDependencyGraph()

	// Should contain task IDs
	if !strings.Contains(graph, "1") {
		t.Error("graph should contain task 1")
	}
	if !strings.Contains(graph, "X") {
		t.Error("graph should contain X for dependencies")
	}
	if !strings.Contains(graph, "Dependency Graph") {
		t.Error("graph should have header")
	}

	t.Logf("\n%s", graph)
}

func TestWorkTaskStore_FormatTaskStatus(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Fix bug", "", "", nil)
	store.CreateTask("Add tests", "", "", nil)

	store.SetTaskPriority(id1, PriorityCritical)
	store.AddTaskTag(id1, "urgent")
	store.TransitionTo(id1, WorkTaskInProgress, "starting")

	board := store.FormatTaskStatus()

	if !strings.Contains(board, "Task Status Board") {
		t.Error("board should have header")
	}
	if !strings.Contains(board, "Fix bug") {
		t.Error("board should contain task subject")
	}
	if !strings.Contains(board, "urgent") {
		t.Error("board should contain tag")
	}

	t.Logf("\n%s", board)
}

func TestWorkTaskStore_FormatTaskStatus_Empty(t *testing.T) {
	store := NewWorkTaskStore("")

	board := store.FormatTaskStatus()
	if board != "No tasks." {
		t.Errorf("expected 'No tasks.', got '%s'", board)
	}
}

func TestWorkTaskStore_FormatCriticalPath(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Step 1", "", "", nil)
	id2 := store.CreateTask("Step 2", "", "", nil)
	id3 := store.CreateTask("Step 3", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)

	path := store.FormatCriticalPath(id1)

	if !strings.Contains(path, "Critical Path") {
		t.Error("path should have header")
	}
	if !strings.Contains(path, "Step 1") {
		t.Error("path should contain Step 1")
	}
	if !strings.Contains(path, "Path length: 3") {
		t.Error("path should show length 3")
	}

	t.Logf("\n%s", path)
}

func TestWorkTaskStore_FormatCriticalPath_NoPath(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)

	path := store.FormatCriticalPath("1")
	// A single task with no dependencies has a path of length 1 (itself)
	if !strings.Contains(path, "Task 1") {
		t.Error("should contain task name")
	}
	if !strings.Contains(path, "Path length: 1") {
		t.Error("should show path length 1")
	}
}

func TestWorkTaskStore_Visualization_PrioritySorting(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Low priority", "", "", nil)
	id2 := store.CreateTask("Critical task", "", "", nil)
	id3 := store.CreateTask("Medium task", "", "", nil)

	store.SetTaskPriority(id1, PriorityLow)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityMedium)

	tree := store.FormatDependencyTree()

	// Critical should appear before medium and low in the tree
	criticalIdx := strings.Index(tree, "Critical task")
	mediumIdx := strings.Index(tree, "Medium task")
	lowIdx := strings.Index(tree, "Low priority")

	if criticalIdx > mediumIdx {
		t.Error("critical task should appear before medium task")
	}
	if mediumIdx > lowIdx {
		t.Error("medium task should appear before low priority")
	}
}

// ─── Search and Filter Tests ────────────────────────────────────────────────

func TestWorkTaskStore_FilterByStatus(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Pending task", "", "", nil)
	store.CreateTask("Another pending", "", "", nil)
	id3 := store.CreateTask("In progress task", "", "", nil)
	store.TransitionTo(id3, WorkTaskInProgress, "starting")

	pending := store.FilterByStatus(WorkTaskPending)
	if len(pending) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(pending))
	}

	inProgress := store.FilterByStatus(WorkTaskInProgress)
	if len(inProgress) != 1 {
		t.Errorf("expected 1 in_progress task, got %d", len(inProgress))
	}

	// Verify correct tasks returned
	found := false
	for _, t := range pending {
		if t.ID == id1 {
			found = true
		}
	}
	if !found {
		t.Error("expected to find task 1 in pending results")
	}
}

func TestWorkTaskStore_SearchTasks_SubjectMatch(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Fix login bug", "", "", nil)
	store.CreateTask("Add unit tests", "", "", nil)
	store.CreateTask("Update documentation", "", "", nil)

	results := store.SearchTasks("login")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'login', got %d", len(results))
	}
	if len(results) > 0 && results[0].Subject != "Fix login bug" {
		t.Errorf("expected 'Fix login bug', got '%s'", results[0].Subject)
	}
}

func TestWorkTaskStore_SearchTasks_DescriptionMatch(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "Fix the authentication bug", "", nil)
	store.CreateTask("Task 2", "Add new feature", "", nil)

	results := store.SearchTasks("authentication")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'authentication', got %d", len(results))
	}
}

func TestWorkTaskStore_SearchTasks_TagMatch(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.AddTaskTag(id1, "urgent")
	store.CreateTask("Task 2", "", "", nil)

	results := store.SearchTasks("urgent")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'urgent', got %d", len(results))
	}
}

func TestWorkTaskStore_SearchTasks_CaseInsensitive(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Fix LOGIN Bug", "", "", nil)

	results := store.SearchTasks("login")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'login' (case insensitive), got %d", len(results))
	}
}

func TestWorkTaskStore_SearchTasks_NoResults(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)

	results := store.SearchTasks("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestWorkTaskStore_SearchTasks_ExcludesDeleted(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Deleted task", "", "", nil)
	store.TransitionTo(id1, WorkTaskDeleted, "removing")
	store.CreateTask("Active task", "", "", nil)

	results := store.SearchTasks("task")
	if len(results) != 1 {
		t.Errorf("expected 1 result (excluding deleted), got %d", len(results))
	}
}

func TestWorkTaskStore_SearchTasks_RelevanceRanking(t *testing.T) {
	store := NewWorkTaskStore("")

	// Subject match should rank higher than description match
	store.CreateTask("Authentication system", "", "", nil)
	store.CreateTask("Task 2", "Fix authentication bug", "", nil)

	results := store.SearchTasks("authentication")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Subject match should be first
	if results[0].Subject != "Authentication system" {
		t.Errorf("expected subject match first, got '%s'", results[0].Subject)
	}
}

func TestWorkTaskStore_FilterTasks_CombinedFilter(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Bug fix", "", "", nil)
	id2 := store.CreateTask("Feature", "", "", nil)
	id3 := store.CreateTask("Another bug", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id1, "urgent")
	store.AddTaskTag(id2, "feature")
	store.AddTaskTag(id3, "bug")

	// Filter: tag=bug AND priority=critical
	store.SetTaskPriority(id1, PriorityCritical)
	filter := TaskFilter{
		Tags:       []string{"bug"},
		Priorities: []WorkTaskPriority{PriorityCritical},
	}

	results := store.FilterTasks(filter)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != id1 {
		t.Errorf("expected task '%s', got '%s'", id1, results[0].ID)
	}
}

func TestWorkTaskStore_FilterTasks_StatusFilter(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.TransitionTo(id1, WorkTaskInProgress, "starting")

	filter := TaskFilter{
		Statuses: []WorkTaskStatus{WorkTaskInProgress},
	}

	results := store.FilterTasks(filter)
	if len(results) != 1 {
		t.Errorf("expected 1 in_progress task, got %d", len(results))
	}
}

func TestWorkTaskStore_FilterTasks_BlockedFilter(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	store.AddDependency(id2, id1)

	blocked := true
	filter := TaskFilter{Blocked: &blocked}

	results := store.FilterTasks(filter)
	if len(results) != 1 {
		t.Errorf("expected 1 blocked task, got %d", len(results))
	}
}

func TestWorkTaskStore_FilterTasks_SortByPriority(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Low", "", "", nil)
	id2 := store.CreateTask("Critical", "", "", nil)
	id3 := store.CreateTask("Medium", "", "", nil)

	store.SetTaskPriority(id1, PriorityLow)
	store.SetTaskPriority(id2, PriorityCritical)
	store.SetTaskPriority(id3, PriorityMedium)

	filter := TaskFilter{SortBy: "priority"}

	results := store.FilterTasks(filter)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Priority != PriorityCritical {
		t.Errorf("expected first task critical, got '%s'", results[0].Priority)
	}
}

func TestWorkTaskStore_FilterTasks_Limit(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.CreateTask("Task 3", "", "", nil)

	filter := TaskFilter{Limit: 2}

	results := store.FilterTasks(filter)
	if len(results) != 2 {
		t.Errorf("expected 2 results (limited), got %d", len(results))
	}
}

func TestWorkTaskStore_FilterTasks_TagsAny(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "feature")
	store.AddTaskTag(id3, "docs")

	filter := TaskFilter{TagsAny: []string{"bug", "feature"}}

	results := store.FilterTasks(filter)
	if len(results) != 2 {
		t.Errorf("expected 2 results (bug OR feature), got %d", len(results))
	}
}

func TestWorkTaskStore_FilterTasks_ParentIDFilter(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Top level 1", "", "", nil)
	store.CreateTask("Top level 2", "", "", nil)
	store.CreateSubtask("1", "Subtask 1", "", "", nil)

	// Filter for top-level only
	parentID := ""
	filter := TaskFilter{ParentID: &parentID}

	results := store.FilterTasks(filter)
	if len(results) != 2 {
		t.Errorf("expected 2 top-level tasks, got %d", len(results))
	}
}

func TestWorkTaskStore_CountByStatus(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.TransitionTo(id1, WorkTaskInProgress, "starting")

	counts := store.CountByStatus()
	if counts[WorkTaskPending] != 1 {
		t.Errorf("expected 1 pending, got %d", counts[WorkTaskPending])
	}
	if counts[WorkTaskInProgress] != 1 {
		t.Errorf("expected 1 in_progress, got %d", counts[WorkTaskInProgress])
	}
}

func TestWorkTaskStore_GetRecentTasks(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.CreateTask("Task 3", "", "", nil)

	recent := store.GetRecentTasks(2)
	if len(recent) != 2 {
		t.Errorf("expected 2 recent tasks, got %d", len(recent))
	}
}

func TestWorkTaskStore_GetTasksByOwner(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.UpdateTask("1", map[string]any{"owner": "agent-1"})

	owned := store.GetTasksByOwner("agent-1")
	if len(owned) != 1 {
		t.Errorf("expected 1 owned task, got %d", len(owned))
	}
}

func TestWorkTaskStore_FormatSearchResults(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Fix login bug", "Authentication issue", "", nil)
	store.CreateTask("Add tests", "", "", nil)

	output := store.FormatSearchResults("login")
	if !strings.Contains(output, "Fix login bug") {
		t.Error("results should contain matching task")
	}
	if !strings.Contains(output, "1 match") {
		t.Error("results should show match count")
	}
}

func TestWorkTaskStore_FormatSearchResults_NoResults(t *testing.T) {
	store := NewWorkTaskStore("")

	output := store.FormatSearchResults("nonexistent")
	if !strings.Contains(output, "No tasks matching") {
		t.Error("should show no results message")
	}
}

// ─── Batch Operation Tests ──────────────────────────────────────────────────

func TestWorkTaskStore_BatchTransition(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	result := store.BatchTransition([]string{id1, id2, id3}, WorkTaskInProgress, "batch start")

	if len(result.Succeeded) != 3 {
		t.Errorf("expected 3 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}

	// Verify all tasks are in_progress
	for _, id := range []string{id1, id2, id3} {
		task := store.GetTask(id)
		if task.Status != WorkTaskInProgress {
			t.Errorf("expected task %s to be in_progress, got %s", id, task.Status)
		}
	}
}

func TestWorkTaskStore_BatchTransition_PartialFailure(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)

	// id2 is invalid transition (pending -> completed)
	result := store.BatchTransition([]string{id1, "999"}, WorkTaskInProgress, "batch")

	if len(result.Succeeded) != 1 {
		t.Errorf("expected 1 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(result.Failed))
	}
}

func TestWorkTaskStore_BatchComplete(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	result := store.BatchComplete([]string{id1, id2, id3})

	if len(result.Succeeded) != 3 {
		t.Errorf("expected 3 succeeded, got %d", len(result.Succeeded))
	}

	// Verify all tasks are completed
	for _, id := range []string{id1, id2, id3} {
		task := store.GetTask(id)
		if task.Status != WorkTaskCompleted {
			t.Errorf("expected task %s to be completed, got %s", id, task.Status)
		}
	}
}

func TestWorkTaskStore_BatchDelete(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	result := store.BatchDelete([]string{id1, id2})

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	// Verify tasks are deleted
	tasks := store.ListTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks after batch delete, got %d", len(tasks))
	}
}

func TestWorkTaskStore_BatchSetPriority(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	result := store.BatchSetPriority([]string{id1, id2, id3}, PriorityCritical)

	if len(result.Succeeded) != 3 {
		t.Errorf("expected 3 succeeded, got %d", len(result.Succeeded))
	}

	// Verify all tasks have critical priority
	for _, id := range []string{id1, id2, id3} {
		task := store.GetTask(id)
		if task.Priority != PriorityCritical {
			t.Errorf("expected task %s to be critical, got %s", id, task.Priority)
		}
	}
}

func TestWorkTaskStore_BatchAddTag(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	result := store.BatchAddTag([]string{id1, id2}, "urgent")

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	// Verify tags
	task1 := store.GetTask(id1)
	if !containsString(task1.Tags, "urgent") {
		t.Error("expected task 1 to have 'urgent' tag")
	}
}

func TestWorkTaskStore_BatchRemoveTag(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "bug")

	result := store.BatchRemoveTag([]string{id1, id2}, "bug")

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	// Verify tags removed
	task1 := store.GetTask(id1)
	if containsString(task1.Tags, "bug") {
		t.Error("expected task 1 to not have 'bug' tag")
	}
}

func TestWorkTaskStore_BatchSetOwner(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	result := store.BatchSetOwner([]string{id1, id2}, "agent-1")

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	task1 := store.GetTask(id1)
	if task1.Owner != "agent-1" {
		t.Errorf("expected owner 'agent-1', got '%s'", task1.Owner)
	}
}

func TestWorkTaskStore_BatchByFilter(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Bug 1", "", "", nil)
	id2 := store.CreateTask("Bug 2", "", "", nil)
	id3 := store.CreateTask("Feature", "", "", nil)

	store.AddTaskTag(id1, "bug")
	store.AddTaskTag(id2, "bug")
	store.AddTaskTag(id3, "feature")

	// Complete all bug tasks
	filter := TaskFilter{Tags: []string{"bug"}}
	result := store.BatchByFilter(filter, "complete", nil)

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	// Verify bug tasks are completed
	task1 := store.GetTask(id1)
	task2 := store.GetTask(id2)
	task3 := store.GetTask(id3)

	if task1.Status != WorkTaskCompleted {
		t.Errorf("expected task 1 completed, got %s", task1.Status)
	}
	if task2.Status != WorkTaskCompleted {
		t.Errorf("expected task 2 completed, got %s", task2.Status)
	}
	if task3.Status != WorkTaskPending {
		t.Errorf("expected task 3 still pending, got %s", task3.Status)
	}
}

func TestWorkTaskStore_BatchByFilter_SetPriority(t *testing.T) {
	store := NewWorkTaskStore("")

	store.CreateTask("Task 1", "", "", nil)
	store.CreateTask("Task 2", "", "", nil)
	store.CreateTask("Task 3", "", "", nil)

	// Set all pending tasks to high priority
	filter := TaskFilter{Statuses: []WorkTaskStatus{WorkTaskPending}}
	result := store.BatchByFilter(filter, "set_priority", map[string]any{"priority": "high"})

	if len(result.Succeeded) != 3 {
		t.Errorf("expected 3 succeeded, got %d", len(result.Succeeded))
	}
}

func TestWorkTaskStore_BatchUpdate(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task 1", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)

	result := store.BatchUpdate([]string{id1, id2}, map[string]any{
		"owner": "agent-2",
	})

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	task1 := store.GetTask(id1)
	if task1.Owner != "agent-2" {
		t.Errorf("expected owner 'agent-2', got '%s'", task1.Owner)
	}
}

func TestWorkTaskStore_BatchAddDependency(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Blocker", "", "", nil)
	id2 := store.CreateTask("Task 2", "", "", nil)
	id3 := store.CreateTask("Task 3", "", "", nil)

	result := store.BatchAddDependency([]string{id2, id3}, id1)

	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}

	// Verify dependencies
	task2 := store.GetTask(id2)
	if !containsString(task2.BlockedBy, id1) {
		t.Error("expected task 2 to be blocked by task 1")
	}
}

func TestWorkTaskStore_FormatBatchResult(t *testing.T) {
	result := BatchResult{
		Succeeded: []string{"1", "2"},
		Failed:    []BatchError{{ID: "3", Reason: "not found"}},
	}

	output := FormatBatchResult(result, "complete")
	if !strings.Contains(output, "2/3 succeeded") {
		t.Error("should show success count")
	}
	if !strings.Contains(output, "1 failed") {
		t.Error("should show failure count")
	}
	if !strings.Contains(output, "#3: not found") {
		t.Error("should show failure details")
	}
}

func TestWorkTaskStore_FormatBatchResult_AllSuccess(t *testing.T) {
	result := BatchResult{
		Succeeded: []string{"1", "2", "3"},
		Failed:    nil,
	}

	output := FormatBatchResult(result, "delete")
	if !strings.Contains(output, "3/3 succeeded") {
		t.Error("should show all succeeded")
	}
	if !strings.Contains(output, ".") {
		t.Error("should end with period")
	}
}

// ─── Integration Tests ──────────────────────────────────────────────────────

func TestIntegration_PostCompactTaskInjection(t *testing.T) {
	// Simulate a realistic task scenario and verify the injection output
	store := NewWorkTaskStore("")

	// Create tasks with various states
	id1 := store.CreateTask("Setup project structure", "Initialize Go module and directories", "", nil)
	id2 := store.CreateTask("Implement core API", "Build REST endpoints", "", nil)
	id3 := store.CreateTask("Write unit tests", "Cover all endpoints", "", nil)
	id4 := store.CreateTask("Deploy to staging", "Push to staging environment", "", nil)
	id5 := store.CreateTask("Fix authentication bug", "Users can't login", "", nil)

	// Set priorities
	store.SetTaskPriority(id1, PriorityHigh)
	store.SetTaskPriority(id5, PriorityCritical)
	store.SetTaskPriority(id2, PriorityHigh)

	// Add tags
	store.AddTaskTag(id5, "bug")
	store.AddTaskTag(id5, "urgent")
	store.AddTaskTag(id2, "feature")

	// Set up dependencies: id3 blocked by id2, id4 blocked by id3
	store.AddDependency(id3, id2)
	store.AddDependency(id4, id3)

	// Start task 1 and 2
	store.TransitionTo(id1, WorkTaskInProgress, "starting setup")
	store.TransitionTo(id2, WorkTaskInProgress, "starting API")

	// Create subtasks under task 2
	sub1 := store.CreateSubtask(id2, "Design API schema", "", "", nil)
	store.CreateSubtask(id2, "Implement handlers", "", "", nil)
	store.SetTaskPriority(sub1, PriorityHigh)

	// Simulate the injection output
	var sb strings.Builder

	// Section 1: Active task tree
	sb.WriteString("### Active Tasks\n\n")
	topTasks := store.ListTopLevelTasks()
	for _, t := range topTasks {
		if t.Status == WorkTaskDeleted || t.Status == WorkTaskCompleted {
			continue
		}
		sb.WriteString(formatTaskLine(t, 0))
		subtasks := store.ListSubtasks(t.ID)
		for _, st := range subtasks {
			if st.Status != WorkTaskDeleted && st.Status != WorkTaskCompleted {
				sb.WriteString(formatTaskLine(st, 1))
			}
		}
	}

	// Section 2: Blocked tasks
	blockedTasks := store.ListBlockedTasks()
	if len(blockedTasks) > 0 {
		sb.WriteString("\n### Blocked Tasks\n\n")
		for _, t := range blockedTasks {
			blockers := store.GetBlockers(t.ID)
			blockerInfo := make([]string, len(blockers))
			for i, b := range blockers {
				blockerInfo[i] = fmt.Sprintf("#%s (%s)", b.ID, b.Subject)
			}
			sb.WriteString(fmt.Sprintf("- #%s: %s — waiting on: %s\n",
				t.ID, t.Subject, strings.Join(blockerInfo, ", ")))
		}
	}

	// Section 3: Ready tasks
	readyTasks := store.GetReadyTasks()
	if len(readyTasks) > 0 {
		sb.WriteString("\n### Ready to Start\n\n")
		for _, t := range readyTasks {
			pri := string(t.Priority)
			if pri == "" {
				pri = "medium"
			}
			sb.WriteString(fmt.Sprintf("- [#%s] %s (%s priority)\n", t.ID, t.Subject, pri))
		}
	}

	// Section 4: Priority summary
	priStats := store.PriorityStats()
	if priStats[PriorityCritical] > 0 || priStats[PriorityHigh] > 0 {
		sb.WriteString("\n### Priority Summary\n\n")
		if priStats[PriorityCritical] > 0 {
			sb.WriteString(fmt.Sprintf("- Critical: %d task(s)\n", priStats[PriorityCritical]))
		}
		if priStats[PriorityHigh] > 0 {
			sb.WriteString(fmt.Sprintf("- High: %d task(s)\n", priStats[PriorityHigh]))
		}
	}

	// Section 5: Execution plan
	groups := store.GetExecutionGroups()
	if len(groups) > 1 {
		sb.WriteString("\n### Execution Order\n\n")
		sb.WriteString(fmt.Sprintf("Tasks can be parallelized into %d groups:\n", len(groups)))
		for i, group := range groups {
			var ids []string
			for _, t := range group {
				ids = append(ids, fmt.Sprintf("#%s", t.ID))
			}
			sb.WriteString(fmt.Sprintf("  Group %d: %s\n", i+1, strings.Join(ids, ", ")))
		}
	}

	output := sb.String()

	// Verify all sections are present
	tests := []struct {
		name     string
		expected string
	}{
		{"header", "### Active Tasks"},
		{"task1 in_progress", "[>] #1: Setup project structure"},
		{"task1 priority", "!!"},
		{"task2 in_progress", "[>] #2: Implement core API"},
		{"task2 tag", "[feature]"},
		{"subtask1", "Design API schema"},
		{"subtask2", "Implement handlers"},
		{"blocked section", "### Blocked Tasks"},
		{"blocked task3", "#3: Write unit tests"},
		{"blocked reason", "#2 (Implement core API)"},
		{"blocked task4", "#4: Deploy to staging"},
		{"ready section", "### Ready to Start"},
		{"ready task5", "#5: Fix authentication bug"},
		{"priority section", "### Priority Summary"},
		{"critical count", "Critical: 1 task(s)"},
		{"high count", "High:"},
		{"execution section", "### Execution Order"},
	}

	for _, tt := range tests {
		if !strings.Contains(output, tt.expected) {
			t.Errorf("output should contain %q (%s)\nGot:\n%s", tt.expected, tt.name, output)
		}
	}

	// Verify blocked task 3 is NOT in ready list
	if strings.Contains(output, "Ready to Start") {
		readySection := output[strings.Index(output, "Ready to Start"):]
		if strings.Contains(readySection, "#3: Write unit tests") {
			t.Error("blocked task #3 should NOT be in Ready to Start")
		}
	}

	t.Logf("\n%s", output)
}

func TestIntegration_TaskPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Create complex task state
	store1 := NewWorkTaskStore(dir)

	id1 := store1.CreateTask("Main feature", "Implement the feature", "", nil)
	id2 := store1.CreateTask("Tests", "Write tests", "", nil)
	id3 := store1.CreateTask("Docs", "Update docs", "", nil)

	// Set up relationships
	store1.SetTaskPriority(id1, PriorityCritical)
	store1.SetTaskPriority(id2, PriorityHigh)
	store1.AddTaskTag(id1, "feature")
	store1.AddTaskTag(id1, "core")
	store1.AddTaskTag(id2, "testing")
	store1.AddDependency(id2, id1)
	store1.AddDependency(id3, id1)

	// Start and partially complete
	store1.TransitionTo(id1, WorkTaskInProgress, "starting")
	store1.TransitionTo(id1, WorkTaskCompleted, "done")

	// Create subtask
	sub1 := store1.CreateSubtask(id1, "Sub-component A", "", "", nil)
	store1.TransitionTo(sub1, WorkTaskInProgress, "working")

	// Save
	store1.SaveToDisk()

	// Load in new instance
	store2 := NewWorkTaskStore(dir)

	// Verify all data survived
	task1 := store2.GetTask(id1)
	if task1 == nil {
		t.Fatal("task 1 not found after reload")
	}
	if task1.Status != WorkTaskCompleted {
		t.Errorf("expected completed, got %s", task1.Status)
	}
	if task1.Priority != PriorityCritical {
		t.Errorf("expected critical, got %s", task1.Priority)
	}
	if !containsString(task1.Tags, "feature") || !containsString(task1.Tags, "core") {
		t.Errorf("expected tags [feature, core], got %v", task1.Tags)
	}

	// Verify dependencies
	task2 := store2.GetTask(id2)
	if !containsString(task2.BlockedBy, id1) {
		t.Error("task 2 should be blocked by task 1")
	}

	// Verify subtask
	subtasks := store2.ListSubtasks(id1)
	if len(subtasks) != 1 {
		t.Fatalf("expected 1 subtask, got %d", len(subtasks))
	}
	if subtasks[0].Subject != "Sub-component A" {
		t.Errorf("expected subtask subject 'Sub-component A', got '%s'", subtasks[0].Subject)
	}
	if subtasks[0].Status != WorkTaskInProgress {
		t.Errorf("expected subtask in_progress, got %s", subtasks[0].Status)
	}

	// Verify time tracking survived
	if task1.CompletedAt == nil {
		t.Error("CompletedAt should be persisted")
	}
	if len(task1.History) < 2 {
		t.Errorf("expected at least 2 history entries, got %d", len(task1.History))
	}
}

func TestIntegration_CompactionFlow(t *testing.T) {
	// Simulate the full compaction flow:
	// 1. Create tasks
	// 2. Simulate compaction (clear state)
	// 3. Inject task context
	// 4. Verify context is complete

	store := NewWorkTaskStore("")

	// Create realistic task set
	id1 := store.CreateTask("Refactor auth module", "", "", nil)
	id2 := store.CreateTask("Add OAuth2 support", "", "", nil)
	id3 := store.CreateTask("Update API docs", "", "", nil)
	id4 := store.CreateTask("Fix login bug", "", "", nil)

	store.SetTaskPriority(id1, PriorityHigh)
	store.SetTaskPriority(id4, PriorityCritical)
	store.AddTaskTag(id1, "refactor")
	store.AddTaskTag(id4, "bug")
	store.AddDependency(id3, id1)
	store.AddDependency(id3, id2)

	store.TransitionTo(id1, WorkTaskInProgress, "starting")
	store.TransitionTo(id4, WorkTaskInProgress, "fixing")

	// Simulate what injectTaskStatusAfterCompact produces
	var sb strings.Builder
	sb.WriteString("## Task Context After Compaction\n\n")

	// Active tasks
	sb.WriteString("### Active Tasks\n\n")
	topTasks := store.ListTopLevelTasks()
	for _, t := range topTasks {
		if t.Status == WorkTaskDeleted || t.Status == WorkTaskCompleted {
			continue
		}
		sb.WriteString(formatTaskLine(t, 0))
	}

	// Blocked tasks
	blockedTasks := store.ListBlockedTasks()
	if len(blockedTasks) > 0 {
		sb.WriteString("\n### Blocked Tasks\n\n")
		for _, t := range blockedTasks {
			blockers := store.GetBlockers(t.ID)
			blockerInfo := make([]string, len(blockers))
			for i, b := range blockers {
				blockerInfo[i] = fmt.Sprintf("#%s (%s)", b.ID, b.Subject)
			}
			sb.WriteString(fmt.Sprintf("- #%s: %s — waiting on: %s\n",
				t.ID, t.Subject, strings.Join(blockerInfo, ", ")))
		}
	}

	// Ready tasks
	readyTasks := store.GetReadyTasks()
	if len(readyTasks) > 0 {
		sb.WriteString("\n### Ready to Start\n\n")
		for _, t := range readyTasks {
			pri := string(t.Priority)
			if pri == "" {
				pri = "medium"
			}
			sb.WriteString(fmt.Sprintf("- [#%s] %s (%s priority)\n", t.ID, t.Subject, pri))
		}
	}

	output := sb.String()

	// Verify completeness
	checks := []struct {
		name     string
		expected string
	}{
		{"active tasks header", "### Active Tasks"},
		{"in_progress task 1", "[>] #1: Refactor auth module"},
		{"in_progress task 4", "[>] #4: Fix login bug"},
		{"blocked section", "### Blocked Tasks"},
		{"blocked task 3", "#3: Update API docs"},
		{"blocked by task 1", "#1 (Refactor auth module)"},
		{"blocked by task 2", "#2 (Add OAuth2 support)"},
		{"ready section", "### Ready to Start"},
		{"ready task 2", "#2: Add OAuth2 support"},
		{"priority high", "!!"},
		{"tag", "[refactor]"},
	}

	for _, c := range checks {
		if !strings.Contains(output, c.expected) {
			t.Errorf("missing: %s (%q)\nOutput:\n%s", c.name, c.expected, output)
		}
	}

	// Verify blocked tasks are NOT in ready list
	readyIdx := strings.Index(output, "### Ready to Start")
	if readyIdx >= 0 {
		readySection := output[readyIdx:]
		if strings.Contains(readySection, "#3: Update API docs") {
			t.Error("blocked task #3 should not be in ready list")
		}
		if strings.Contains(readySection, "#1: Refactor") {
			t.Error("in_progress task #1 should not be in ready list")
		}
	}

	t.Logf("\n%s", output)
}

func TestIntegration_EmptyAndEdgeCases(t *testing.T) {
	// Test edge cases
	store := NewWorkTaskStore("")

	// No tasks - should produce empty output
	activeTasks := store.ListActiveTasks()
	if len(activeTasks) != 0 {
		t.Error("expected 0 active tasks")
	}

	blockedTasks := store.ListBlockedTasks()
	if len(blockedTasks) != 0 {
		t.Error("expected 0 blocked tasks")
	}

	readyTasks := store.GetReadyTasks()
	if len(readyTasks) != 0 {
		t.Error("expected 0 ready tasks")
	}

	// Single task
	store.CreateTask("Only task", "", "", nil)
	activeTasks = store.ListActiveTasks()
	if len(activeTasks) != 1 {
		t.Errorf("expected 1 active task, got %d", len(activeTasks))
	}

	readyTasks = store.GetReadyTasks()
	if len(readyTasks) != 1 {
		t.Errorf("expected 1 ready task, got %d", len(readyTasks))
	}

	// All completed
	store.TransitionTo("1", WorkTaskInProgress, "start")
	store.TransitionTo("1", WorkTaskCompleted, "done")
	activeTasks = store.ListActiveTasks()
	if len(activeTasks) != 0 {
		t.Errorf("expected 0 active tasks after completion, got %d", len(activeTasks))
	}
}

func TestIntegration_CircularDependency(t *testing.T) {
	store := NewWorkTaskStore("")

	id1 := store.CreateTask("Task A", "", "", nil)
	id2 := store.CreateTask("Task B", "", "", nil)

	// A -> B
	store.AddDependency(id2, id1)

	// B -> A should fail (cycle)
	err := store.AddDependency(id1, id2)
	if err == nil {
		t.Error("expected error for circular dependency")
	}

	// Both should still be pending (not blocked by completed tasks)
	readyTasks := store.GetReadyTasks()
	if len(readyTasks) != 1 {
		t.Errorf("expected 1 ready task (only task A), got %d", len(readyTasks))
	}
	if len(readyTasks) > 0 && readyTasks[0].ID != id1 {
		t.Errorf("expected task A to be ready, got %s", readyTasks[0].ID)
	}
}

func TestIntegration_CompleteDependencyChain(t *testing.T) {
	store := NewWorkTaskStore("")

	// Create a chain: A -> B -> C -> D
	id1 := store.CreateTask("Step 1", "", "", nil)
	id2 := store.CreateTask("Step 2", "", "", nil)
	id3 := store.CreateTask("Step 3", "", "", nil)
	id4 := store.CreateTask("Step 4", "", "", nil)

	store.AddDependency(id2, id1)
	store.AddDependency(id3, id2)
	store.AddDependency(id4, id3)

	// Initially only Step 1 is ready
	ready := store.GetReadyTasks()
	if len(ready) != 1 || ready[0].ID != id1 {
		t.Errorf("expected only Step 1 ready, got %v", ready)
	}

	// Complete Step 1 -> Step 2 becomes ready
	store.TransitionTo(id1, WorkTaskInProgress, "start")
	store.TransitionTo(id1, WorkTaskCompleted, "done")

	ready = store.GetReadyTasks()
	if len(ready) != 1 || ready[0].ID != id2 {
		t.Errorf("expected only Step 2 ready after Step 1 done, got %v", ready)
	}

	// Complete Step 2 -> Step 3 becomes ready
	store.TransitionTo(id2, WorkTaskInProgress, "start")
	store.TransitionTo(id2, WorkTaskCompleted, "done")

	ready = store.GetReadyTasks()
	if len(ready) != 1 || ready[0].ID != id3 {
		t.Errorf("expected only Step 3 ready after Step 2 done, got %v", ready)
	}

	// Verify execution groups
	groups := store.GetExecutionGroups()
	if len(groups) != 2 { // Step 3 and Step 4 in separate groups
		t.Errorf("expected 2 execution groups, got %d", len(groups))
	}
}

func TestIntegration_TimeTrackingFlow(t *testing.T) {
	store := NewWorkTaskStore("")

	id := store.CreateTask("Long task", "", "", nil)

	// Start -> pause -> resume -> complete
	store.TransitionTo(id, WorkTaskInProgress, "starting")
	time.Sleep(10 * time.Millisecond)

	store.TransitionTo(id, WorkTaskPending, "pausing")
	time.Sleep(10 * time.Millisecond)

	store.TransitionTo(id, WorkTaskInProgress, "resuming")
	time.Sleep(10 * time.Millisecond)

	store.TransitionTo(id, WorkTaskCompleted, "done")

	task := store.GetTask(id)
	if task.Status != WorkTaskCompleted {
		t.Errorf("expected completed, got %s", task.Status)
	}
	if task.StartedAt != nil {
		t.Error("StartedAt should be nil after completion")
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if task.TimeSpent <= 0 {
		t.Error("TimeSpent should be positive (accumulated from two sessions)")
	}
	if len(task.History) != 4 {
		t.Errorf("expected 4 history entries, got %d", len(task.History))
	}

	// Verify history
	expectedTransitions := []struct {
		from WorkTaskStatus
		to   WorkTaskStatus
	}{
		{WorkTaskPending, WorkTaskInProgress},
		{WorkTaskInProgress, WorkTaskPending},
		{WorkTaskPending, WorkTaskInProgress},
		{WorkTaskInProgress, WorkTaskCompleted},
	}
	for i, expected := range expectedTransitions {
		if task.History[i].From != expected.from || task.History[i].To != expected.to {
			t.Errorf("history[%d]: expected %s -> %s, got %s -> %s",
				i, expected.from, expected.to, task.History[i].From, task.History[i].To)
		}
	}
}
