package tools

import (
	"fmt"
	"strings"
)

// AgentSpawnFunc is the callback function to spawn a child agent loop.
// It returns (agentID, result, errorText, toolsUsed, durationMs).
// The agentID is always generated and returned first, even for async launches.
type AgentSpawnFunc func(
	prompt string,
	subagentType string,
	model string,
	runInBackground bool,
	allowedTools []string,
	disallowedTools []string,
	inheritContext bool,
	parentMessages []map[string]any,
) (agentID string, result string, errText string, toolsUsed int, durationMs int64)

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

The Agent tool creates autonomous sub-agents with their own context and tool access. Each sub-agent runs independently and returns results when complete.`
}

func (t *AgentTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"description", "prompt"},
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "Brief 3-5 word description of what the agent will do",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The complete task for the agent to perform. Be specific and include all necessary context.",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "Type of specialized agent to use (optional). Leave blank for general-purpose.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Model override for the agent (optional). Defaults to parent's model.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Run the agent in the background and return immediately (optional, default false).",
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
		},
	}
}

func (t *AgentTool) CheckPermissions(params map[string]any) string { return "" }

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
	runInBackground, _ := params["run_in_background"].(bool)

	allowedTools := extractStringList(params["allowed_tools"])
	disallowedTools := extractStringList(params["disallowed_tools"])
	inheritContext, _ := params["inherit_context"].(bool)

	// Always disallow recursive agent spawning
	disallowedTools = append(disallowedTools, "agent")

	_ = description // logged by parent via transcript

	if runInBackground {
		// Async path: SpawnFunc launches the goroutine internally and returns the agentID
		agentID, _, _, _, _ := t.SpawnFunc(
			prompt, subagentType, model, true,
			allowedTools, disallowedTools, inheritContext, nil,
		)
		return ToolResultOK(fmt.Sprintf(
			"Agent launched in background.\n\n"+
				"agentId: %s\n"+
				"Status: async_launched\n"+
				"Description: %s",
			agentID, description,
		))
	}

	// Sync path: block until complete
	agentID, result, errText, toolsUsed, durationMs := t.SpawnFunc(
		prompt, subagentType, model, false,
		allowedTools, disallowedTools, inheritContext, nil,
	)

	if errText != "" {
		return ToolResultError(errText)
	}

	// Explore and plan agents return raw results without usage trailer
	skipUsage := subagentType == "explore" || subagentType == "plan"
	return ToolResultOK(formatAgentResult(result, agentID, subagentType, toolsUsed, durationMs, skipUsage))
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
