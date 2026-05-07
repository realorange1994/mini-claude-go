package tools

import (
	"fmt"
	"time"
)

// TaskOutputFunc is the callback to get a task's output.
// Returns (result, errorText).
type TaskOutputFunc func(agentID string, block bool, timeout time.Duration) (string, string)

// TaskOutputTool reads the output of a background sub-agent task.
type TaskOutputTool struct {
	GetOutputFunc TaskOutputFunc
}

func (t *TaskOutputTool) Name() string { return "task_output" }
func (t *TaskOutputTool) Description() string {
	return "Retrieve the output of a background sub-agent task. " +
		"Supports blocking wait for completion with a configurable timeout. " +
		"IMPORTANT: Do NOT use block=true for background agents (run_in_background=true) — " +
		"you will be notified via task-notification when they complete. Calling task_output with block=true " +
		"will block your turn and prevent you from responding to the user. " +
		"Use block=true only when explicitly asked by the user, or for synchronous agents."
}

func (t *TaskOutputTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"task_id"},
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The agent ID of the task to read output from",
			},
			"block": map[string]any{
				"type":        "boolean",
				"description": "If true, block until the task completes (default: false)",
			},
			"timeout": map[string]any{
				"type":        "number",
				"description": "Max wait time in milliseconds (default: 30000, max: 600000)",
			},
		},
	}
}

func (t *TaskOutputTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *TaskOutputTool) Execute(params map[string]any) ToolResult {
	if t.GetOutputFunc == nil {
		return ToolResultError("task output system not initialized")
	}

	taskID, _ := params["task_id"].(string)
	if taskID == "" {
		return ToolResultError("task_id is required")
	}

	block, _ := params["block"].(bool)
	timeoutMs, _ := params["timeout"].(float64)
	if timeoutMs <= 0 {
		timeoutMs = 30000 // default: 30 seconds (matching official)
	}
	if timeoutMs > 600000 {
		timeoutMs = 600000
	}

	result, errText := t.GetOutputFunc(taskID, block, time.Duration(timeoutMs)*time.Millisecond)
	if errText != "" {
		return ToolResultError(errText)
	}
	if result == "" {
		return ToolResultOK(fmt.Sprintf("Agent %s: no output available yet", taskID))
	}
	return ToolResultOK(result)
}
