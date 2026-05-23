package tools

import (
	"testing"
)

// ─── ProcessTool interface ──────────────────────────────────────────────────

func TestProcessToolName(t *testing.T) {
	tool := &ProcessTool{}
	if tool.Name() != "process" {
		t.Errorf("expected 'process', got %q", tool.Name())
	}
}

func TestProcessToolSchema(t *testing.T) {
	tool := &ProcessTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["operation"]; !ok {
		t.Error("schema should have operation property")
	}
	if _, ok := props["pid"]; !ok {
		t.Error("schema should have pid property")
	}
	if _, ok := props["pattern"]; !ok {
		t.Error("schema should have pattern property")
	}
	if _, ok := props["signal"]; !ok {
		t.Error("schema should have signal property")
	}
	if _, ok := props["user"]; !ok {
		t.Error("schema should have user property")
	}
	if _, ok := props["lines"]; !ok {
		t.Error("schema should have lines property")
	}
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "operation" {
		t.Errorf("expected required=[operation], got %v", required)
	}
}

func TestProcessToolPermissions(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

// ─── ProcessTool Execute ────────────────────────────────────────────────────

func TestProcessToolExecuteNoOperation(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing operation should return error")
	}
}

func TestProcessToolExecuteUnknownOperation(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{"operation": "unknown"})
	if !result.IsError {
		t.Error("unknown operation should return error")
	}
}

func TestProcessToolExecuteKillNoPid(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{"operation": "kill"})
	if !result.IsError {
		t.Error("kill without pid should return error")
	}
}

func TestProcessToolExecuteKillZeroPid(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{"operation": "kill", "pid": 0})
	if !result.IsError {
		t.Error("kill with pid=0 should return error")
	}
}

func TestProcessToolExecutePkillNoPattern(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{"operation": "pkill"})
	if !result.IsError {
		t.Error("pkill without pattern should return error")
	}
}

func TestProcessToolExecutePgrepNoPattern(t *testing.T) {
	tool := &ProcessTool{}
	result := tool.Execute(map[string]any{"operation": "pgrep"})
	if !result.IsError {
		t.Error("pgrep without pattern should return error")
	}
}

// ─── sanitizePSInput ────────────────────────────────────────────────────────

func TestSanitizePSInputQuotes(t *testing.T) {
	result := sanitizePSInput(`it's "bad"`)
	if result != "its bad" {
		t.Errorf("expected 'its bad', got %q", result)
	}
}

func TestSanitizePSInputDollar(t *testing.T) {
	result := sanitizePSInput("$var")
	if result != "var" {
		t.Errorf("expected 'var', got %q", result)
	}
}

func TestSanitizePSInputPipe(t *testing.T) {
	result := sanitizePSInput("a|b")
	if result != "ab" {
		t.Errorf("expected 'ab', got %q", result)
	}
}

func TestSanitizePSInputSemicolon(t *testing.T) {
	result := sanitizePSInput("a;b")
	if result != "ab" {
		t.Errorf("expected 'ab', got %q", result)
	}
}

func TestSanitizePSInputBraces(t *testing.T) {
	result := sanitizePSInput("{code}")
	if result != "code" {
		t.Errorf("expected 'code', got %q", result)
	}
}

func TestSanitizePSInputBacktick(t *testing.T) {
	result := sanitizePSInput("`cmd`")
	if result != "cmd" {
		t.Errorf("expected 'cmd', got %q", result)
	}
}

func TestSanitizePSInputNewline(t *testing.T) {
	result := sanitizePSInput("line1\nline2")
	if result != "line1line2" {
		t.Errorf("expected 'line1line2', got %q", result)
	}
}

func TestSanitizePSInputClean(t *testing.T) {
	result := sanitizePSInput("clean_name")
	if result != "clean_name" {
		t.Errorf("clean input should be unchanged, got %q", result)
	}
}

func TestSanitizePSInputEmpty(t *testing.T) {
	result := sanitizePSInput("")
	if result != "" {
		t.Errorf("empty should remain empty, got %q", result)
	}
}

// ─── runCmd ─────────────────────────────────────────────────────────────────

func TestRunCmdNil(t *testing.T) {
	result := runCmd(nil)
	if !result.IsError {
		t.Error("nil command should return error")
	}
}
