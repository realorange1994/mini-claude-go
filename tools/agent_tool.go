package tools

import (
	"fmt"
	"strings"
)

// AgentSpawnFunc is the callback function to spawn a child agent loop.
// It returns (agentID, result, errText, outputFile, toolsUsed, durationMs).
// For async launches, result/errText are empty and outputFile is the live output file path.
// The agentID is always generated and returned first, even for async launches.
type AgentSpawnFunc func(
	description string,
	prompt string,
	subagentType string,
	model string,
	runInBackground bool,
	allowedTools []string,
	disallowedTools []string,
	inheritContext bool,
	maxTurns int,
	parentMessages []map[string]any,
) (agentID string, result string, errText string, outputFile string, toolsUsed int, durationMs int64)

// AgentTool spawns a child agent to execute a specialized task.
type AgentTool struct {
	SpawnFunc AgentSpawnFunc
}

func (t *AgentTool) Name() string { return "agent" }
func (t *AgentTool) Description() string {
	return `Launch a sub-agent to handle a complex, multi-step task autonomously. ` +
		`Use this tool (NOT mcp_call_tool or any MCP LLM tool) when the user wants to dispatch, delegate, or assign a task to a sub-agent. ` +
		`Sub-agents have their own isolated conversation context and tool access. ` +
		`Supports both synchronous (default) and asynchronous (run_in_background=true) execution.

When NOT to use the Agent tool:
- If you want to read a specific file path → use file_read instead
- If you are searching for a specific class/function definition → use grep instead
- If you are searching within a specific file → use file_read instead
- If the task is simple and can be done directly → do it yourself
- If you need to run a single shell command → use exec instead

When TO use the Agent tool:
- Complex multi-step research tasks requiring independent exploration
- Multiple independent subtasks that can run in parallel
- Full codebase-wide investigations (use Agent with a specific goal)
- Tasks that require specialized sub-context that would benefit from fork mode (inherit parent context)

The Agent tool creates autonomous sub-agents with their own context and tool access. Each sub-agent runs independently and returns results when complete.

When launching a background agent (run_in_background=true):
- The agent runs asynchronously; you will receive a notification when it completes.
- Do NOT call task_output, Read, or Bash to check the agent's progress — this blocks your turn and defeats the purpose of background execution.
- After launching, you know nothing about what the agent found. Never fabricate or predict agent results.
- Instead, acknowledge the launch, end your response, and the notification will arrive in a separate turn.
- If the user asks about a running agent before it completes, tell them it's still running — give status, not a guess.`
}

func (t *AgentTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"description", "prompt"},
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of the task",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "The type of specialized agent to use for this task (optional). Leave blank for general-purpose.",
			},
			"model": map[string]any{
				"type":        "string",
				"enum":        []string{"sonnet", "opus", "haiku"},
				"description": "Optional model override for this agent. Takes precedence over the agent definition's model. If omitted, inherits from the parent.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "DEPRECATED — sub-agents always run in background. This parameter is ignored.",
			},
			"allowed_tools": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Explicit whitelist of tools the agent can use (optional). Use [\"*\"] for all tools.",
			},
			"disallowed_tools": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Tools the agent cannot use (optional). The 'agent' tool is always disallowed.",
			},
			"inherit_context": map[string]any{
				"type":        "boolean",
				"description": "Fork mode: inherit the parent's conversation history (optional, default false). When true, the sub-agent sees the parent's full conversation context.",
			},
			"max_turns": map[string]any{
				"type":        "integer",
				"description": "Maximum number of turns the sub-agent can execute before being forcibly stopped (optional, default 200). A turn is one user/assistant exchange. Set a reasonable limit to prevent runaway agents.",
			},
		},
	}
}

func (t *AgentTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *AgentTool) Execute(params map[string]any) ToolResult {
	if t.SpawnFunc == nil {
		return ToolResultError("agent system not initialized")
	}

	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return ToolResultError("prompt is required and must be a non-empty string")
	}

	description, _ := params["description"].(string)
	subagentType, _ := params["subagent_type"].(string)
	model, _ := params["model"].(string)

	allowedTools := extractStringList(params["allowed_tools"])
	disallowedTools := extractStringList(params["disallowed_tools"])
	inheritContext, _ := params["inherit_context"].(bool)

	// Extract max_turns — default to 200 for safety ceiling.
	maxTurns, ok := params["max_turns"].(float64)
	if !ok || maxTurns <= 0 {
		maxTurns = 200 // safety ceiling: prevents runaway agents
	}

	// Always disallow recursive agent spawning
	disallowedTools = append(disallowedTools, "agent")

	// Sub-agents always run in background — they must not block the main REPL.
	// This matches Claude Code's behavior where all agent spawns are async.
	agentID, _, _, outputFile, _, _ := t.SpawnFunc(
		description, prompt, subagentType, model, true,
		allowedTools, disallowedTools, inheritContext, int(maxTurns), nil,
	)
	return ToolResultOK(fmt.Sprintf(
		"Agent launched in background.\n\n"+
			"agentId: %s\n"+
			"Status: async_launched\n"+
			"output_file: %s\n"+
			"Description: %s\n\n"+
			"The agent is working in the background. You will be notified automatically when it completes.\n"+
			"Do NOT call task_output to wait for this agent — it will block your turn and prevent you from responding to the user.\n"+
			"Do not duplicate this agent's work — avoid working with the same files or topics it is using.\n"+
			"Briefly tell the user what you launched, then end your response. The notification will arrive in a separate turn.",
		agentID, outputFile, description,
	))
}

// extractStringList converts an interface{} (from JSON array) to []string.
func extractStringList(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// formatAgentResult formats a sub-agent's output with usage metadata.
// When skipUsage is true, only the result text is returned (used for explore/plan agents).
// agentID and agentType are included in the output footer for traceability.
func formatAgentResult(result string, agentID string, agentType string, toolsUsed int, durationMs int64, skipUsage bool) string {
	if skipUsage {
		return result
	}
	var sb strings.Builder
	sb.WriteString(result)
	sb.WriteString("\n\n---\n")
	if agentID != "" {
		sb.WriteString(fmt.Sprintf("agentId: %s\n", agentID))
	}
	if agentType != "" {
		sb.WriteString(fmt.Sprintf("agentType: %s\n", agentType))
	}
	sb.WriteString(fmt.Sprintf("<usage>tool_uses: %d\nduration_ms: %d</usage>", toolsUsed, durationMs))
	return sb.String()
}
