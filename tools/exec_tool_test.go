package tools

import (
	"strings"
	"testing"
)

func TestBashDenyRmRf(t *testing.T) {
	tool := &ExecTool{}
	dangerous := []string{
		"rm -rf /",
		"rm -rf ~",
		"sudo rm -rf /",
	}
	for _, cmd := range dangerous {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result == "" {
			t.Errorf("expected denial for: %s", cmd)
		}
	}
}

func TestBashDenyInternalURL(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{
		"curl http://10.0.0.1/admin",
		"curl http://localhost:8080/internal",
		"curl http://192.168.1.1/config",
		"curl http://127.0.0.1:3000/api",
	}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result == "" {
			t.Errorf("expected internal URL to be denied: %s", cmd)
		}
	}
}

func TestBashDenyForkBomb(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{
		":(){ :|: & };:",
		"bomb(){ bomb|bomb & }; bomb",
	}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result == "" {
			t.Errorf("expected fork bomb denial for: %s", cmd)
		}
	}
}

func TestBashSafeCommand(t *testing.T) {
	tool := &ExecTool{}
	result := tool.Execute(map[string]any{"command": "echo hello"})
	if result.IsError {
		t.Errorf("expected echo to succeed, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", result.Output)
	}
}

func TestBashLsCommand(t *testing.T) {
	tool := &ExecTool{}
	result := tool.Execute(map[string]any{"command": "ls"})
	if result.IsError {
		t.Errorf("expected ls to succeed, got: %s", result.Output)
	}
}

func TestBashDenyMkfs(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "mkfs.ext4 /dev/sda"})
	if result == "" {
		t.Error("expected denial for mkfs")
	}
}

func TestBashDenySudoRm(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "sudo rm -rf /tmp/test"})
	if result == "" {
		t.Error("expected denial for sudo rm")
	}
}

func TestBashDenyRedirectToDev(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "echo bad > /dev/sda"})
	if result == "" {
		t.Error("expected denial for redirect to /dev/sda")
	}
}

func TestBashAllowPublicURL(t *testing.T) {
	tool := &ExecTool{}
	result := tool.CheckPermissions(map[string]any{"command": "curl https://example.com/api"})
	if result != "" {
		t.Errorf("expected public URL to be allowed, got: %s", result)
	}
}

func TestBashDenyPowerCommands(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{"shutdown -h now", "reboot", "poweroff"}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result == "" {
			t.Errorf("expected denial for: %s", cmd)
		}
	}
}

func TestExecToolBackgroundNoCallback(t *testing.T) {
	// When callback is nil, should fall through to foreground execution
	tool := &ExecTool{}
	result := tool.Execute(map[string]any{
		"command":           "echo hello",
		"run_in_background": true,
	})
	if result.IsError {
		t.Errorf("expected success with fallback, got: %s", result.Output)
	}
	if result.Output != "STDOUT:\nhello\nExit code: 0" {
		t.Logf("got output: %s", result.Output)
	}
}

func TestExecToolBackgroundWithCallback(t *testing.T) {
	var called bool
	var gotCommand string
	tool := &ExecTool{
		BackgroundTaskCallback: func(command, workingDir string) (string, string, string) {
			called = true
			gotCommand = command
			_ = workingDir
			return "b12345678", "/tmp/test.output", ""
		},
	}
	result := tool.Execute(map[string]any{
		"command":           "echo test",
		"run_in_background": true,
	})
	if !called {
		t.Error("expected callback to be invoked")
	}
	if gotCommand != "echo test" {
		t.Errorf("expected command 'echo test', got %q", gotCommand)
	}
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestExecToolForegroundIgnoresBackground(t *testing.T) {
	tool := &ExecTool{}
	// Without run_in_background=true, should run in foreground
	result := tool.Execute(map[string]any{
		"command": "echo foreground",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
}

func TestExecToolBackgroundEmptyCommand(t *testing.T) {
	tool := &ExecTool{
		BackgroundTaskCallback: func(command, workingDir string) (string, string, string) {
			t.Error("callback should not be called for empty command")
			return "", "", ""
		},
	}
	result := tool.Execute(map[string]any{
		"command":           "  ",
		"run_in_background": true,
	})
	if !result.IsError {
		t.Error("expected error for empty command")
	}
}

func TestExecToolBackgroundCallbackError(t *testing.T) {
	tool := &ExecTool{
		BackgroundTaskCallback: func(command, workingDir string) (string, string, string) {
			return "", "", "Error: simulated failure"
		},
	}
	result := tool.Execute(map[string]any{
		"command":           "echo test",
		"run_in_background": true,
	})
	if !result.IsError {
		t.Error("expected error from callback")
	}
}

func TestExecToolInputSchema(t *testing.T) {
	tool := &ExecTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}
	if _, ok := props["run_in_background"]; !ok {
		t.Error("expected run_in_background in schema")
	}
	if _, ok := props["command"]; !ok {
		t.Error("expected command in schema")
	}
}
