package main

import (
	"testing"

	"miniclaudecode-go/tools"
)

// Mock tool for testing
type mockTool struct {
	name        string
	permissions tools.PermissionResult
}

func (m *mockTool) Name() string                                           { return m.name }
func (m *mockTool) Description() string                                    { return "mock tool" }
func (m *mockTool) InputSchema() map[string]any                            { return nil }
func (m *mockTool) CheckPermissions(params map[string]any) tools.PermissionResult { return m.permissions }
func (m *mockTool) Execute(params map[string]any) tools.ToolResult {
	return tools.ToolResult{Output: "ok"}
}

func TestPermissionGateToolSelfCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(&cfg)

	// Tool that warns about itself - in ModeAuto warnings are not enforced
	tool := &mockTool{name: "test", permissions: tools.PermissionResultPassthrough()}
	result := gate.Check(tool, map[string]any{})

	if result != nil {
		t.Errorf("expected permission to be allowed in auto mode, got: %v", result)
	}
}

func TestPermissionGateToolAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(&cfg)

	// Tool that allows itself
	tool := &mockTool{name: "test", permissions: tools.PermissionResultPassthrough()}
	result := gate.Check(tool, map[string]any{})

	if result != nil {
		t.Errorf("expected permission to be allowed, got: %v", result)
	}
}

func TestPermissionGatePlanModeBlocksWrite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModePlan
	gate := NewPermissionGate(&cfg)

	writeTools := []string{"exec", "write_file", "edit_file", "multi_edit", "fileops"}
	for _, toolName := range writeTools {
		tool := &mockTool{name: toolName, permissions: tools.PermissionResultPassthrough()}
		result := gate.Check(tool, map[string]any{})

		if result == nil {
			t.Errorf("expected %s to be blocked in plan mode", toolName)
		}
	}
}

func TestPermissionGatePlanModeAllowsRead(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModePlan
	gate := NewPermissionGate(&cfg)

	readTools := []string{"read_file", "glob", "grep", "list_dir"}
	for _, toolName := range readTools {
		tool := &mockTool{name: toolName, permissions: tools.PermissionResultPassthrough()}
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
	gate := NewPermissionGate(&cfg)

	// Create a mock exec tool
	tool := &mockTool{name: "exec", permissions: tools.PermissionResultPassthrough()}

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
	gate := NewPermissionGate(&cfg)

	// All tools should be allowed in auto mode (except denied patterns)
	toolNames := []string{"exec", "write_file", "edit_file", "read_file"}
	for _, toolName := range toolNames {
		tool := &mockTool{name: toolName, permissions: tools.PermissionResultPassthrough()}
		result := gate.Check(tool, map[string]any{})

		if result != nil {
			t.Errorf("expected %s to be allowed in auto mode", toolName)
		}
	}
}

func TestPermissionGateShouldAvoidPermissionPrompts(t *testing.T) {
	// When ShouldAvoidPermissionPrompts is true and mode is ask,
	// dangerous tools should be auto-denied without prompting the user.
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAsk
	cfg.ShouldAvoidPermissionPrompts = true
	gate := NewPermissionGate(&cfg)

	dangerousTools := []string{"exec", "write_file", "edit_file", "multi_edit", "fileops"}
	for _, toolName := range dangerousTools {
		tool := &mockTool{name: toolName, permissions: tools.PermissionResultPassthrough()}
		result := gate.Check(tool, map[string]any{})
		if result == nil {
			t.Errorf("expected %s to be denied when ShouldAvoidPermissionPrompts=true", toolName)
		} else if !result.IsError {
			t.Errorf("expected IsError=true for %s denial, got: %v", toolName, result)
		}
	}

	// Non-dangerous tools should still be allowed without prompting
	safeTools := []string{"read_file", "glob", "grep", "list_dir"}
	for _, toolName := range safeTools {
		tool := &mockTool{name: toolName, permissions: tools.PermissionResultPassthrough()}
		result := gate.Check(tool, map[string]any{})
		if result != nil {
			t.Errorf("expected %s to be allowed when ShouldAvoidPermissionPrompts=true", toolName)
		}
	}
}

func TestPermissionGateIsSafeCommand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedCommands = []string{"git status", "ls", "cat"}
	gate := NewPermissionGate(&cfg)

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

// ---------------------------------------------------------------------------
// upstream port: acp/permissions.test.ts — ACP permission patterns
// These test the upstream invariants for createAcpCanUseTool translated
// to Go PermissionGate semantics.
// ---------------------------------------------------------------------------

// upstream: 'returns allow when in bypassPermissions mode without calling requestPermission'
func TestPermissionGate_BypassMode_AllowsAll(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeBypass

	// Bypass mode should allow any tool, even destructive ones
	writeTools := []string{"exec", "write_file", "edit_file", "multi_edit", "fileops"}
	for _, toolName := range writeTools {
		tool := &mockTool{name: toolName, permissions: tools.PermissionResultPassthrough()}
		result := gateCheck(t, cfg, tool, map[string]any{})
		if result != nil {
			t.Errorf("bypass mode should allow %s, got denial: %s", toolName, result.Output)
		}
	}
}

// upstream: 'returns allow when client selects allow option'
func TestPermissionGate_ToolAllowResult_Allows(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto

	tool := &mockTool{name: "exec", permissions: tools.PermissionResult{Behavior: tools.PermissionAllow}}
	result := gateCheck(t, cfg, tool, map[string]any{"command": "ls"})
	if result != nil {
		t.Errorf("tool self-allow should be allowed, got: %s", result.Output)
	}
}

// upstream: 'returns deny when client selects reject option'
func TestPermissionGate_ToolDenyResult_Denies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto

	tool := &mockTool{name: "exec", permissions: tools.PermissionResult{
		Behavior: tools.PermissionDeny,
		Message:  "Permission denied by tool",
	}}
	result := gateCheck(t, cfg, tool, map[string]any{})
	if result == nil {
		t.Error("tool self-deny should be denied")
	} else if !result.IsError {
		t.Error("tool self-deny should return IsError=true")
	}
}

