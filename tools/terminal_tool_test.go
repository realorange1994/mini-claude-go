package tools

import (
	"runtime"
	"strings"
	"testing"
)

// ─── TerminalTool interface ──────────────────────────────────────────────────

func TestTerminalToolName(t *testing.T) {
	tool := &TerminalTool{}
	if tool.Name() != "terminal" {
		t.Errorf("expected 'terminal', got %q", tool.Name())
	}
}

func TestTerminalToolSchema(t *testing.T) {
	tool := &TerminalTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "operation" {
		t.Errorf("expected required=[operation], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["operation"]; !ok {
		t.Error("schema should have operation property")
	}
	if _, ok := props["manager"]; !ok {
		t.Error("schema should have manager property")
	}
	if _, ok := props["session"]; !ok {
		t.Error("schema should have session property")
	}
	if _, ok := props["command"]; !ok {
		t.Error("schema should have command property")
	}
	if _, ok := props["cwd"]; !ok {
		t.Error("schema should have cwd property")
	}
	if _, ok := props["new_name"]; !ok {
		t.Error("schema should have new_name property")
	}
}

func TestTerminalToolPermissions(t *testing.T) {
	tool := &TerminalTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

func TestTerminalToolExecuteWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	tool := &TerminalTool{}
	result := tool.Execute(map[string]any{"operation": "list"})
	if !result.IsError {
		t.Error("Windows should return error")
	}
	if !strings.Contains(result.Output, "not supported on Windows") {
		t.Errorf("should mention Windows unsupported, got %q", result.Output)
	}
}

// ─── buildTerminalCommand ───────────────────────────────────────────────────

func TestBuildTerminalCommandListTmux(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandListScreen(t *testing.T) {
	cmd, err := buildTerminalCommand("screen", "list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandNewDefaultSession(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "new", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default session name is "main"
	found := false
	for _, arg := range cmd.Args {
		if arg == "main" {
			found = true
		}
	}
	if !found {
		t.Errorf("default session name 'main' should be in args: %v", cmd.Args)
	}
}

func TestBuildTerminalCommandNewWithSession(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "new", map[string]any{"session": "work"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, arg := range cmd.Args {
		if arg == "work" {
			found = true
		}
	}
	if !found {
		t.Errorf("session name 'work' should be in args: %v", cmd.Args)
	}
}

func TestBuildTerminalCommandAttachNoSession(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "attach", map[string]any{})
	if err == nil {
		t.Error("attach without session should return error")
	}
}

func TestBuildTerminalCommandAttachWithSession(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "attach", map[string]any{"session": "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandSendNoSession(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "send", map[string]any{"command": "ls"})
	if err == nil {
		t.Error("send without session should return error")
	}
}

func TestBuildTerminalCommandSendNoCommand(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "send", map[string]any{"session": "main"})
	if err == nil {
		t.Error("send without command should return error")
	}
}

func TestBuildTerminalCommandSendValid(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "send", map[string]any{"session": "main", "command": "ls -la"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandKillNoSession(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "kill", map[string]any{})
	if err == nil {
		t.Error("kill without session should return error")
	}
}

func TestBuildTerminalCommandKillWithSession(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "kill", map[string]any{"session": "old"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandRenameNoSession(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "rename", map[string]any{"new_name": "new"})
	if err == nil {
		t.Error("rename without session should return error")
	}
}

func TestBuildTerminalCommandRenameNoNewName(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "rename", map[string]any{"session": "old"})
	if err == nil {
		t.Error("rename without new_name should return error")
	}
}

func TestBuildTerminalCommandRenameValid(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "rename", map[string]any{"session": "old", "new_name": "new"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandUnknownOp(t *testing.T) {
	_, err := buildTerminalCommand("tmux", "explode", nil)
	if err == nil {
		t.Error("unknown operation should return error")
	}
}

func TestBuildTerminalCommandDetach(t *testing.T) {
	cmd, err := buildTerminalCommand("tmux", "detach", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}

func TestBuildTerminalCommandScreenDetach(t *testing.T) {
	cmd, err := buildTerminalCommand("screen", "detach", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Path == "" {
		t.Error("command should have a path")
	}
}
