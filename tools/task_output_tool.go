package tools

import (
	"fmt"
	"time"
)

// TaskOutputFunc is the callback to get a task's output.
// Returns (result, errorText).
type TaskOutputFunc func(agentID string, block bool, timeout time.Duration) (string, string)

// TaskProgressFunc is the callback to get a task's progress snapshot.
// Returns (progressText, errorText).
type TaskProgressFunc func(agentID string, lastN int) (string, string)

// TaskOutputTool reads the output or progress of a background sub-agent task.
type TaskOutputTool struct {
	GetOutputFunc   TaskOutputFunc
	GetProgressFunc TaskProgressFunc // optional, enables progress snapshot mode
}

func (t *TaskOutputTool) Name() string { return "task_output" }
func (t *TaskOutputTool) Description() string {
	return "Retrieve the output of a background sub-agent task. " +
		"Supports blocking wait for completion with a configurable timeout. " +
		"IMPORTANT: Do NOT use block=true for background agents (run_in_background=true) — " +
		"you will be notified via task-notification when they complete. Calling task_output with block=true " +
		"will block your turn and prevent you from responding to the user. " +
		"Use block=true only when explicitly asked by the user, or for synchronous agents. " +
		"Use progress=true for a lightweight progress snapshot without waiting."
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
			"progress": map[string]any{
				"type":        "boolean",
				"description": "If true, return a lightweight progress snapshot instead of full output (default: false). Includes total lines, bytes, status, and last 100 lines. Does NOT block.",
			},
			"last_n": map[string]any{
				"type":        "number",
				"description": "Number of recent lines to return when progress=true (default: 100)",
			},
		},
	}
}

func (t *TaskOutputTool) CheckPermissions(params map[string]any) PermissionResult {
	return PermissionResultPassthrough()
}

func (t *TaskOutputTool) Execute(params map[string]any) ToolResult {
	if t.GetOutputFunc == nil {
		return ToolResultError("task output system not initialized")
	}

	taskID, _ := params["task_id"].(string)
	if taskID == "" {
		return ToolResultError("task_id is required")
	}

	// Progress snapshot mode: lightweight, non-blocking
	progress, _ := params["progress"].(bool)
	if progress {
		if t.GetProgressFunc != nil {
			lastN, _ := params["last_n"].(float64)
			if lastN <= 0 {
				lastN = 100
			}
			result, errText := t.GetProgressFunc(taskID, int(lastN))
			if errText != "" {
				return ToolResultError(errText)
			}
			return ToolResultOK(result)
		}
		// Fallback: if progress func not available, return partial info
		return ToolResultOK(fmt.Sprintf("Task %s: progress tracking not available for this task type.\nUse block=false to get current output.", taskID))
	}

	// Standard mode: get output (blocking or non-blocking)
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
