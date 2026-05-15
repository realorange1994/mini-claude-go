package tools

import (
	"context"
	"fmt"
	"strings"
)

// AgentExecutionMode defines how a sub-agent is executed.
type AgentExecutionMode string

const (
	AgentModeSync  AgentExecutionMode = "sync"  // foreground, blocks until done
	AgentModeAsync AgentExecutionMode = "async" // background, returns immediately (default)
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

// AgentSpawnSyncFunc is the callback for synchronous (foreground) sub-agent execution.
// It blocks until the sub-agent completes and returns the result directly.
// Signature matches AgentSpawnFunc but runs in the calling goroutine.
type AgentSpawnSyncFunc AgentSpawnFunc

// AgentTool spawns a child agent to execute a specialized task.
type AgentTool struct {
	SpawnFunc     AgentSpawnFunc
	SpawnSyncFunc AgentSpawnSyncFunc // for sync/foreground mode
	HandleStore   *AgentHandleStore  // for named agent routing
}

func (t *AgentTool) Name() string { return "agent" }
func (t *AgentTool) Description() string {
	return `Launch a sub-agent to handle a complex, multi-step task autonomously. ` +
		`Use this tool (NOT mcp_call_tool or any MCP LLM tool) when the user wants to dispatch, delegate, or assign a task to a sub-agent. ` +
		`Sub-agents have their own isolated conversation context and tool access. ` +
		`Supports both synchronous (mode="sync") and asynchronous (mode="async", default) execution.

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

Execution modes:
- mode="async" (default): agent runs in background; returns immediately with agent ID.
- mode="sync": agent runs in foreground; blocks until complete and returns result directly.

When launching a background agent (mode="async"):
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
				"enum":        []any{"sonnet", "opus", "haiku"},
				"description": "Optional model override for this agent. Takes precedence over the agent definition's model. If omitted, inherits from the parent.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []any{"sync", "async"},
				"description": `Execution mode: "sync" blocks until the agent completes and returns its result directly; "async" (default) launches the agent in the background and returns immediately with an agent ID.`,
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Optional name for the agent, enabling routing via send_message. Must be a short alphanumeric identifier (max 32 chars, hyphens/underscores allowed).",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "DEPRECATED — use mode=\"async\" instead. This parameter is ignored.",
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
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in milliseconds (max 600000 / 10 minutes). Default: 600000 (10 minutes). For sync mode, the call blocks up to this duration. For async mode, the agent runs in background regardless.",
			},
			"worktree": map[string]any{
				"type":        "object",
				"description": "Worktree isolation settings (optional). When enabled, the sub-agent runs in a separate git worktree.",
				"properties": map[string]any{
					"enabled": map[string]any{
						"type":        "boolean",
						"description": "Enable worktree isolation for this agent.",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Custom name for the worktree (auto-generated if empty).",
					},
					"keep": map[string]any{
						"type":        "boolean",
						"description": "Keep the worktree after the agent completes (default: false, auto-removed).",
					},
				},
			},
		},
	}
}

func (t *AgentTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *AgentTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	// Check context early
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: agent timed out: %v", ctx.Err()), IsError: true}
	default:
	}

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

	// Extract mode: "sync" (foreground) or "async" (background, default)
	modeStr, _ := params["mode"].(string)
	mode := AgentExecutionMode(modeStr)
	if mode == "" {
		mode = AgentModeAsync
	}

	// Extract agent name for routing
	agentName, _ := params["name"].(string)

	// Extract max_turns — default to 200 for safety ceiling.
	maxTurns, ok := params["max_turns"].(float64)
	if !ok || maxTurns <= 0 {
		maxTurns = 200 // safety ceiling: prevents runaway agents
	}

	// Always disallow recursive agent spawning
	disallowedTools = append(disallowedTools, "agent")

	// Sync mode: run in goroutine so we can respect context cancellation
	if mode == AgentModeSync {
		type syncResult struct {
			agentID    string
			result     string
			errText    string
			toolsUsed  int
			durationMs int64
		}
		ch := make(chan syncResult, 1)
		spawnFn := t.SpawnFunc
		if t.SpawnSyncFunc != nil {
			spawnFn = AgentSpawnFunc(t.SpawnSyncFunc)
		}
		go func() {
			agentID, result, errText, _, toolsUsed, durationMs := spawnFn(
				description, prompt, subagentType, model, false,
				allowedTools, disallowedTools, inheritContext, int(maxTurns), nil,
			)
			ch <- syncResult{agentID, result, errText, toolsUsed, durationMs}
		}()
		select {
		case <-ctx.Done():
			return ToolResult{Output: fmt.Sprintf("Error: agent timed out (sync mode)"), IsError: true}
		case r := <-ch:
			if r.errText != "" {
				return ToolResultError(fmt.Sprintf("sync agent failed: %s", r.errText))
			}
			safeResult, safe := SanitizeHandoffOutput(r.result)
			if !safe {
				return ToolResultOK(fmt.Sprintf(
					"Sync agent completed (handoff filtered).\n\n"+
						"agentId: %s\n"+
						"Status: completed (filtered)\n"+
						"Description: %s\n"+
						"Duration: %dms, Tools used: %d\n\n"+
						"%s",
					r.agentID, description, r.durationMs, r.toolsUsed, safeResult,
				))
			}
			return ToolResultOK(formatAgentResult(safeResult, r.agentID, subagentType, r.toolsUsed, r.durationMs, false))
		}
	}

	// Async mode (default): run in background, return immediately with agent ID
	// Register agent name if provided
	if agentName != "" && t.HandleStore != nil {
		// Validate agent name
		if !isValidAgentName(agentName) {
			return ToolResultError(fmt.Sprintf(
				"invalid agent name %q: must be alphanumeric (max 32 chars, hyphens/underscores allowed)",
				agentName))
		}
	}

	agentID, _, _, outputFile, _, _ := t.SpawnFunc(
		description, prompt, subagentType, model, true,
		allowedTools, disallowedTools, inheritContext, int(maxTurns), nil,
	)

	// Register in handle store for named routing
	if agentName != "" && t.HandleStore != nil {
		done := make(chan struct{})
		t.HandleStore.Register(agentName, &AgentHandle{
			Name:   agentName,
			TaskID: agentID,
			Status: "running",
			Done:   done,
		})
	}

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

func (t *AgentTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

// isValidAgentName checks that the name is alphanumeric with hyphens/underscores, max 32 chars.
func isValidAgentName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
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
