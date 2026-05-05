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
	return "Update the todo list for the current session. To be used proactively and often to track progress and pending tasks. " +
		"Make sure that at least one task is in_progress at all times. " +
		"Always provide both content (imperative form, e.g. 'Fix authentication bug') and activeForm (present continuous, e.g. 'Fixing authentication bug') for each task.\n\n" +
		"## When to Use This Tool\n" +
		"Use this tool proactively in these scenarios:\n\n" +
		"1. Complex multi-step tasks - When a task requires 3 or more distinct steps or actions\n" +
		"2. Non-trivial and complex tasks - Tasks that require careful planning or multiple operations\n" +
		"3. User explicitly requests todo list - When the user directly asks you to use the todo list\n" +
		"4. User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)\n" +
		"5. After receiving new instructions - Immediately capture user requirements as todos\n" +
		"6. When you start working on a task - Mark it as in_progress BEFORE beginning work. Ideally you should only have one todo as in_progress at a time\n" +
		"7. After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation\n\n" +
		"## When NOT to Use This Tool\n\n" +
		"Skip using this tool when:\n" +
		"1. There is only a single, straightforward task\n" +
		"2. The task is trivial and tracking it provides no organizational benefit\n" +
		"3. The task can be completed in less than 3 trivial steps\n" +
		"4. The task is purely conversational or informational\n\n" +
		"## Task States and Management\n\n" +
		"1. Task States: Use these states to track progress:\n" +
		"   - pending: Task not yet started\n" +
		"   - in_progress: Currently working on (limit to ONE task at a time)\n" +
		"   - completed: Task finished successfully\n\n" +
		"2. Task Management:\n" +
		"   - Update task status in real-time as you work\n" +
		"   - Mark tasks complete IMMEDIATELY after finishing (don't batch completions)\n" +
		"   - Exactly ONE task must be in_progress at any time (not less, not more)\n" +
		"   - Complete current tasks before starting new ones\n" +
		"   - Remove tasks that are no longer relevant from the list entirely\n\n" +
		"3. Task Completion Requirements:\n" +
		"   - ONLY mark a task as completed when you have FULLY accomplished it\n" +
		"   - If you encounter errors, blockers, or cannot finish, keep the task as in_progress\n" +
		"   - When blocked, create a new task describing what needs to be resolved\n" +
		"   - Never mark a task as completed if:\n" +
		"     - Tests are failing\n" +
		"     - Implementation is partial\n" +
		"     - You encountered unresolved errors\n\n" +
		"When in doubt, use this tool. Being proactive with task management demonstrates attentiveness and ensures you complete all requirements successfully."
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

	// Matching upstream: reinforce that the model should use the todo list
	// to track progress and proceed with the current task.
	return ToolResultOK("Todos have been successfully. Ensure that you use the todo list to track your progress. Please proceed with the current tasks as applicable")
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}