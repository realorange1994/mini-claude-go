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

func TestBashAllowInternalURL(t *testing.T) {
	tool := &ExecTool{}
	cmds := []string{
		"curl http://10.0.0.1/admin",
		"curl http://localhost:8080/internal",
		"curl http://192.168.1.1/config",
		"curl http://127.0.0.1:3000/api",
	}
	for _, cmd := range cmds {
		result := tool.CheckPermissions(map[string]any{"command": cmd})
		if result != "" {
			t.Errorf("expected internal URL to be allowed: %s, got: %s", cmd, result)
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
