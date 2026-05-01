package main

import (
	"testing"
)

func TestAutoAllowlisted(t *testing.T) {
	safeTools := []string{
		"read_file", "glob", "grep", "list_dir", "tool_search",
		"brief", "runtime_info", "memory_add", "memory_search",
		"task_create", "task_list", "task_get", "task_update",
		"list_mcp_tools", "list_skills", "search_skills", "read_skill",
		"mcp_server_status",
	}
	for _, name := range safeTools {
		if !IsAutoAllowlisted(name) {
			t.Errorf("expected %q to be allowlisted", name)
		}
	}

	// Non-whitelisted tools should not be allowlisted
	dangerousTools := []string{"exec", "write_file", "edit_file", "multi_edit", "fileops", "git", "agent"}
	for _, name := range dangerousTools {
		if IsAutoAllowlisted(name) {
			t.Errorf("expected %q to NOT be allowlisted", name)
		}
	}
}

func TestNewAutoModeClassifierDisabled(t *testing.T) {
	// Empty API key → disabled
	c := NewAutoModeClassifier("", "", "model")
	if c.IsEnabled() {
		t.Error("expected classifier to be disabled with empty API key")
	}

	// Empty model → disabled
	c = NewAutoModeClassifier("key", "", "")
	if c.IsEnabled() {
		t.Error("expected classifier to be disabled with empty model")
	}
}

func TestClassifierClassifyDisabled(t *testing.T) {
	c := NewAutoModeClassifier("", "", "model")
	result := c.Classify("exec", map[string]any{"command": "rm -rf /"}, "")
	if result.Allow {
		t.Error("disabled classifier should block (fail-closed), not allow")
	}
	if result.Reason == "" {
		t.Error("expected a reason for blocking")
	}
}

func TestClassifierClassifyWhitelisted(t *testing.T) {
	c := NewAutoModeClassifier("fake-key", "", "fake-model")
	// Even with a "enabled" classifier, whitelisted tools should auto-allow
	// without making an LLM call (which would fail with fake credentials)
	result := c.Classify("read_file", map[string]any{"path": "/etc/passwd"}, "")
	if !result.Allow {
		t.Error("whitelisted tool should be allowed")
	}
	if result.Reason != "whitelisted tool" {
		t.Errorf("expected 'whitelisted tool' reason, got %q", result.Reason)
	}
}

