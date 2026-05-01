package tools

import (
	"fmt"
	"strings"
	"time"
)

// AgentSpawnFunc is the callback function to spawn a child agent loop.
// It returns (result, errorText, toolsUsed, durationMs).
type AgentSpawnFunc func(
	prompt string,
	subagentType string,
	model string,
	runInBackground bool,
	allowedTools []string,
	disallowedTools []string,
	inheritContext bool,
	parentMessages []map[string]any,
) (result string, errText string, toolsUsed int, durationMs int64)

// AgentTool spawns a child agent to execute a specialized task.
type AgentTool struct {
	SpawnFunc AgentSpawnFunc
}

func (t *AgentTool) Name() string { return "agent" }
func (t *AgentTool) Description() string {
	return "Launch a sub-agent to handle a complex, multi-step task autonomously. " +
		"Use this tool to delegate specialized work that can be completed independently. " +
		"Sub-agents have their own isolated conversation context and tool access. " +
		"Supports both synchronous (default) and asynchronous (run_in_background=true) execution."
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

	// Always disallow recursive agent spawning
	disallowedTools = append(disallowedTools, "agent")

	_ = description // logged by parent via transcript

	if runInBackground {
		// Async path: launch goroutine and return immediately
		agentID := fmt.Sprintf("agent-%d", time.Now().UnixNano())
		go func() {
			t.SpawnFunc(
				prompt, subagentType, model, true,
				allowedTools, disallowedTools, false, nil,
			)
		}()
		return ToolResultOK(fmt.Sprintf(
			"Agent launched in background.\n\n"+
				"agentId: %s\n"+
				"Status: async_launched\n"+
				"Description: %s",
			agentID, description,
		))
	}

	// Sync path: block until complete
	result, errText, toolsUsed, durationMs := t.SpawnFunc(
		prompt, subagentType, model, false,
		allowedTools, disallowedTools, false, nil,
	)

	if errText != "" {
		return ToolResultError(errText)
	}

	return ToolResultOK(formatAgentResult(result, toolsUsed, durationMs))
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
func formatAgentResult(result string, toolsUsed int, durationMs int64) string {
	var sb strings.Builder
	sb.WriteString(result)
	sb.WriteString("\n\n---\n")
	sb.WriteString(fmt.Sprintf("<usage>tool_uses: %d\nduration_ms: %d</usage>", toolsUsed, durationMs))
	return sb.String()
}
