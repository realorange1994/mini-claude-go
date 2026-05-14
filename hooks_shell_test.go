package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestHookGlobMatch tests the glob matching function.
func TestHookGlobMatch(t *testing.T) {
	tests := []struct {
		pattern  string
		text     string
		expected bool
	}{
		{"Bash", "Bash", true},
		{"*", "Bash", true},
		{"*", "Write", true},
		{"B*", "Bash", true},
		{"B*h", "Bash", true},
		{"?ash", "Bash", true},
		{"Ba?h", "Bash", true},
		{"Ba?h", "Batch", false},
		{"B??h", "Bash", true},
		{"Write", "Edit", false},
		{"Write*", "Write", true},
		{"Write*", "WriteFile", true},
		{"*Use", "PreToolUse", true},
	}

	for _, tt := range tests {
		got := hookGlobMatch(tt.pattern, tt.text)
		if got != tt.expected {
			t.Errorf("hookGlobMatch(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.expected)
		}
	}
}

// TestMatchHook tests the hook matcher.
func TestMatchHook(t *testing.T) {
	tests := []struct {
		matcher  string
		tool     string
		expected bool
	}{
		{"Bash", "Bash", true},
		{"*", "Bash", true},
		{"Write*", "WriteFile", true},
		{"B*h", "Bash", true},
		{"Edit", "Write", false},
	}

	for _, tt := range tests {
		hook := HookCommand{Matcher: tt.matcher, Command: "echo hi"}
		got := MatchHook(hook, tt.tool)
		if got != tt.expected {
			t.Errorf("MatchHook(%q, %q) = %v, want %v", tt.matcher, tt.tool, got, tt.expected)
		}
	}
}

// TestHookShellResult_ParseStdout tests parsing of hook stdout.
func TestHookShellResult_ParseStdout(t *testing.T) {
	tests := []struct {
		name            string
		stdout          string
		expectBlock     bool
		expectAsk       bool
		expectReason    string
		expectContinue  bool
	}{
		{
			name:           "block decision",
			stdout:         `{"decision":"block","reason":"unsafe command"}`,
			expectBlock:    true,
			expectReason:   "unsafe command",
			expectContinue: true,
		},
		{
			name:           "approve decision",
			stdout:         `{"decision":"approve"}`,
			expectBlock:    false,
		},
		{
			name:           "PreToolUse deny",
			stdout:         `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"blocked by policy"}}`,
			expectBlock:    true,
			expectReason:   "blocked by policy",
			expectContinue: true,
		},
		{
			name:           "PreToolUse allow",
			stdout:         `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`,
			expectBlock:    false,
		},
		{
			name:           "PreToolUse ask",
			stdout:         `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}`,
			expectAsk:      true,
			expectContinue: true,
		},
		{
			name:           "continue false",
			stdout:         `{"continue":false,"stopReason":"hook stop"}`,
			expectContinue: false,
		},
		{
			name:           "plain text",
			stdout:         "some plain text output",
			expectBlock:    false,
			expectContinue: true, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &HookShellResult{RawStdout: tt.stdout}
			r.ParseStdout()

			if r.ShouldBlock() != tt.expectBlock {
				t.Errorf("ShouldBlock() = %v, want %v", r.ShouldBlock(), tt.expectBlock)
			}
			if r.ShouldAsk() != tt.expectAsk {
				t.Errorf("ShouldAsk() = %v, want %v", r.ShouldAsk(), tt.expectAsk)
			}
			if tt.expectReason != "" && r.BlockReason() != tt.expectReason {
				t.Errorf("BlockReason() = %q, want %q", r.BlockReason(), tt.expectReason)
			}
		})
	}
}

// TestExecuteShellHook_Echo tests a simple echo command hook.
func TestExecuteShellHook_Echo(t *testing.T) {
	if os.Getenv("CLAUDE_HOOK_TEST") == "1" {
		// Skip in subprocess to avoid recursive test execution
		t.Skip("skipping in subprocess")
	}

	hook := HookCommand{
		Matcher: "test",
		Command: "echo '{\"decision\":\"approve\"}'",
	}

	input := `{"tool_name":"test","tool_input":{}}`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ExecuteShellHook(ctx, hook, HookPreToolUse, input, nil)
	if err != nil {
		t.Fatalf("ExecuteShellHook() error: %v", err)
	}

	result.ParseStdout()
	if result.ShouldBlock() {
		t.Errorf("hook should not have blocked, got block reason: %s", result.BlockReason())
	}
}

// TestExecuteShellHook_Block tests a hook that blocks execution.
func TestExecuteShellHook_Block(t *testing.T) {
	if os.Getenv("CLAUDE_HOOK_TEST") == "1" {
		t.Skip("skipping in subprocess")
	}

	hook := HookCommand{
		Matcher: "Bash",
		Command: "echo '{\"decision\":\"block\",\"reason\":\"test block\"}'",
	}

	input := `{"tool_name":"Bash","tool_input":{}}`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ExecuteShellHook(ctx, hook, HookPreToolUse, input, nil)
	if err != nil {
		t.Fatalf("ExecuteShellHook() error: %v", err)
	}

	result.ParseStdout()
	if !result.ShouldBlock() {
		t.Fatalf("hook should have blocked")
	}
	if result.BlockReason() != "test block" {
		t.Errorf("BlockReason() = %q, want %q", result.BlockReason(), "test block")
	}
}

