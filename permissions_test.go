package main

import (
	"testing"

	"miniclaudecode-go/tools"
)

// Mock tool for testing
type mockTool struct {
	name        string
	permissions string
}

func (m *mockTool) Name() string                           { return m.name }
func (m *mockTool) Description() string                    { return "mock tool" }
func (m *mockTool) InputSchema() map[string]any            { return nil }
func (m *mockTool) CheckPermissions(params map[string]any) string { return m.permissions }
func (m *mockTool) Execute(params map[string]any) tools.ToolResult {
	return tools.ToolResult{Output: "ok"}
}

func TestPermissionGateToolSelfCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(cfg)

	// Tool that warns about itself - in ModeAuto warnings are not enforced
	tool := &mockTool{name: "test", permissions: ""}
	result := gate.Check(tool, map[string]any{})

	if result != nil {
		t.Errorf("expected permission to be allowed in auto mode, got: %v", result)
	}
}

func TestPermissionGateToolAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(cfg)

	// Tool that allows itself
	tool := &mockTool{name: "test", permissions: ""}
	result := gate.Check(tool, map[string]any{})

	if result != nil {
		t.Errorf("expected permission to be allowed, got: %v", result)
	}
}

func TestPermissionGatePlanModeBlocksWrite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModePlan
	gate := NewPermissionGate(cfg)

	writeTools := []string{"exec", "write_file", "edit_file", "multi_edit", "fileops"}
	for _, toolName := range writeTools {
		tool := &mockTool{name: toolName, permissions: ""}
		result := gate.Check(tool, map[string]any{})

		if result == nil {
			t.Errorf("expected %s to be blocked in plan mode", toolName)
		}
	}
}

func TestPermissionGatePlanModeAllowsRead(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModePlan
	gate := NewPermissionGate(cfg)

	readTools := []string{"read_file", "glob", "grep", "list_dir"}
	for _, toolName := range readTools {
		tool := &mockTool{name: toolName, permissions: ""}
		result := gate.Check(tool, map[string]any{})

		if result != nil {
			t.Errorf("expected %s to be allowed in plan mode", toolName)
		}
	}
}

func TestPermissionGateDeniedPatterns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	cfg.DeniedPatterns = []string{"rm -rf", "sudo"}
	gate := NewPermissionGate(cfg)

	// Create a mock exec tool
	tool := &mockTool{name: "exec", permissions: ""}

	testCases := []struct {
		command  string
		expected bool // true = should be denied
	}{
		{"rm -rf /", true},
		{"sudo rm file", true},
		{"ls -la", false},
		{"git status", false},
	}

	for _, tc := range testCases {
		result := gate.Check(tool, map[string]any{"command": tc.command})
		denied := result != nil
		if denied != tc.expected {
			t.Errorf("command %q: expected denied=%v, got %v", tc.command, tc.expected, denied)
		}
	}
}

func TestPermissionGateAutoModeAllowsAll(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(cfg)

	// All tools should be allowed in auto mode (except denied patterns)
	tools := []string{"exec", "write_file", "edit_file", "read_file"}
	for _, toolName := range tools {
		tool := &mockTool{name: toolName, permissions: ""}
		result := gate.Check(tool, map[string]any{})

		if result != nil {
			t.Errorf("expected %s to be allowed in auto mode", toolName)
		}
	}
}

func TestPermissionGateIsSafeCommand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedCommands = []string{"git status", "ls", "cat"}
	gate := NewPermissionGate(cfg)

	testCases := []struct {
		command string
		safe    bool
	}{
		{"git status", true},
		{"git status -s", true},
		{"ls -la", true},
		{"cat file.txt", true},
		{"rm -rf /", false},
		{"sudo apt install", false},
	}

	for _, tc := range testCases {
		result := gate.isSafeCommand(tc.command)
		if result != tc.safe {
			t.Errorf("isSafeCommand(%q) = %v, expected %v", tc.command, result, tc.safe)
		}
	}
}
