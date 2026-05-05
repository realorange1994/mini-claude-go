package tools

import (
	"fmt"
	"strings"
	"sync"
)

// TodoStatus represents the state of a todo item.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

// TodoItem represents a single task in the todo list.
type TodoItem struct {
	Content    string     `json:"content"`
	Status     TodoStatus `json:"status"`
	ActiveForm string     `json:"activeForm,omitempty"`
}

// TodoListState is the interface that TodoList implements.
type TodoListState interface {
	Update(items []TodoItem)
	BuildReminder() string
}

// TodoList holds the agent's structured task list.
// It is updated by TodoWriteTool and its content is injected into the system prompt.
type TodoList struct {
	mu                    sync.RWMutex
	Items                 []TodoItem
	turnsSinceLastWrite   int
	turnsSinceLastRemind  int
	reminderMessageShown  bool
}

const (
	todoReminderTurnsSinceWrite  = 10
	todoReminderTurnsBetweenReminders = 10
)

// NewTodoList creates an empty task list.
func NewTodoList() *TodoList {
	return &TodoList{Items: make([]TodoItem, 0)}
}

// Update replaces the entire todo list and resets the write counter.
func (t *TodoList) Update(items []TodoItem) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Items = items
	t.turnsSinceLastWrite = 0
}

// IncrementTurn increments the turn counters. Returns true if a TodoWrite
// reminder should be injected (model hasn't used TodoWrite for >=10 turns
// and hasn't been reminded for >=10 turns).
func (t *TodoList) IncrementTurn() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.turnsSinceLastWrite++
	t.turnsSinceLastRemind++
	shouldRemind := t.turnsSinceLastWrite >= todoReminderTurnsSinceWrite &&
		t.turnsSinceLastRemind >= todoReminderTurnsBetweenReminders
	if shouldRemind {
		t.turnsSinceLastRemind = 0
	}
	return shouldRemind
}

// BuildIdleReminder returns a nudge message when the model hasn't used
// TodoWrite for a while. This matches upstream's periodic todo_reminder attachment.
func (t *TodoList) BuildIdleReminder() string {
	return "The TodoWrite tool hasn't been used recently. If you're on tasks that would benefit from tracking progress, consider using the TodoWrite tool to update your task list. If your current task list is stale, update it. If you don't have a task list, create one for multi-step work."
}

// BuildReminder returns the task list formatted for injection into the system prompt.
// Returns empty string if the list is empty.
func (t *TodoList) BuildReminder() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if len(t.Items) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## Current Tasks\n")
	for _, item := range t.Items {
		icon := "\u25cb" // ○
		switch item.Status {
		case TodoInProgress:
			icon = "\u25d0" // ◐
		case TodoCompleted:
			icon = "\u25cf" // ●
		}
		active := ""
		if item.ActiveForm != "" {
			active = " (" + item.ActiveForm + ")"
		}
		sb.WriteString(fmt.Sprintf("  %s %s%s [%s]\n", icon, item.Content, active, item.Status))
	}
	return sb.String()
}

// TodoWriteTool updates the agent's structured todo list.
// The model calls this tool to create, update, or delete tasks.
// The list is injected into the system prompt as a reminder.
type TodoWriteTool struct {
	TodoList TodoListState
}

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Description() string {
	return "Update your task list. Use this to track multi-step work. " +
		"Create tasks when starting non-trivial work, update status as you progress, " +
		"mark completed when done. The list is shown in the system prompt as a reminder. " +
		"Call this tool with the full updated list — it replaces the previous list."
}

func (t *TodoWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"todos"},
		"properties": map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"content", "status"},
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "Task description in imperative form (e.g., 'Fix authentication bug')",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed"},
							"description": "Current task status",
						},
						"activeForm": map[string]any{
							"type":        "string",
							"description": "Present continuous form shown in spinner (e.g., 'Running tests')",
						},
					},
				},
				"description": "Complete list of tasks (replaces previous list)",
			},
		},
	}
}

func (t *TodoWriteTool) CheckPermissions(params map[string]any) string { return "" }

func (t *TodoWriteTool) Execute(params map[string]any) ToolResult {
	todosRaw, ok := params["todos"].([]any)
	if !ok {
		return ToolResultError("todos must be an array")
	}

	var items []TodoItem
	for _, raw := range todosRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		item := TodoItem{
			Content:    strVal(m, "content"),
			Status:     TodoStatus(strVal(m, "status")),
			ActiveForm: strVal(m, "activeForm"),
		}
		items = append(items, item)
	}

	t.TodoList.Update(items)

	// Build a concise result for the model
	var summary strings.Builder
	for _, item := range items {
		switch item.Status {
		case TodoInProgress:
			summary.WriteString("\u25d0 ")
		case TodoCompleted:
			summary.WriteString("\u25cf ")
		default:
			summary.WriteString("\u25cb ")
		}
		summary.WriteString(item.Content)
		summary.WriteString(" [")
		summary.WriteString(string(item.Status))
		summary.WriteString("]\n")
	}

	return ToolResultOK("Todo list updated:\n" + summary.String())
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}