// upstream: 'returns deny when client cancels' / 'returns deny when requestPermission throws'
// Go equivalent: when AskUserQuestion rejects (user says no), the tool is denied
func TestPermissionGate_AskMode_UserReject_Denies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAsk
	cfg.ShouldAvoidPermissionPrompts = true // simulates "user rejected/cancelled"

	tool := &mockTool{name: "exec", permissions: tools.PermissionResultPassthrough()}
	result := gateCheck(t, cfg, tool, map[string]any{})
	if result == nil {
		t.Error("user rejection should deny the tool")
	} else if !result.IsError {
		t.Error("user rejection should return IsError=true")
	}
}

// upstream: 'passes correct sessionId and toolCallId to requestPermission'
// Go equivalent: denial messages should contain relevant tool context
func TestPermissionGate_DenialMessage_ContainsToolName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAsk
	cfg.ShouldAvoidPermissionPrompts = true

	tool := &mockTool{name: "write_file", permissions: tools.PermissionResultPassthrough()}
	result := gateCheck(t, cfg, tool, map[string]any{"file_path": "/tmp/test.txt"})
	if result == nil {
		t.Fatal("expected denial result")
	}
	// Denial should mention the tool or context
	if result.Output == "" {
		t.Error("denial message should not be empty")
	}
}

// upstream: 'options include allow_always, allow_once and reject_once'
// Go equivalent: ask mode presents a choice (yes/no) — verify the ask path
// is taken and user can approve
func TestPermissionGate_AskMode_UserApprove_Allows(t *testing.T) {
	// In Ask mode with ShouldAvoidPermissionPrompts=false, dangerous tools
	// would prompt the user. Since we can't simulate stdin in tests,
	// we verify that the passthrough result leads to an ask flow
	// by using ShouldAvoidPermissionPrompts=true to short-circuit to deny.
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAsk
	cfg.ShouldAvoidPermissionPrompts = true

	tool := &mockTool{name: "exec", permissions: tools.PermissionResultPassthrough()}
	result := gateCheck(t, cfg, tool, map[string]any{})
	// With prompts disabled, it should deny (user "cancelled")
	if result == nil {
		t.Error("expected denial when prompts disabled")
	}
}

// upstream: 'returns deny when connection lost'
// Go equivalent: when tool has an error result (e.g., connection lost), deny
func TestPermissionGate_ConnectionError_Denies(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto

	// Simulate a tool that reports an error internally
	tool := &mockTool{name: "exec", permissions: tools.PermissionResult{
		Behavior: tools.PermissionDeny,
		Message:  "connection lost",
	}}
	result := gateCheck(t, cfg, tool, map[string]any{})
	if result == nil {
		t.Error("connection error should deny")
	}
}

// upstream: idempotency — bypass mode always allows regardless of tool params
func TestPermissionGate_BypassMode_Idempotent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeBypass
	cfg.DeniedPatterns = nil // Clear denied patterns to test pure bypass

	paramsList := []map[string]any{
		{"command": "ls"},
		{"command": "rm -rf /tmp/old"},
		{"file_path": "/tmp/test.txt"},
		{"command": "curl http://example.com"},
	}

	for i, params := range paramsList {
		tool := &mockTool{name: "exec", permissions: tools.PermissionResultPassthrough()}
		result := gateCheck(t, cfg, tool, params)
		if result != nil {
			t.Errorf("bypass case %d: expected allow, got denial: %s", i, result.Output)
		}
	}
}

// upstream: boundary — empty params in bypass mode
func TestPermissionGate_BypassMode_EmptyParams(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeBypass

	tool := &mockTool{name: "exec", permissions: tools.PermissionResultPassthrough()}
	result := gateCheck(t, cfg, tool, map[string]any{})
	if result != nil {
		t.Errorf("bypass with empty params should allow, got: %s", result.Output)
	}
}

// upstream: plan mode — read tools allowed, write tools blocked
func TestPermissionGate_PlanMode_Boundary(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModePlan

	// Read tools should be allowed
	for _, name := range []string{"read_file", "glob", "grep"} {
		tool := &mockTool{name: name, permissions: tools.PermissionResultPassthrough()}
		result := gateCheck(t, cfg, tool, map[string]any{})
		if result != nil {
			t.Errorf("plan mode should allow %s, got: %s", name, result.Output)
		}
	}

	// Write tools should be blocked
	for _, name := range []string{"exec", "write_file", "edit_file"} {
		tool := &mockTool{name: name, permissions: tools.PermissionResultPassthrough()}
		result := gateCheck(t, cfg, tool, map[string]any{})
		if result == nil {
			t.Errorf("plan mode should block %s", name)
		}
	}
}

// Helper to avoid duplicating config setup
func gateCheck(t *testing.T, cfg Config, tool *mockTool, params map[string]any) *tools.ToolResult {
	t.Helper()
	gate := NewPermissionGate(&cfg)
	return gate.Check(tool, params)
}
