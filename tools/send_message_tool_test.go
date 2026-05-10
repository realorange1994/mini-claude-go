package tools

import (
	"strings"
	"testing"
)

// ─── SendMessageTool interface ───────────────────────────────────────────────

func TestSendMessageToolName(t *testing.T) {
	tool := &SendMessageTool{}
	if tool.Name() != "send_message" {
		t.Errorf("expected 'send_message', got %q", tool.Name())
	}
}

func TestSendMessageToolSchema(t *testing.T) {
	tool := &SendMessageTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "agent_id" {
		t.Errorf("expected required=[agent_id], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["agent_id"]; !ok {
		t.Error("schema should have agent_id property")
	}
	if _, ok := props["name"]; !ok {
		t.Error("schema should have name property")
	}
	if _, ok := props["message"]; !ok {
		t.Error("schema should have message property")
	}
	if _, ok := props["summary"]; !ok {
		t.Error("schema should have summary property")
	}
}

func TestSendMessageToolPermissions(t *testing.T) {
	tool := &SendMessageTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestSendMessageToolExecuteNoAgent(t *testing.T) {
	tool := &SendMessageTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing agent_id and name should return error")
	}
}

func TestSendMessageToolExecuteEmptyAgent(t *testing.T) {
	tool := &SendMessageTool{}
	result := tool.Execute(map[string]any{"agent_id": "", "name": ""})
	if !result.IsError {
		t.Error("empty agent_id and name should return error")
	}
}

func TestSendMessageToolExecuteStatusQuery(t *testing.T) {
	tool := &SendMessageTool{
		GetStatusFunc: func(id string) string { return "agent is running" },
	}
	result := tool.Execute(map[string]any{"agent_id": "agent-1"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if result.Output != "agent is running" {
		t.Errorf("expected 'agent is running', got %q", result.Output)
	}
}

func TestSendMessageToolExecuteStatusWithNilFunc(t *testing.T) {
	tool := &SendMessageTool{
		SendMessageFunc: nil,
		GetStatusFunc:   nil,
	}
	result := tool.Execute(map[string]any{"agent_id": "agent-1"})
	if !result.IsError {
		t.Error("nil GetStatusFunc and empty message should error (or at least not succeed)")
	}
}

func TestSendMessageToolExecuteSendMessageNilFunc(t *testing.T) {
	tool := &SendMessageTool{
		GetStatusFunc:   nil,
		SendMessageFunc: nil,
	}
	result := tool.Execute(map[string]any{"agent_id": "agent-1", "message": "hello"})
	if !result.IsError {
		t.Error("nil SendMessageFunc should return error")
	}
}

func TestSendMessageToolExecuteSuccess(t *testing.T) {
	var gotAgentID, gotMessage string
	tool := &SendMessageTool{
		SendMessageFunc: func(id, msg string) (string, string) {
			gotAgentID = id
			gotMessage = msg
			return "message delivered", ""
		},
	}
	result := tool.Execute(map[string]any{"agent_id": "abc-123", "message": "hello agent"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotAgentID != "abc-123" {
		t.Errorf("expected agent_id 'abc-123', got %q", gotAgentID)
	}
	if gotMessage != "hello agent" {
		t.Errorf("expected message 'hello agent', got %q", gotMessage)
	}
	if !strings.Contains(result.Output, "message delivered") {
		t.Errorf("result should contain response, got %q", result.Output)
	}
}

func TestSendMessageToolExecuteError(t *testing.T) {
	tool := &SendMessageTool{
		SendMessageFunc: func(id, msg string) (string, string) {
			return "", "agent not responding"
		},
	}
	result := tool.Execute(map[string]any{"agent_id": "abc-123", "message": "ping"})
	if !result.IsError {
		t.Error("error from send func should return error")
	}
}

func TestSendMessageToolExecuteByName(t *testing.T) {
	var gotAgentID string
	tool := &SendMessageTool{
		SendMessageFunc: func(id, msg string) (string, string) {
			gotAgentID = id
			return "ok", ""
		},
	}
	result := tool.Execute(map[string]any{"name": "my-agent", "message": "hello"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotAgentID != "my-agent" {
		t.Errorf("name should be used as agent_id, got %q", gotAgentID)
	}
}

func TestSendMessageToolExecuteAgentIDTakesPriority(t *testing.T) {
	var gotAgentID string
	tool := &SendMessageTool{
		SendMessageFunc: func(id, msg string) (string, string) {
			gotAgentID = id
			return "ok", ""
		},
	}
	result := tool.Execute(map[string]any{"agent_id": "real-id", "name": "my-agent", "message": "hi"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotAgentID != "real-id" {
		t.Errorf("agent_id should take priority over name, got %q", gotAgentID)
	}
}
