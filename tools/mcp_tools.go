package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"miniclaudecode-go/mcp"
)

// ListMCPTools lists tools from registered MCP servers.
type ListMCPTools struct {
	Manager *mcp.Manager
}

func (*ListMCPTools) Name() string        { return "list_mcp_tools" }
func (*ListMCPTools) Description() string { return "List available tools from MCP servers. Optionally filter by server name or pattern." }

func (*ListMCPTools) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Filter by MCP server name.",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Filter by tool name pattern.",
			},
		},
		"required": []string{},
	}
}

func (*ListMCPTools) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *ListMCPTools) Execute(params map[string]any) ToolResult {
	if t.Manager == nil {
		return ToolResult{Output: "Error: MCP manager not available", IsError: true}
	}

	server, _ := params["server"].(string)
	pattern, _ := params["pattern"].(string)

	allTools := t.Manager.ListTools()

	if server != "" {
		var filtered []mcp.Tool
		for _, tool := range allTools {
			if strings.Contains(tool.Name, server) {
				filtered = append(filtered, tool)
			}
		}
		allTools = filtered
	}

	if pattern != "" {
		patternLower := strings.ToLower(pattern)
		var filtered []mcp.Tool
		for _, tool := range allTools {
			if strings.Contains(strings.ToLower(tool.Name), patternLower) {
				filtered = append(filtered, tool)
			}
		}
		allTools = filtered
	}

	if len(allTools) == 0 {
		servers := t.Manager.ListServers()
		if len(servers) == 0 {
			return ToolResult{Output: "No MCP servers configured."}
		}
		return ToolResult{Output: "No MCP tools found."}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("MCP Tools (%d total)", len(allTools)))
	for _, tool := range allTools {
		desc := tool.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		lines = append(lines, fmt.Sprintf("  %s", tool.Name))
		if desc != "" {
			lines = append(lines, fmt.Sprintf("    -> %s", desc))
		}
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}

// MCPTimeoutCallback is called when an MCP tool call times out.
// Returns (taskID, outputFile, errText, onDone).
// The onDone callback is invoked when the background MCP call completes.
type MCPTimeoutCallback func(toolName, server string, args map[string]any) (
	taskID, outputFile, errText string,
	onDone func(result string, isError bool),
)

// MCPToolCaller dynamically calls tools on MCP servers.
type MCPToolCaller struct {
	Manager         *mcp.Manager
	TimeoutCallback MCPTimeoutCallback
}

func (*MCPToolCaller) Name() string        { return "mcp_call_tool" }
func (*MCPToolCaller) Description() string { return "Call a tool on an MCP server. Use list_mcp_tools first to discover available tools." }

func (*MCPToolCaller) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "MCP server name (optional, auto-detected if omitted).",
			},
			"tool": map[string]any{
				"type":        "string",
				"description": "Tool name to call.",
			},
			"arguments": map[string]any{
				"type":        "object",
				"description": "Arguments to pass to the tool.",
			},
		},
		"required": []string{"tool"},
	}
}

func (*MCPToolCaller) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

type mcpCallResult struct {
	text    string
	isError bool
}

const mcpDefaultTimeoutMs = 30 * 1000 // 30 seconds

