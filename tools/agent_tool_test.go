package tools

import (
	"strings"
	"testing"
)

// ─── AgentTool interface ────────────────────────────────────────────────────

func TestAgentToolName(t *testing.T) {
	tool := &AgentTool{}
	if tool.Name() != "agent" {
		t.Errorf("expected 'agent', got %q", tool.Name())
	}
}

func TestAgentToolSchema(t *testing.T) {
	tool := &AgentTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 2 {
		t.Errorf("expected 2 required params, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["description"]; !ok {
		t.Error("schema should have description property")
	}
	if _, ok := props["prompt"]; !ok {
		t.Error("schema should have prompt property")
	}
	if _, ok := props["subagent_type"]; !ok {
		t.Error("schema should have subagent_type property")
	}
	if _, ok := props["model"]; !ok {
		t.Error("schema should have model property")
	}
	if _, ok := props["allowed_tools"]; !ok {
		t.Error("schema should have allowed_tools property")
	}
	if _, ok := props["disallowed_tools"]; !ok {
		t.Error("schema should have disallowed_tools property")
	}
	if _, ok := props["inherit_context"]; !ok {
		t.Error("schema should have inherit_context property")
	}
	if _, ok := props["max_turns"]; !ok {
		t.Error("schema should have max_turns property")
	}
}

func TestAgentToolPermissions(t *testing.T) {
	tool := &AgentTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestAgentToolExecuteNilCallback(t *testing.T) {
	tool := &AgentTool{}
	result := tool.Execute(map[string]any{"prompt": "test", "description": "test"})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestAgentToolExecuteNoPrompt(t *testing.T) {
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			return "id", "result", "", "", 0, 0
		},
	}
	result := tool.Execute(map[string]any{"description": "test"})
	if !result.IsError {
		t.Error("missing prompt should return error")
	}
}

func TestAgentToolExecuteValid(t *testing.T) {
	var gotPrompt, gotDesc string
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			gotDesc = desc
			gotPrompt = prompt
			return "agent-123", "result text", "", "output.log", 3, 1500
		},
	}
	result := tool.Execute(map[string]any{
		"prompt":       "Find all bugs",
		"description":  "Bug finder",
		"subagent_type": "general-purpose",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if gotPrompt != "Find all bugs" {
		t.Errorf("expected prompt 'Find all bugs', got %q", gotPrompt)
	}
	if gotDesc != "Bug finder" {
		t.Errorf("expected desc 'Bug finder', got %q", gotDesc)
	}
	if !strings.Contains(result.Output, "agent-123") {
		t.Error("result should contain agent ID")
	}
}

func TestAgentToolExecuteDisallowsAgent(t *testing.T) {
	var gotDisallowed []string
	tool := &AgentTool{
		SpawnFunc: func(desc, prompt, subagent, model string, bg bool, allowed, disallowed []string, inherit bool, maxTurns int, parentMsg []map[string]any) (string, string, string, string, int, int64) {
			gotDisallowed = disallowed
			return "id", "result", "", "", 0, 0
		},
	}
	tool.Execute(map[string]any{"prompt": "test", "description": "test"})
	found := false
	for _, d := range gotDisallowed {
		if d == "agent" {
			found = true
		}
	}
	if !found {
		t.Error("agent tool should always be in disallowed list")
	}
}

// ─── extractStringList ──────────────────────────────────────────────────────

func TestExtractStringListNil(t *testing.T) {
	result := extractStringList(nil)
	if result != nil {
		t.Errorf("nil should return nil, got %v", result)
	}
}

func TestExtractStringListEmptyArray(t *testing.T) {
	result := extractStringList([]any{})
	if len(result) != 0 {
		t.Errorf("empty array should return empty slice, got %v", result)
	}
}

func TestExtractStringListStrings(t *testing.T) {
	result := extractStringList([]any{"a", "b", "c"})
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("expected [a,b,c], got %v", result)
	}
}

func TestExtractStringListMixedTypes(t *testing.T) {
	result := extractStringList([]any{"a", 42, "b", true, "c"})
	if len(result) != 3 {
		t.Errorf("should only extract strings, got %d items: %v", len(result), result)
	}
}

func TestExtractStringListNonArray(t *testing.T) {
	result := extractStringList("not an array")
	if result != nil {
		t.Errorf("non-array should return nil, got %v", result)
	}
}

// ─── formatAgentResult ──────────────────────────────────────────────────────

func TestFormatAgentResultSkipUsage(t *testing.T) {
	result := formatAgentResult("Hello world", "id1", "explore", 5, 1000, true)
	if result != "Hello world" {
		t.Errorf("skipUsage should return raw result, got %q", result)
	}
}

func TestFormatAgentResultWithUsage(t *testing.T) {
	result := formatAgentResult("Hello world", "id1", "explore", 5, 1000, false)
	if !strings.Contains(result, "Hello world") {
		t.Error("result should contain original text")
	}
	if !strings.Contains(result, "agentId: id1") {
		t.Error("result should contain agent ID")
	}
	if !strings.Contains(result, "agentType: explore") {
		t.Error("result should contain agent type")
	}
	if !strings.Contains(result, "tool_uses: 5") {
		t.Error("result should contain tool uses")
	}
	if !strings.Contains(result, "duration_ms: 1000") {
		t.Error("result should contain duration")
	}
}

func TestFormatAgentResultNoID(t *testing.T) {
	result := formatAgentResult("test", "", "type", 1, 100, false)
	if strings.Contains(result, "agentId:") {
		t.Error("empty agentID should not appear in output")
	}
}

func TestFormatAgentResultNoType(t *testing.T) {
	result := formatAgentResult("test", "id1", "", 1, 100, false)
	if strings.Contains(result, "agentType:") {
		t.Error("empty agentType should not appear in output")
	}
}
