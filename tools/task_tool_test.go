package tools

import (
	"errors"
	"testing"
)

// ─── TaskCreateTool ─────────────────────────────────────────────────────────

func TestTaskCreateToolName(t *testing.T) {
	tool := &TaskCreateTool{}
	if tool.Name() != "task_create" {
		t.Errorf("expected 'task_create', got %q", tool.Name())
	}
}

func TestTaskCreateToolSchema(t *testing.T) {
	tool := &TaskCreateTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 2 {
		t.Errorf("expected 2 required params, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["subject"]; !ok {
		t.Error("schema should have subject property")
	}
	if _, ok := props["description"]; !ok {
		t.Error("schema should have description property")
	}
	if _, ok := props["active_form"]; !ok {
		t.Error("schema should have active_form property")
	}
	if _, ok := props["metadata"]; !ok {
		t.Error("schema should have metadata property")
	}
}

func TestTaskCreateToolPermissions(t *testing.T) {
	tool := &TaskCreateTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTaskCreateToolExecuteNilCallback(t *testing.T) {
	tool := &TaskCreateTool{}
	result := tool.Execute(map[string]any{"subject": "test", "description": "desc"})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestTaskCreateToolExecuteMissingSubject(t *testing.T) {
	called := false
	tool := &TaskCreateTool{
		CreateFunc: func(s, d, af string, m map[string]any) string {
			called = true
			return "1"
		},
	}
	result := tool.Execute(map[string]any{"description": "desc"})
	if !result.IsError {
		t.Error("missing subject should return error")
	}
	if called {
		t.Error("callback should not be called")
	}
}

func TestTaskCreateToolExecuteMissingDescription(t *testing.T) {
	called := false
	tool := &TaskCreateTool{
		CreateFunc: func(s, d, af string, m map[string]any) string {
			called = true
			return "1"
		},
	}
	result := tool.Execute(map[string]any{"subject": "test"})
	if !result.IsError {
		t.Error("missing description should return error")
	}
	if called {
		t.Error("callback should not be called")
	}
}

func TestTaskCreateToolExecuteValid(t *testing.T) {
	var gotSubject, gotDesc, gotAF string
	tool := &TaskCreateTool{
		CreateFunc: func(s, d, af string, m map[string]any) string {
			gotSubject = s
			gotDesc = d
			gotAF = af
			return "42"
		},
	}
	result := tool.Execute(map[string]any{
		"subject":     "Fix bug",
		"description": "Fix the critical bug",
		"active_form": "Fixing bug",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotSubject != "Fix bug" {
		t.Errorf("expected subject 'Fix bug', got %q", gotSubject)
	}
	if gotDesc != "Fix the critical bug" {
		t.Errorf("expected description 'Fix the critical bug', got %q", gotDesc)
	}
	if gotAF != "Fixing bug" {
		t.Errorf("expected active_form 'Fixing bug', got %q", gotAF)
	}
}

func TestTaskCreateToolExecuteWithMetadata(t *testing.T) {
	var gotMeta map[string]any
	tool := &TaskCreateTool{
		CreateFunc: func(s, d, af string, m map[string]any) string {
			gotMeta = m
			return "1"
		},
	}
	meta := map[string]any{"priority": "high"}
	result := tool.Execute(map[string]any{
		"subject":   "test",
		"description": "desc",
		"metadata":  meta,
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotMeta["priority"] != "high" {
		t.Errorf("expected metadata, got %v", gotMeta)
	}
}

// ─── TaskListTool ───────────────────────────────────────────────────────────

func TestTaskListToolName(t *testing.T) {
	tool := &TaskListTool{}
	if tool.Name() != "task_list" {
		t.Errorf("expected 'task_list', got %q", tool.Name())
	}
}

func TestTaskListToolSchema(t *testing.T) {
	tool := &TaskListTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if len(props) != 0 {
		t.Logf("task_list props: %v", props)
	}
}

func TestTaskListToolPermissions(t *testing.T) {
	tool := &TaskListTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTaskListToolExecuteNilCallback(t *testing.T) {
	tool := &TaskListTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestTaskListToolExecuteEmpty(t *testing.T) {
	tool := &TaskListTool{
		ListFunc: func() []WorkTaskInfo { return nil },
	}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestTaskListToolExecuteWithTasks(t *testing.T) {
	tool := &TaskListTool{
		ListFunc: func() []WorkTaskInfo {
			return []WorkTaskInfo{
				{ID: "1", Subject: "Fix bug", Status: "in_progress", BlockedBy: []string{}},
			}
		},
	}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestTaskListToolLongSubjectTruncation(t *testing.T) {
	longSubject := "This is a very very very very very very very long subject title"
	tool := &TaskListTool{
		ListFunc: func() []WorkTaskInfo {
			return []WorkTaskInfo{
				{ID: "1", Subject: longSubject, Status: "pending", BlockedBy: []string{}},
			}
		},
	}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

// ─── TaskGetTool ────────────────────────────────────────────────────────────

func TestTaskGetToolName(t *testing.T) {
	tool := &TaskGetTool{}
	if tool.Name() != "task_get" {
		t.Errorf("expected 'task_get', got %q", tool.Name())
	}
}

func TestTaskGetToolSchema(t *testing.T) {
	tool := &TaskGetTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "task_id" {
		t.Errorf("expected required=[task_id], got %v", required)
	}
}

func TestTaskGetToolPermissions(t *testing.T) {
	tool := &TaskGetTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTaskGetToolExecuteNilCallback(t *testing.T) {
	tool := &TaskGetTool{}
	result := tool.Execute(map[string]any{"task_id": "1"})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestTaskGetToolExecuteMissingTaskID(t *testing.T) {
	tool := &TaskGetTool{
		GetFunc: func(id string) (*WorkTaskInfo, bool) { return nil, false },
	}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing task_id should return error")
	}
}

func TestTaskGetToolExecuteNotFound(t *testing.T) {
	tool := &TaskGetTool{
		GetFunc: func(id string) (*WorkTaskInfo, bool) { return nil, false },
	}
	result := tool.Execute(map[string]any{"task_id": "999"})
	if !result.IsError {
		t.Error("not found should return error")
	}
}

func TestTaskGetToolExecuteFound(t *testing.T) {
	task := &WorkTaskInfo{
		ID:          "1",
		Subject:     "Fix bug",
		Status:      "pending",
		Description: "Fix the critical bug",
		ActiveForm:  "Fixing bug",
		Owner:       "dev1",
		Blocks:      []string{"2"},
		BlockedBy:   []string{"3"},
		CreatedAt:   "2025-01-01",
		UpdatedAt:   "2025-01-02",
	}
	tool := &TaskGetTool{
		GetFunc: func(id string) (*WorkTaskInfo, bool) {
			if id == "1" {
				return task, true
			}
			return nil, false
		},
	}
	result := tool.Execute(map[string]any{"task_id": "1"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

// ─── TaskUpdateTool ─────────────────────────────────────────────────────────

func TestTaskUpdateToolName(t *testing.T) {
	tool := &TaskUpdateTool{}
	if tool.Name() != "task_update" {
		t.Errorf("expected 'task_update', got %q", tool.Name())
	}
}

func TestTaskUpdateToolSchema(t *testing.T) {
	tool := &TaskUpdateTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "task_id" {
		t.Errorf("expected required=[task_id], got %v", required)
	}
}

func TestTaskUpdateToolPermissions(t *testing.T) {
	tool := &TaskUpdateTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTaskUpdateToolExecuteNilCallback(t *testing.T) {
	tool := &TaskUpdateTool{}
	result := tool.Execute(map[string]any{"task_id": "1"})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestTaskUpdateToolExecuteMissingTaskID(t *testing.T) {
	called := false
	tool := &TaskUpdateTool{
		UpdateFunc: func(id string, u map[string]any) error { called = true; return nil },
	}
	result := tool.Execute(map[string]any{"subject": "new subject"})
	if !result.IsError {
		t.Error("missing task_id should return error")
	}
	if called {
		t.Error("callback should not be called")
	}
}

func TestTaskUpdateToolExecuteNoFields(t *testing.T) {
	tool := &TaskUpdateTool{
		UpdateFunc: func(id string, u map[string]any) error { return nil },
	}
	result := tool.Execute(map[string]any{"task_id": "1"})
	if !result.IsError {
		t.Error("no update fields should return error")
	}
}

func TestTaskUpdateToolExecuteValid(t *testing.T) {
	var gotID string
	var gotUpdates map[string]any
	tool := &TaskUpdateTool{
		UpdateFunc: func(id string, u map[string]any) error {
			gotID = id
			gotUpdates = u
			return nil
		},
	}
	result := tool.Execute(map[string]any{
		"task_id": "1",
		"status":  "in_progress",
		"owner":   "dev1",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotID != "1" {
		t.Errorf("expected task_id '1', got %q", gotID)
	}
	if gotUpdates["status"] != "in_progress" {
		t.Errorf("expected status update, got %v", gotUpdates)
	}
}

func TestTaskUpdateToolExecuteError(t *testing.T) {
	tool := &TaskUpdateTool{
		UpdateFunc: func(id string, u map[string]any) error {
			return errors.New("task not found")
		},
	}
	result := tool.Execute(map[string]any{
		"task_id": "999",
		"status":  "completed",
	})
	if !result.IsError {
		t.Error("update error should return error")
	}
}

// ─── TaskStopTool ───────────────────────────────────────────────────────────

func TestTaskStopToolName(t *testing.T) {
	tool := &TaskStopTool{}
	if tool.Name() != "task_stop" {
		t.Errorf("expected 'task_stop', got %q", tool.Name())
	}
}

func TestTaskStopToolSchema(t *testing.T) {
	tool := &TaskStopTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "task_id" {
		t.Errorf("expected required=[task_id], got %v", required)
	}
}

func TestTaskStopToolPermissions(t *testing.T) {
	tool := &TaskStopTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTaskStopToolExecuteMissingTaskID(t *testing.T) {
	tool := &TaskStopTool{
		StopFunc: func(id string) error { return nil },
	}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing task_id should return error")
	}
}

func TestTaskStopToolExecuteSuccess(t *testing.T) {
	var gotID string
	tool := &TaskStopTool{
		StopFunc: func(id string) error { gotID = id; return nil },
	}
	result := tool.Execute(map[string]any{"task_id": "abc"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotID != "abc" {
		t.Errorf("expected task_id 'abc', got %q", gotID)
	}
}