func (t *MCPToolCaller) Execute(params map[string]any) ToolResult {
	if t.Manager == nil {
		return ToolResult{Output: "Error: MCP manager not available", IsError: true}
	}

	toolName, _ := params["tool"].(string)
	if toolName == "" {
		return ToolResult{Output: "Error: tool is required", IsError: true}
	}

	server, _ := params["server"].(string)
	args, _ := params["arguments"].(map[string]any)
	callArgs := mcp.ToolCallArgs(args)

	// Use context.Background() — timeout is handled by the timer-based select below.
	// This ensures user interrupt (via explicit context cancellation) still works,
	// while timeout doesn't trigger context cancellation (which would break stdio connections).
	ctx := context.Background()

	resultCh := make(chan mcpCallResult, 1)
	resultReady := make(chan struct{})

	go func() {
		var result *mcp.ToolResult
		var err error

		if server != "" {
			result, err = t.Manager.CallToolWithServer(ctx, server, toolName, callArgs)
		} else {
			result, err = t.Manager.CallTool(ctx, toolName, callArgs)
		}

		var output string
		var isError bool
		if err != nil {
			output = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			var parts []string
			for _, block := range result.Content {
				if block.Type == "text" {
					parts = append(parts, block.Text)
				}
			}
			output = strings.Join(parts, "\n")
			isError = result.IsError
		}

		select {
		case resultCh <- mcpCallResult{text: output, isError: isError}:
		default:
		}
		close(resultReady)
	}()

	// Use a timer for timeout instead of context.WithTimeout. This way, when the
	// timeout fires we can return without cancelling the context — the MCP call
	// continues running in the background. User interrupt still kills via context.
	timer := time.NewTimer(time.Duration(mcpDefaultTimeoutMs) * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Timeout: register as background task, return task ID immediately.
		if t.TimeoutCallback != nil {
			taskID, outputFile, errText, onDone := t.TimeoutCallback(toolName, server, args)
			// Spawn goroutine to collect the result when it arrives.
			// The resultCh is still open — the background goroutine is still running.
			go func() {
				select {
				case <-resultCh:
					// Result arrived before timeout cleanup
				case <-resultReady:
					// Same as above — channel was closed after send
				}
				// Drain any remaining result
				for {
					select {
					case r := <-resultCh:
						if onDone != nil {
							onDone(r.text, r.isError)
						}
					default:
						return
					}
				}
			}()
			if errText != "" {
				return ToolResult{Output: fmt.Sprintf("Error: MCP call timed out after %dms. %s", mcpDefaultTimeoutMs/1000, errText), IsError: true}
			}
			return ToolResult{
				Output: fmt.Sprintf(
					"MCP call timed out after %ds and is continuing in the background.\n"+
						"Tool: %s\nTask ID: %s\nOutput file: %s\n"+
						"Use the task_output tool to check results when ready.",
					mcpDefaultTimeoutMs/1000, toolName, taskID, outputFile),
			}
		}
		return ToolResult{
			Output: fmt.Sprintf("Error: MCP call timed out after %ds. Use task_output later to check if it completed.", mcpDefaultTimeoutMs/1000),
			IsError: true,
		}
	case r := <-resultCh:
		return ToolResult{Output: r.text, IsError: r.isError}
	}
}

// MCPServerStatus reports MCP server connection status.
type MCPServerStatus struct {
	Manager *mcp.Manager
}

func (*MCPServerStatus) Name() string        { return "mcp_server_status" }
func (*MCPServerStatus) Description() string { return "Check the connection status of MCP servers." }

func (*MCPServerStatus) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Filter by server name.",
			},
		},
		"required": []string{},
	}
}

func (*MCPServerStatus) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *MCPServerStatus) Execute(params map[string]any) ToolResult {
	if t.Manager == nil {
		return ToolResult{Output: "Error: MCP manager not available", IsError: true}
	}

	server, _ := params["server"].(string)
	servers := t.Manager.ListServers()

	if len(servers) == 0 {
		return ToolResult{Output: "No MCP servers configured."}
	}

	var lines []string
	lines = append(lines, "MCP Server Status")

	for _, name := range servers {
		if server != "" && name != server {
			continue
		}
		status := t.Manager.GetServerStatus(name)
		icon := "[OK]"
		if status != "connected" {
			icon = "[FAIL]"
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", icon, name, status))
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}

// parseMCPToolResult parses a raw JSON-RPC response into a ToolResult.
func parseMCPToolResult(resp json.RawMessage) (*mcp.ToolResult, error) {
	if resp == nil {
		return &mcp.ToolResult{Content: []mcp.ContentBlock{}}, nil
	}
	var result mcp.ToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
