package main

import (
	"fmt"
)

// injectTodoReminder appends a TODO reminder to tool result text if there
// are pending work tasks. This matches openclacky's inject_todo_reminder
// pattern: after every non-TodoWrite tool execution, check for pending
// tasks and remind the model to update them.
//
// This is a passive nudge — LLMs frequently forget to mark tasks as complete
// after doing the actual work. The reminder keeps the task list accurate,
// which improves the agent's own context management.
func injectTodoReminder(toolName string, result string, agent *AgentLoop) string {
	// Skip for TodoWrite/TaskCreate/TaskUpdate themselves to avoid redundancy
	if toolName == "TodoWrite" || toolName == "TaskCreate" || toolName == "TaskUpdate" ||
		toolName == "task_create" || toolName == "task_update" {
		return result
	}

	// Get pending tasks from the work task store
	if agent.workTaskStore == nil {
		return result
	}

	pending := 0
	for _, t := range agent.workTaskStore.ListTasks() {
		if t.Status == WorkTaskPending || t.Status == WorkTaskInProgress {
			pending++
		}
	}
	if pending == 0 {
		return result
	}

	reminder := fmt.Sprintf(
		"\n\n--- REMINDER: You have %d pending task(s). After completing each task, remember to mark it as completed using TodoWrite with status 'completed'. ---",
		pending,
	)

	return result + reminder
}