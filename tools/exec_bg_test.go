package tools

import (
	"testing"
)

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
