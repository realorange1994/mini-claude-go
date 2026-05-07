package tools

import (
	"testing"

	"miniclaudecode-go/mcp"
)

func TestListMCPToolsNoManager(t *testing.T) {
	tool := &ListMCPTools{Manager: nil}
	result := tool.Execute(map[string]any{})

	if !result.IsError {
		t.Error("expected error when manager is nil")
	}
}

func TestListMCPToolsEmpty(t *testing.T) {
	tool := &ListMCPTools{Manager: mcp.NewManager()}
	result := tool.Execute(map[string]any{})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "No MCP servers configured." {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestListMCPToolsInputSchema(t *testing.T) {
	tool := &ListMCPTools{}
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("expected type to be object")
	}
	if tool.Name() != "list_mcp_tools" {
		t.Errorf("unexpected name: %s", tool.Name())
	}
}

func TestMCPToolCallerNoManager(t *testing.T) {
	tool := &MCPToolCaller{Manager: nil}
	result := tool.Execute(map[string]any{})

	if !result.IsError {
		t.Error("expected error when manager is nil")
	}
}

func TestMCPToolCallerMissingTool(t *testing.T) {
	tool := &MCPToolCaller{Manager: mcp.NewManager()}
	result := tool.Execute(map[string]any{})

	if !result.IsError {
		t.Error("expected error when tool is missing")
	}
}

func TestMCPToolCallerToolNotFound(t *testing.T) {
	tool := &MCPToolCaller{Manager: mcp.NewManager()}
	result := tool.Execute(map[string]any{"tool": "nonexistent"})

	if !result.IsError {
		t.Error("expected error when tool not found")
	}
}

func TestMCPToolCallerInputSchema(t *testing.T) {
	tool := &MCPToolCaller{}
	schema := tool.InputSchema()

	required, ok := schema["required"].([]string)
	if !ok {
		t.Error("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "tool" {
		t.Errorf("expected required to be ['tool'], got %v", required)
	}
	if tool.Name() != "mcp_call_tool" {
		t.Errorf("unexpected name: %s", tool.Name())
	}
}

func TestMCPServerStatusNoManager(t *testing.T) {
	tool := &MCPServerStatus{Manager: nil}
	result := tool.Execute(map[string]any{})

	if !result.IsError {
		t.Error("expected error when manager is nil")
	}
}

func TestMCPServerStatusEmpty(t *testing.T) {
	tool := &MCPServerStatus{Manager: mcp.NewManager()}
	result := tool.Execute(map[string]any{})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "No MCP servers configured." {
		t.Errorf("unexpected output: %s", result.Output)
	}
}

func TestMCPServerStatusInputSchema(t *testing.T) {
	tool := &MCPServerStatus{}
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Error("expected type to be object")
	}
	if tool.Name() != "mcp_server_status" {
		t.Errorf("unexpected name: %s", tool.Name())
	}
}

func TestMCPToolsCheckPermissions(t *testing.T) {
	// All MCP tools should have empty permission checks
	tools := []Tool{
		&ListMCPTools{},
		&MCPToolCaller{},
		&MCPServerStatus{},
	}

	for _, tool := range tools {
		result := tool.CheckPermissions(map[string]any{})
		if result.Behavior != PermissionPassthrough {
			t.Errorf("%s: expected empty permission check, got %+v", tool.Name(), result)
		}
	}
}