func TestParseClassifierResponse(t *testing.T) {
	tests := []struct {
		input      string
		wantAllow  bool
		wantReason string
	}{
		{`{"decision":"allow","reason":"safe operation"}`, true, "safe operation"},
		{`{"decision":"block","reason":"dangerous"}`, false, "dangerous"},
		{`{"decision":"ALLOW","reason":""}`, true, "classified as safe"},
		{`{"decision":"BLOCK","reason":""}`, false, "classified as potentially unsafe"},
		// With markdown wrapper
		{"```json\n{\"decision\":\"allow\",\"reason\":\"ok\"}\n```", true, "ok"},
		// Unparseable
		{"I cannot classify this", false, ""}, // returns nil
	}

	for _, tc := range tests {
		result := parseClassifierResponse(tc.input)
		if tc.wantReason == "" {
			// Expect nil
			if result != nil {
				t.Errorf("parseClassifierResponse(%q): expected nil, got %+v", tc.input, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("parseClassifierResponse(%q): expected non-nil result", tc.input)
			continue
		}
		if result.Allow != tc.wantAllow {
			t.Errorf("parseClassifierResponse(%q): Allow=%v, want %v", tc.input, result.Allow, tc.wantAllow)
		}
		if result.Reason != tc.wantReason {
			t.Errorf("parseClassifierResponse(%q): Reason=%q, want %q", tc.input, result.Reason, tc.wantReason)
		}
	}
}

func TestFormatActionForClassifier(t *testing.T) {
	tests := []struct {
		toolName string
		input    map[string]any
		contains string // output should contain this substring
	}{
		{"exec", map[string]any{"command": "ls -la"}, "ls -la"},
		{"write_file", map[string]any{"path": "/tmp/test.txt"}, "/tmp/test.txt"},
		{"edit_file", map[string]any{"path": "main.go", "old_string": "func main()"}, "main.go"},
		{"git", map[string]any{"args": "commit -m test"}, "commit -m test"},
		{"agent", map[string]any{"description": "search", "prompt": "find all TODOs"}, "search"},
	}

	for _, tc := range tests {
		output := formatActionForClassifier(tc.toolName, tc.input)
		if !containsSubstring(output, tc.contains) {
			t.Errorf("formatActionForClassifier(%q, %v): output %q should contain %q",
				tc.toolName, tc.input, output, tc.contains)
		}
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFormatToolInputCompact(t *testing.T) {
	tests := []struct {
		toolName string
		input    any
		contains string
	}{
		{"exec", map[string]any{"command": "ls"}, "ls"},
		{"write_file", map[string]any{"path": "/tmp/f.txt"}, "/tmp/f.txt"},
		{"grep", map[string]any{"pattern": "TODO", "path": "src"}, "TODO"},
		{"glob", map[string]any{"pattern": "**/*.go"}, "**/*.go"},
	}

	for _, tc := range tests {
		output := formatToolInputCompact(tc.toolName, tc.input)
		if !containsSubstring(output, tc.contains) {
			t.Errorf("formatToolInputCompact(%q, %v): output %q should contain %q",
				tc.toolName, tc.input, output, tc.contains)
		}
	}
}

func TestCacheKeyGeneration(t *testing.T) {
	c := NewAutoModeClassifier("key", "", "model")

	// exec commands should be cached by command prefix
	key1 := c.cacheKey("exec", map[string]any{"command": "ls -la"})
	if key1 != "exec:ls -la" {
		t.Errorf("cacheKey for exec: got %q, want %q", key1, "exec:ls -la")
	}

	// file ops should be cached by tool+path
	key2 := c.cacheKey("write_file", map[string]any{"path": "/tmp/test.txt"})
	if key2 != "write_file:/tmp/test.txt" {
		t.Errorf("cacheKey for write_file: got %q, want %q", key2, "write_file:/tmp/test.txt")
	}

	// Generic: tool name only
	key3 := c.cacheKey("unknown_tool", map[string]any{"x": "y"})
	if key3 != "unknown_tool" {
		t.Errorf("cacheKey for unknown: got %q, want %q", key3, "unknown_tool")
	}
}

func TestBuildCompactTranscriptEmpty(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	result := BuildCompactTranscript(ctx, 20)
	if result != "" {
		t.Errorf("expected empty transcript for empty context, got %q", result)
	}
}

func TestBuildCompactTranscriptWithUserMessage(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello, please help me")
	result := BuildCompactTranscript(ctx, 20)
	if !containsSubstring(result, "[User]") {
		t.Error("expected [User] tag in transcript")
	}
	if !containsSubstring(result, "Hello") {
		t.Error("expected 'Hello' in transcript")
	}
}

func TestBuildCompactTranscriptSkipsAssistant(t *testing.T) {
	cfg := DefaultConfig()
	ctx := NewConversationContext(cfg)
	ctx.AddUserMessage("Hello")
	ctx.AddAssistantText("I can help with that")
	result := BuildCompactTranscript(ctx, 20)
	if containsSubstring(result, "[Assistant]") {
		t.Error("assistant text should be skipped in transcript")
	}
	if containsSubstring(result, "I can help") {
		t.Error("assistant text content should not appear in transcript")
	}
}

func TestPermissionGateAutoModeWithClassifier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(&cfg)

	// Without classifier: whitelisted tools auto-allow
	whitelistedTool := &mockTool{name: "read_file", permissions: ""}
	result := gate.Check(whitelistedTool, map[string]any{"path": "/tmp/test.txt"})
	if result != nil {
		t.Errorf("whitelisted tool should be allowed in auto mode without classifier, got: %v", result)
	}

	// Without classifier: non-whitelisted tools also auto-allow (legacy fallback)
	execTool := &mockTool{name: "exec", permissions: ""}
	result = gate.Check(execTool, map[string]any{"command": "ls -la"})
	if result != nil {
		t.Errorf("exec should be auto-allowed when no classifier configured, got: %v", result)
	}
}

func TestPermissionGateAutoModeDenialTracking(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(&cfg)

	// Without any classifier configured, non-whitelisted tools auto-allow (legacy)
	execTool := &mockTool{name: "exec", permissions: ""}
	result := gate.Check(execTool, map[string]any{"command": "ls -la"})
	if result != nil {
		t.Error("without classifier, auto mode should auto-allow (legacy behavior)")
	}

	// Denial count is not tracked when no classifier is present (all auto-allowed)
	// Denial tracking only applies when classifier is enabled and blocks actions
}

func TestPermissionGateAutoModeDenialCountReset(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(&cfg)

	// Simulate denial count being set directly
	gate.denialCount = 5

	// Whitelisted tool should reset denial count to 0
	whitelistedTool := &mockTool{name: "read_file", permissions: ""}
	gate.Check(whitelistedTool, map[string]any{})
	if gate.denialCount != 0 {
		t.Errorf("whitelisted tool should reset denial count, got %d", gate.denialCount)
	}
}

func TestPermissionGateAutoModeWhitelistResetsDenial(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PermissionMode = ModeAuto
	gate := NewPermissionGate(&cfg)
	classifier := NewAutoModeClassifier("", "", "model")
	gate.WithClassifier(classifier)

	// Set up some denial count
	gate.denialCount = 5

	// Whitelisted tool should reset denial count
	whitelistedTool := &mockTool{name: "read_file", permissions: ""}
	gate.Check(whitelistedTool, map[string]any{})
	if gate.denialCount != 0 {
		t.Errorf("whitelisted tool should reset denial count, got %d", gate.denialCount)
	}
}
