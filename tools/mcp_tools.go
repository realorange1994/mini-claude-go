package tools

import (
	"context"
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

func (*ListMCPTools) CheckPermissions(params map[string]any) string { return "" }

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

// MCPToolCaller dynamically calls tools on MCP servers.
type MCPToolCaller struct {
	Manager *mcp.Manager
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

func (*MCPToolCaller) CheckPermissions(params map[string]any) string { return "" }

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result *mcp.ToolResult
	var err error

	if server != "" {
		result, err = t.Manager.CallToolWithServer(ctx, server, toolName, callArgs)
	} else {
		result, err = t.Manager.CallTool(ctx, toolName, callArgs)
	}

	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}

	var parts []string
	for _, block := range result.Content {
		if block.Type == "text" {
			parts = append(parts, block.Text)
		}
	}

	output := strings.Join(parts, "\n")
	if result.IsError {
		return ToolResult{Output: output, IsError: true}
	}
	return ToolResult{Output: output}
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

func (*MCPServerStatus) CheckPermissions(params map[string]any) string { return "" }

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
