package main

import (
	"context"
	"fmt"

	"miniclaudecode-go/tools"
)

const defaultMaxAgeDays = 7
const maxJobs = 50

// --- CronCreateTool ---

type CronCreateTool struct{}

func (t *CronCreateTool) Name() string { return "cron_create" }

func (t *CronCreateTool) Description() string {
	return fmt.Sprintf(`Schedule a prompt to run at a future time — either recurring on a cron schedule, or once at a specific time.

Uses standard 5-field cron in the user's local timezone: minute hour day-of-month month day-of-week.
"0 9 * * *" means 9am local — no timezone conversion needed.

## One-shot tasks (recurring: false)
Fire once then auto-delete. Pin minute/hour/day-of-month/month to specific values.

## Recurring jobs (recurring: true, the default)
Fire on every cron match until deleted or auto-expired after %d days.

## Avoid :00 and :30 minute marks
When the user's request is approximate, pick a minute that is NOT 0 or 30.

## Durability
By default (durable: false) the job lives only in this session. Pass durable: true to persist to .claude/scheduled_tasks.json.

Recurring tasks auto-expire after %d days. Returns a job ID you can pass to cron_delete.`,
		defaultMaxAgeDays, defaultMaxAgeDays)
}

func (t *CronCreateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"cron", "prompt"},
		"properties": map[string]any{
			"cron": map[string]any{
				"type":        "string",
				"description": `Standard 5-field cron expression in local time: "M H DoM Mon DoW" (e.g. "*/5 * * * *" = every 5 minutes).`,
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt to enqueue at each fire time.",
			},
			"recurring": map[string]any{
				"type":        "boolean",
				"description": fmt.Sprintf("true (default) = fire on every cron match until deleted or auto-expired after %d days. false = fire once then auto-delete.", defaultMaxAgeDays),
			},
			"durable": map[string]any{
				"type":        "boolean",
				"description": "true = persist to .claude/scheduled_tasks.json and survive restarts. false (default) = in-memory only.",
			},
		},
	}
}

func (t *CronCreateTool) CheckPermissions(params map[string]any) tools.PermissionResult {
	return tools.PermissionResultPassthrough()
}

func (t *CronCreateTool) ExecuteContext(ctx context.Context, params map[string]any) tools.ToolResult {
	cron, _ := params["cron"].(string)
	prompt, _ := params["prompt"].(string)
	if cron == "" {
		return tools.ToolResult{Output: "Error: cron parameter is required", IsError: true}
	}
	if prompt == "" {
		return tools.ToolResult{Output: "Error: prompt parameter is required", IsError: true}
	}

	recurring := true
	if r, ok := params["recurring"].(bool); ok {
		recurring = r
	}
	durable := false
	if d, ok := params["durable"].(bool); ok {
		durable = d
	}

	// Validate cron
	if ParseCronExpression(cron) == nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: invalid cron expression '%s'. Expected 5 fields: M H DoM Mon DoW.", cron), IsError: true}
	}
	if nextCronRunMs(cron, nowMs()) == nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: cron expression '%s' does not match any calendar date in the next year.", cron), IsError: true}
	}

	// Check task limit
	dir := getProjectDir()
	allTasks := listAllCronTasks(dir)
	if len(allTasks) >= maxJobs {
		return tools.ToolResult{Output: fmt.Sprintf("Error: too many scheduled jobs (max %d). Cancel one first.", maxJobs), IsError: true}
	}

	id, err := addCronTask(cron, prompt, recurring, durable, "", dir)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: failed to create cron task: %v", err), IsError: true}
	}

	human := CronToHuman(cron)
	where := "Persisted to .claude/scheduled_tasks.json"
	if !durable {
		where = "Session-only (not written to disk, dies when Claude exits)"
	}

	var content string
	if recurring {
		content = fmt.Sprintf("Scheduled recurring job %s (%s). %s. Auto-expires after %d days. Use cron_delete to cancel sooner.", id, human, where, defaultMaxAgeDays)
	} else {
		content = fmt.Sprintf("Scheduled one-shot task %s (%s). %s. It will fire once then auto-delete.", id, human, where)
	}

	return tools.ToolResult{Output: content}
}

func (t *CronCreateTool) Execute(params map[string]any) tools.ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

// --- CronDeleteTool ---

type CronDeleteTool struct{}

func (t *CronDeleteTool) Name() string { return "cron_delete" }

func (t *CronDeleteTool) Description() string {
	return "Cancel a scheduled cron job by ID. Removes it from .claude/scheduled_tasks.json (durable jobs) or the in-memory session store (session-only jobs)."
}

func (t *CronDeleteTool) InputSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"id"},
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Job ID returned by cron_create.",
			},
		},
	}
}

func (t *CronDeleteTool) CheckPermissions(params map[string]any) tools.PermissionResult {
	return tools.PermissionResultPassthrough()
}

func (t *CronDeleteTool) ExecuteContext(ctx context.Context, params map[string]any) tools.ToolResult {
	id, _ := params["id"].(string)
	if id == "" {
		return tools.ToolResult{Output: "Error: id parameter is required", IsError: true}
	}

	// Validate: task exists
	dir := getProjectDir()
	allTasks := listAllCronTasks(dir)
	found := false
	for _, task := range allTasks {
		if task.ID == id {
			found = true
			break
		}
	}
	if !found {
		return tools.ToolResult{Output: fmt.Sprintf("Error: no scheduled job with id '%s'", id), IsError: true}
	}

	if err := removeCronTasks([]string{id}, dir); err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Error: failed to delete cron task: %v", err), IsError: true}
	}
	return tools.ToolResult{Output: fmt.Sprintf("Cancelled job %s.", id)}
}

func (t *CronDeleteTool) Execute(params map[string]any) tools.ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

// --- CronListTool ---

type CronListTool struct{}

func (t *CronListTool) Name() string { return "cron_list" }

func (t *CronListTool) Description() string {
	return "List scheduled cron jobs"
}

func (t *CronListTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *CronListTool) CheckPermissions(params map[string]any) tools.PermissionResult {
	return tools.PermissionResultPassthrough()
}

func (t *CronListTool) ExecuteContext(ctx context.Context, params map[string]any) tools.ToolResult {
	dir := getProjectDir()
	allTasks := listAllCronTasks(dir)
	if len(allTasks) == 0 {
		return tools.ToolResult{Output: "No scheduled tasks."}
	}

	lines := make([]string, 0, len(allTasks))
	for _, task := range allTasks {
		human := CronToHuman(task.Cron)
		recurringStr := "one-shot"
		if task.Recurring {
			recurringStr = "recurring"
		}
		durableStr := ""
		if !task.Durable {
			durableStr = " [session-only]"
		}
		prompt := task.Prompt
		if len(prompt) > 80 {
			prompt = prompt[:77] + "..."
		}
		lines = append(lines, fmt.Sprintf("%s — %s (%s)%s: %s", task.ID, human, recurringStr, durableStr, prompt))
	}

	return tools.ToolResult{Output: "## Scheduled Jobs\n\n" + joinLines(lines)}
}

func (t *CronListTool) Execute(params map[string]any) tools.ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
