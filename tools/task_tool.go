package tools

import (
	"fmt"
	"sort"
	"strings"
)

// WorkTaskInfo represents a work task for display purposes.
type WorkTaskInfo struct {
	ID          string
	Subject     string
	Description string
	ActiveForm  string
	Status      string
	Owner       string
	Metadata    map[string]any
	Blocks      []string
	BlockedBy   []string
	CreatedAt   string
	UpdatedAt   string
}

// WorkTaskCreateFunc is the callback to create a work task.
type WorkTaskCreateFunc func(subject, description, activeForm string, metadata map[string]any) string

// WorkTaskListFunc is the callback to list work tasks.
type WorkTaskListFunc func() []WorkTaskInfo

// WorkTaskGetFunc is the callback to get a work task by ID.
type WorkTaskGetFunc func(id string) (*WorkTaskInfo, bool)

// WorkTaskUpdateFunc is the callback to update a work task.
type WorkTaskUpdateFunc func(id string, updates map[string]any) error

// TaskCreateTool creates a new work task.
type TaskCreateTool struct {
	CreateFunc WorkTaskCreateFunc
}

func (t *TaskCreateTool) Name() string { return "task_create" }

func (t *TaskCreateTool) Description() string {
	return "Create a structured task to track work items. Use for complex multi-step tasks to organize progress."
}

func (t *TaskCreateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"subject", "description"},
		"properties": map[string]any{
			"subject": map[string]any{
				"type":        "string",
				"description": "Brief title for the task (imperative form, e.g., 'Fix authentication bug')",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Detailed description of what needs to be done",
			},
			"active_form": map[string]any{
				"type":        "string",
				"description": "Present continuous form shown in spinner (e.g., 'Fixing authentication bug')",
			},
			"metadata": map[string]any{
				"type":        "object",
				"description": "Optional arbitrary metadata to attach to the task",
			},
		},
	}
}

func (t *TaskCreateTool) CheckPermissions(params map[string]any) string { return "" }

func (t *TaskCreateTool) Execute(params map[string]any) ToolResult {
	if t.CreateFunc == nil {
		return ToolResultError("task system not initialized")
	}

	subject, _ := params["subject"].(string)
	description, _ := params["description"].(string)

	if subject == "" {
		return ToolResultError("subject is required")
	}
	if description == "" {
		return ToolResultError("description is required")
	}

	activeForm, _ := params["active_form"].(string)
	var metadata map[string]any
	if m, ok := params["metadata"].(map[string]any); ok {
		metadata = m
	}

	taskID := t.CreateFunc(subject, description, activeForm, metadata)
	return ToolResultOK(fmt.Sprintf("Task #%s created successfully: %s", taskID, subject))
}

// TaskListTool lists all work tasks.
type TaskListTool struct {
	ListFunc WorkTaskListFunc
}

func (t *TaskListTool) Name() string { return "task_list" }

func (t *TaskListTool) Description() string {
	return "List all tasks. Returns a table of all tasks with their ID, subject, status, and dependencies."
}

func (t *TaskListTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *TaskListTool) CheckPermissions(params map[string]any) string { return "" }

func (t *TaskListTool) Execute(params map[string]any) ToolResult {
	if t.ListFunc == nil {
		return ToolResultError("task system not initialized")
	}

	tasks := t.ListFunc()
	if len(tasks) == 0 {
		return ToolResultOK("No tasks found.")
	}

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("%-6s %-40s %-12s %-10s %s\n", "ID", "Subject", "Status", "Owner", "Blocked By"))
	sb.WriteString(strings.Repeat("-", 80))
	sb.WriteString("\n")

	for _, task := range tasks {
		subject := task.Subject
		if len(subject) > 38 {
			subject = subject[:35] + "..."
		}
		blockedBy := strings.Join(task.BlockedBy, ", ")
		if blockedBy == "" {
			blockedBy = "-"
		}
		owner := task.Owner
		if owner == "" {
			owner = "-"
		}
		sb.WriteString(fmt.Sprintf("%-6s %-40s %-12s %-10s %s\n", "#"+task.ID, subject, task.Status, owner, blockedBy))
	}

	sb.WriteString(fmt.Sprintf("\n%d task(s) total", len(tasks)))

	return ToolResultOK(sb.String())
}

// TaskGetTool retrieves detailed information about a specific task.
type TaskGetTool struct {
	GetFunc WorkTaskGetFunc
}

func (t *TaskGetTool) Name() string { return "task_get" }

func (t *TaskGetTool) Description() string {
	return "Get details of a specific task by ID. Returns full task information including description and dependencies."
}

func (t *TaskGetTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"task_id"},
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The ID of the task to retrieve",
			},
		},
	}
}

func (t *TaskGetTool) CheckPermissions(params map[string]any) string { return "" }

func (t *TaskGetTool) Execute(params map[string]any) ToolResult {
	if t.GetFunc == nil {
		return ToolResultError("task system not initialized")
	}

	taskID, _ := params["task_id"].(string)
	if taskID == "" {
		return ToolResultError("task_id is required")
	}

	task, found := t.GetFunc(taskID)
	if !found {
		return ToolResultError(fmt.Sprintf("Task #%s not found", taskID))
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Task #%s\n", task.ID))
	sb.WriteString(fmt.Sprintf("  Subject:     %s\n", task.Subject))
	sb.WriteString(fmt.Sprintf("  Status:      %s\n", task.Status))
	sb.WriteString(fmt.Sprintf("  Description: %s\n", task.Description))

	if task.ActiveForm != "" {
		sb.WriteString(fmt.Sprintf("  Active Form: %s\n", task.ActiveForm))
	}
	if task.Owner != "" {
		sb.WriteString(fmt.Sprintf("  Owner:       %s\n", task.Owner))
	}
	if len(task.Blocks) > 0 {
		sb.WriteString(fmt.Sprintf("  Blocks:      %s\n", strings.Join(task.Blocks, ", ")))
	}
	if len(task.BlockedBy) > 0 {
		sb.WriteString(fmt.Sprintf("  Blocked By:  %s\n", strings.Join(task.BlockedBy, ", ")))
	}
	if len(task.Metadata) > 0 {
		sb.WriteString("  Metadata:\n")
		// Sort metadata keys for deterministic output
		keys := make([]string, 0, len(task.Metadata))
		for k := range task.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("    %s: %v\n", k, task.Metadata[k]))
		}
	}

	sb.WriteString(fmt.Sprintf("  Created:     %s\n", task.CreatedAt))
	sb.WriteString(fmt.Sprintf("  Updated:     %s\n", task.UpdatedAt))

	return ToolResultOK(sb.String())
}

