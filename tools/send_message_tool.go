package tools

import "fmt"

// SendMessageFunc is the callback for sending a message to a running sub-agent.
type SendMessageFunc func(agentID string, message string) (result string, errText string)

// GetStatusFunc is the callback for getting the status of a sub-agent.
type GetStatusFunc func(agentID string) string

// ResolveNameFunc is the callback for resolving an agent name to an agent ID.
type ResolveNameFunc func(name string) string

// SendMessageTool sends a message to a running sub-agent, or queries its status.
type SendMessageTool struct {
	SendMessageFunc SendMessageFunc
	GetStatusFunc   GetStatusFunc
	ResolveNameFunc ResolveNameFunc // for name -> agentID resolution
	HandleStore     *AgentHandleStore
	AgentStore      *AgentTaskStore
}

func (t *SendMessageTool) Name() string { return "send_message" }
func (t *SendMessageTool) Description() string {
	return "Send a message to a running sub-agent, or query its status. " +
		"Use this to continue work on a background agent, ask for progress, or retrieve results. " +
		"Agents can be addressed by ID or by name (if a name was provided when the agent was launched)."
}

func (t *SendMessageTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"agent_id"},
		"properties": map[string]any{
			"agent_id": map[string]any{
				"type":        "string",
				"description": "The agent ID to send a message to (from the agent launch result). Mutually exclusive with 'name'.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "The registered agent name to send a message to (mutually exclusive with 'agent_id').",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message to send to the agent. If empty, returns the agent's current status and result (if available).",
			},
			"summary": map[string]any{
				"type":        "string",
				"description": "Optional summary of what you are requesting or informing about (for logging purposes).",
			},
		},
	}
}

func (t *SendMessageTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *SendMessageTool) Execute(params map[string]any) ToolResult {
	agentID, _ := params["agent_id"].(string)
	name, _ := params["name"].(string)

	// Resolve agent_id from name if agent_id is not provided
	if agentID == "" && name != "" {
		// Try HandleStore first
		if t.HandleStore != nil {
			if handle, ok := t.HandleStore.Lookup(name); ok {
				agentID = handle.TaskID
			}
		}
		// Try ResolveNameFunc
		if agentID == "" && t.ResolveNameFunc != nil {
			agentID = t.ResolveNameFunc(name)
		}
		// Last resort: treat name as agent ID directly
		if agentID == "" {
			agentID = name
		}
	}

	if agentID == "" {
		return ToolResultError("either agent_id or name is required")
	}

	// Check if agent is still running before sending
	if t.AgentStore != nil {
		if task := t.AgentStore.Get(agentID); task != nil {
			if task.IsTerminal() {
				return ToolResultError(fmt.Sprintf("Agent %s is not running (status: %s)", agentID, task.Status))
			}
		}
	}

	message, _ := params["message"].(string)

	if message == "" && t.GetStatusFunc != nil {
		// Query status only
		status := t.GetStatusFunc(agentID)
		return ToolResultOK(status)
	}

	if t.SendMessageFunc == nil {
		return ToolResultError("send_message system not initialized")
	}

	result, errText := t.SendMessageFunc(agentID, message)
	if errText != "" {
		return ToolResultError(errText)
	}
	return ToolResultOK(result)
}
