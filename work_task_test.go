package main

import (
	"testing"

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
	store.UpdateTask("2", map[string]any{"status": "in_progress"})
	store.UpdateTask("3", map[string]any{"status": "completed"})

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
	store.UpdateTask(id3, map[string]any{"status": "completed"})

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
	store.UpdateTask(id1, map[string]any{"status": "completed"})

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
	store.UpdateTask(id1, map[string]any{"status": "completed"})
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
	store.UpdateTask(id1, map[string]any{"status": "completed"})

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