// TaskUpdateTool updates a work task's fields.
type TaskUpdateTool struct {
	UpdateFunc WorkTaskUpdateFunc
}

func (t *TaskUpdateTool) Name() string { return "task_update" }

func (t *TaskUpdateTool) Description() string {
	return "Update a task's fields. Use to change status, assign owners, mark dependencies, or edit descriptions."
}

func (t *TaskUpdateTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"task_id"},
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The ID of the task to update",
			},
			"subject": map[string]any{
				"type":        "string",
				"description": "New subject for the task",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "New description for the task",
			},
			"active_form": map[string]any{
				"type":        "string",
				"description": "Present continuous form shown in spinner (e.g., 'Running tests')",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed", "deleted"},
				"description": "New status for the task",
			},
			"owner": map[string]any{
				"type":        "string",
				"description": "New owner for the task",
			},
			"add_blocks": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Task IDs that this task blocks",
			},
			"add_blocked_by": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Task IDs that block this task",
			},
			"metadata": map[string]any{
				"type":        "object",
				"description": "Metadata keys to merge into the task. Set a key to null to delete it.",
			},
		},
	}
}

func (t *TaskUpdateTool) CheckPermissions(params map[string]any) string { return "" }

func (t *TaskUpdateTool) Execute(params map[string]any) ToolResult {
	if t.UpdateFunc == nil {
		return ToolResultError("task system not initialized")
	}

	taskID, _ := params["task_id"].(string)
	if taskID == "" {
		return ToolResultError("task_id is required")
	}

	updates := make(map[string]any)
	updatedFields := []string{}

	// Collect all provided update fields
	if v, ok := params["subject"].(string); ok && v != "" {
		updates["subject"] = v
		updatedFields = append(updatedFields, "subject")
	}
	if v, ok := params["description"].(string); ok && v != "" {
		updates["description"] = v
		updatedFields = append(updatedFields, "description")
	}
	if v, ok := params["active_form"].(string); ok && v != "" {
		updates["activeForm"] = v
		updatedFields = append(updatedFields, "activeForm")
	}
	if v, ok := params["status"].(string); ok && v != "" {
		updates["status"] = v
		updatedFields = append(updatedFields, "status")
	}
	if v, ok := params["owner"].(string); ok && v != "" {
		updates["owner"] = v
		updatedFields = append(updatedFields, "owner")
	}
	// Coerce scalar to array for add_blocked_by
	if v, ok := params["add_blocked_by"]; ok {
		switch val := v.(type) {
		case float64:
			params["add_blocked_by"] = []any{fmt.Sprintf("%d", int(val))}
		case string:
			params["add_blocked_by"] = []any{val}
		}
	}
	// Coerce scalar to array for add_blocks
	if v, ok := params["add_blocks"]; ok {
		switch val := v.(type) {
		case float64:
			params["add_blocks"] = []any{fmt.Sprintf("%d", int(val))}
		case string:
			params["add_blocks"] = []any{val}
		}
	}

	if v, ok := params["add_blocks"].([]any); ok && len(v) > 0 {
		updates["addBlocks"] = v
		updatedFields = append(updatedFields, "blocks")
	}
	if v, ok := params["add_blocked_by"].([]any); ok && len(v) > 0 {
		updates["addBlockedBy"] = v
		updatedFields = append(updatedFields, "blockedBy")
	}
	if v, ok := params["metadata"].(map[string]any); ok {
		updates["metadata"] = v
		updatedFields = append(updatedFields, "metadata")
	}

	if len(updates) == 0 {
		return ToolResultError("no update fields provided")
	}

	err := t.UpdateFunc(taskID, updates)
	if err != nil {
		return ToolResultError(fmt.Sprintf("Failed to update task: %s", err.Error()))
	}

	return ToolResultOK(fmt.Sprintf("Updated task #%s: %s", taskID, strings.Join(updatedFields, ", ")))
}

// TaskStopTool stops a running background task by its ID.
type TaskStopTool struct {
	StopFunc func(taskID string) error
}

func (t *TaskStopTool) Name() string { return "task_stop" }
func (t *TaskStopTool) Description() string {
	return "Stop a running background task by its ID. Use this to terminate long-running or stuck processes."
}

func (t *TaskStopTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"task_id"},
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The ID of the background task to stop",
			},
		},
	}
}

func (t *TaskStopTool) CheckPermissions(params map[string]any) string { return "" }

func (t *TaskStopTool) Execute(params map[string]any) ToolResult {
	if t.StopFunc == nil {
		return ToolResultError("task stop system not initialized")
	}
	taskID, _ := params["task_id"].(string)
	if taskID == "" {
		return ToolResultError("task_id is required")
	}
	err := t.StopFunc(taskID)
	if err != nil {
		return ToolResultError(err.Error())
	}
	return ToolResultOK(fmt.Sprintf("Task %s stopped successfully", taskID))
}
