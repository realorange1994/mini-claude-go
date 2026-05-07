package tools

import (
	"fmt"
	"strings"
	"time"
)

// AgentListTool lists all running/pending background agents.
type AgentListTool struct {
	Store *AgentTaskStore
}

func (t *AgentListTool) Name() string { return "agent_list" }

func (t *AgentListTool) Description() string {
	return "List all background sub-agent tasks with their ID, description, status, and model. " +
		"Use this to check on running agents or review completed/failed ones."
}

func (t *AgentListTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type":        "string",
				"description": "Filter by status: pending, running, completed, failed, killed",
				"enum":        []string{"pending", "running", "completed", "failed", "killed"},
			},
		},
	}
}

func (t *AgentListTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *AgentListTool) Execute(params map[string]any) ToolResult {
	if t.Store == nil {
		return ToolResultError("agent task store not initialized")
	}

	statusFilter, _ := params["status"].(string)

	var tasks []*AgentTask
	if statusFilter != "" {
		tasks = t.Store.ListByStatus(TaskStatus(statusFilter))
	} else {
		tasks = t.Store.List()
	}

	if len(tasks) == 0 {
		if statusFilter != "" {
			return ToolResultOK(fmt.Sprintf("No agents with status '%s'.", statusFilter))
		}
		return ToolResultOK("No agents found.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-10s %-12s %-30s %-10s %-20s\n",
		"ID", "Status", "Description", "Model", "Started"))
	sb.WriteString(strings.Repeat("-", 85))
	sb.WriteString("\n")

	for _, task := range tasks {
		desc := task.Description
		if len(desc) > 28 {
			desc = desc[:25] + "..."
		}
		model := task.Model
		if model == "" {
			model = "-"
		}
		sb.WriteString(fmt.Sprintf("%-10s %-12s %-30s %-10s %-20s\n",
			task.ID, task.Status, desc, model,
			task.StartTime.Format("15:04:05")))
	}

	sb.WriteString(fmt.Sprintf("\n%d agent(s) total", len(tasks)))
	return ToolResultOK(sb.String())
}

// AgentGetTool gets details of a specific agent including recent output.
type AgentGetTool struct {
	Store *AgentTaskStore
}

func (t *AgentGetTool) Name() string { return "agent_get" }

func (t *AgentGetTool) Description() string {
	return "Get details of a specific background agent by ID, including status, description, model, and captured output."
}

func (t *AgentGetTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"agent_id"},
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "The ID of the agent to inspect",
			},
			"tail": map[string]any{
				"type":        "number",
				"description": "Number of output lines to show from the end (default: 50, max: 200)",
			},
		},
	}
}

func (t *AgentGetTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *AgentGetTool) Execute(params map[string]any) ToolResult {
	if t.Store == nil {
		return ToolResultError("agent task store not initialized")
	}

	agentID, _ := params["agent_id"].(string)
	if agentID == "" {
		return ToolResultError("agent_id is required")
	}

	task := t.Store.Get(agentID)
	if task == nil {
		return ToolResultError(fmt.Sprintf("Agent %s not found", agentID))
	}

	// Mark task as notified so post-compact recovery knows the model
	// has already seen this agent's results.
	task.Notified = true

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Agent: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("  Status:        %s\n", task.Status))
	sb.WriteString(fmt.Sprintf("  Type:          %s\n", task.Type))
	sb.WriteString(fmt.Sprintf("  Description:   %s\n", task.Description))
	sb.WriteString(fmt.Sprintf("  SubagentType:  %s\n", task.SubagentType))
	sb.WriteString(fmt.Sprintf("  Model:         %s\n", task.Model))
	sb.WriteString(fmt.Sprintf("  Started:       %s\n", task.StartTime.Format(time.RFC3339)))
	if !task.EndTime.IsZero() {
		sb.WriteString(fmt.Sprintf("  Ended:         %s\n", task.EndTime.Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("  Duration:      %s\n", task.EndTime.Sub(task.StartTime).Round(1)))
	}
	if task.ToolsUsed > 0 {
		sb.WriteString(fmt.Sprintf("  Tools Used:    %d\n", task.ToolsUsed))
	}
	if task.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("  Duration (ms): %d\n", task.DurationMs))
	}
	if task.TranscriptPath != "" {
		sb.WriteString(fmt.Sprintf("  Transcript:    %s\n", task.TranscriptPath))
	}

	// Show output
	output := task.GetOutput()
	if output != "" {
		tailLines, _ := params["tail"].(float64)
		if tailLines <= 0 {
			tailLines = 50
		}
		if tailLines > 200 {
			tailLines = 200
		}

		lines := strings.Split(output, "\n")
		var showLines []string
		if len(lines) > int(tailLines) {
			skipped := len(lines) - int(tailLines)
			showLines = lines[len(lines)-int(tailLines):]
			sb.WriteString(fmt.Sprintf("\n  Output (last %d of %d lines):\n", int(tailLines), len(lines)))
			sb.WriteString(fmt.Sprintf("  ... (%d earlier lines omitted) ...\n", skipped))
		} else {
			showLines = lines
			sb.WriteString(fmt.Sprintf("\n  Output (%d lines):\n", len(lines)))
		}
		for _, line := range showLines {
			sb.WriteString("  " + line + "\n")
		}
	} else {
		sb.WriteString("\n  Output: (none yet)\n")
	}

	return ToolResultOK(sb.String())
}

// AgentKillTool stops a running background agent by ID.
type AgentKillTool struct {
	Store *AgentTaskStore
}

func (t *AgentKillTool) Name() string { return "agent_kill" }

func (t *AgentKillTool) Description() string {
	return "Kill a running background agent by its ID. The agent's context is cancelled and it is marked as killed."
}

func (t *AgentKillTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"agent_id"},
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "The ID of the agent to kill",
			},
		},
	}
}

func (t *AgentKillTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *AgentKillTool) Execute(params map[string]any) ToolResult {
	if t.Store == nil {
		return ToolResultError("agent task store not initialized")
	}

	agentID, _ := params["agent_id"].(string)
	if agentID == "" {
		return ToolResultError("agent_id is required")
	}

	task := t.Store.Get(agentID)
	if task == nil {
		return ToolResultError(fmt.Sprintf("Agent %s not found", agentID))
	}

	if task.IsTerminal() {
		return ToolResultError(fmt.Sprintf("Agent %s is not running (status: %s)", agentID, task.Status))
	}

	if t.Store.Kill(agentID) {
		return ToolResultOK(fmt.Sprintf("Agent %s has been killed.", agentID))
	}
	return ToolResultError(fmt.Sprintf("Failed to kill agent %s", agentID))
}

// time import needed for RFC3339 formatting
var _ = fmt.Sprintf // suppress unused import