// TestHookBlockError tests the HookBlockError type.
func TestHookBlockError(t *testing.T) {
	err := HookBlockError{
		ToolName: "Bash",
		Command:  "echo hi",
		Reason:   "unsafe",
	}

	msg := err.Error()
	if msg == "" {
		t.Error("HookBlockError.Error() returned empty string")
	}
}

// TestLoadHooksFromSettings tests loading hooks from a JSON settings file.
func TestLoadHooksFromSettings(t *testing.T) {
	// Create a temporary settings file
	content := `{
		"hooks": {
			"PreToolUse": [
				{"matcher": "Bash", "command": "echo pre-bash"},
				{"matcher": "*", "command": "echo wildcard", "timeout": 30}
			],
			"PostToolUse": [
				{"matcher": "Write", "command": "echo post-write"}
			]
		}
	}`

	tmpFile, err := os.CreateTemp("", "settings-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	hooks, err := LoadHooksFromSettings(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadHooksFromSettings() error: %v", err)
	}

	if len(hooks["PreToolUse"]) != 2 {
		t.Errorf("Expected 2 PreToolUse hooks, got %d", len(hooks["PreToolUse"]))
	}

	if len(hooks["PostToolUse"]) != 1 {
		t.Errorf("Expected 1 PostToolUse hook, got %d", len(hooks["PostToolUse"]))
	}

	if hooks["PreToolUse"][0].Matcher != "Bash" {
		t.Errorf("First hook matcher = %q, want %q", hooks["PreToolUse"][0].Matcher, "Bash")
	}

	if hooks["PreToolUse"][1].Timeout != 30 {
		t.Errorf("Second hook timeout = %d, want 30", hooks["PreToolUse"][1].Timeout)
	}
}

// TestLoadHooksFromSettings_MissingFile tests loading from a non-existent file.
func TestLoadHooksFromSettings_MissingFile(t *testing.T) {
	_, err := LoadHooksFromSettings("/nonexistent/path/settings.json")
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

// TestLoadHooksFromSettings_NoHooks tests loading settings without hooks section.
func TestLoadHooksFromSettings_NoHooks(t *testing.T) {
	content := `{"mcp": {"servers": {}}}`

	tmpFile, err := os.CreateTemp("", "settings-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	hooks, err := LoadHooksFromSettings(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadHooksFromSettings() error: %v", err)
	}

	if hooks != nil {
		t.Errorf("Expected nil hooks, got %v", hooks)
	}
}

// TestHookShellResult_UpdatedInput tests parsing updatedInput from hook output.
func TestHookShellResult_UpdatedInput(t *testing.T) {
	stdout := `{
		"hookSpecificOutput": {
			"hookEventName": "PreToolUse",
			"permissionDecision": "allow",
			"updatedInput": {"command": "echo safe", "timeout": 30}
		}
	}`

	r := &HookShellResult{RawStdout: stdout}
	r.ParseStdout()

	if r.PermissionDecision != "allow" {
		t.Errorf("PermissionDecision = %q, want %q", r.PermissionDecision, "allow")
	}

	if r.UpdatedInput == nil {
		t.Fatal("UpdatedInput should not be nil")
	}

	if r.UpdatedInput["command"] != "echo safe" {
		t.Errorf("UpdatedInput[command] = %v, want %q", r.UpdatedInput["command"], "echo safe")
	}
}

// TestHookEnvironment tests that hook environment includes CLAUDE variables.
func TestHookEnvironment(t *testing.T) {
	extra := map[string]string{"CLAUDE_TEST_VAR": "test_value"}
	env := buildHookEnv(extra)

	// Check that CLAUDE_PROJECT_DIR is set
	foundProjectDir := false
	foundTestVar := false
	prefix := "CLAUDE_PROJECT_DIR="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			foundProjectDir = true
		}
		if e == "CLAUDE_TEST_VAR=test_value" {
			foundTestVar = true
		}
	}

	if !foundProjectDir {
		t.Error("CLAUDE_PROJECT_DIR not found in hook environment")
	}
	if !foundTestVar {
		t.Error("CLAUDE_TEST_VAR not found in hook environment")
	}
}

// TestHookJSONRoundTrip tests that hook input can be marshaled and unmarshaled.
func TestHookJSONRoundTrip(t *testing.T) {
	input := HookInput{
		HookType: "pre_tool_use",
		Metadata: map[string]interface{}{
			"tool_name": "Bash",
			"tool_input": map[string]interface{}{
				"command": "ls -la",
			},
		},
	}

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if decoded["HookType"] != "pre_tool_use" {
		t.Errorf("decoded[HookType] = %v, want pre_tool_use", decoded["HookType"])
	}
}
