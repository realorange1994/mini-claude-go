package main

import (
	"testing"
)

func TestInvocationConfig_DefaultStyle(t *testing.T) {
	c := NewInvocationConfig()

	if c.GetStyle("bash") != InvocationJSON {
		t.Errorf("expected json, got %s", c.GetStyle("bash"))
	}
}

func TestInvocationConfig_ToolStyle(t *testing.T) {
	c := NewInvocationConfig()
	c.SetToolStyle("bash", InvocationShell)

	if c.GetStyle("bash") != InvocationShell {
		t.Errorf("expected shell, got %s", c.GetStyle("bash"))
	}
	if c.GetStyle("read_file") != InvocationJSON {
		t.Errorf("expected json for other tool, got %s", c.GetStyle("read_file"))
	}
}

func TestResolveInvocationStyle(t *testing.T) {
	c := NewInvocationConfig()
	c.SetToolStyle("bash", InvocationShell)

	if ResolveInvocationStyle(c, "bash") != InvocationShell {
		t.Error("expected shell")
	}
	if ResolveInvocationStyle(c, "read_file") != InvocationJSON {
		t.Error("expected json")
	}
	if ResolveInvocationStyle(nil, "bash") != InvocationJSON {
		t.Error("expected json for nil config")
	}
}

func TestShellWrap_SimpleArgs(t *testing.T) {
	params := map[string]any{
		"command": "ls -la",
		"timeout": 30,
	}

	result := ShellWrap("bash", params)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestShellWrap_BoolArgs(t *testing.T) {
	params := map[string]any{
		"recursive": true,
		"verbose":   false,
	}

	result := ShellWrap("grep", params)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestShellWrap_StringWithSpaces(t *testing.T) {
	params := map[string]any{
		"query": "hello world",
	}

	result := ShellWrap("search", params)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestShouldUseShell(t *testing.T) {
	c := NewInvocationConfig()
	c.SetToolStyle("bash", InvocationShell)

	if !ShouldUseShell(c, "bash", true) {
		t.Error("expected shell for bash with shell field")
	}
	if ShouldUseShell(c, "bash", false) {
		t.Error("expected no shell without shell field")
	}
	if ShouldUseShell(c, "read_file", true) {
		t.Error("expected no shell for read_file")
	}
	if ShouldUseShell(nil, "bash", true) {
		t.Error("expected no shell for nil config")
	}
}
