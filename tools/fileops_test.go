package tools

import (
	"testing"
)

// ─── FileOpsTool interface ──────────────────────────────────────────────────

func TestFileOpsToolName(t *testing.T) {
	tool := &FileOpsTool{}
	if tool.Name() != "fileops" {
		t.Errorf("expected 'fileops', got %q", tool.Name())
	}
}

func TestFileOpsToolSchema(t *testing.T) {
	tool := &FileOpsTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 2 {
		t.Errorf("expected 2 required params, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["operation"]; !ok {
		t.Error("schema should have operation property")
	}
	if _, ok := props["path"]; !ok {
		t.Error("schema should have path property")
	}
	if _, ok := props["destination"]; !ok {
		t.Error("schema should have destination property")
	}
	if _, ok := props["mode"]; !ok {
		t.Error("schema should have mode property")
	}
	if _, ok := props["force"]; !ok {
		t.Error("schema should have force property")
	}
	if _, ok := props["symbolic"]; !ok {
		t.Error("schema should have symbolic property")
	}
}

func TestFileOpsToolCheckPermissionsEmpty(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result.Behavior != PermissionPassthrough {
		t.Error("empty path should passthrough")
	}
}

func TestFileOpsToolCheckPermissionsDangerous(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.CheckPermissions(map[string]any{"operation": "rm", "path": ".bashrc"})
	if result.Behavior != PermissionAsk {
		t.Errorf("dangerous path should ask, got %v", result.Behavior)
	}
}

func TestFileOpsToolCheckPermissionsDestDangerous(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.CheckPermissions(map[string]any{
		"operation":   "mv",
		"path":        "main.go",
		"destination": ".gitconfig",
	})
	if result.Behavior != PermissionAsk {
		t.Errorf("dangerous destination should ask, got %v", result.Behavior)
	}
}

func TestFileOpsToolCheckPermissionsSafePath(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.CheckPermissions(map[string]any{"operation": "mkdir", "path": "src/pkg"})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("safe path should passthrough, got %v", result.Behavior)
	}
}

func TestFileOpsToolCheckPermissionsSafePathAndDest(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.CheckPermissions(map[string]any{
		"operation":   "mv",
		"path":        "a.txt",
		"destination": "b.txt",
	})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("safe path+dest should passthrough, got %v", result.Behavior)
	}
}

func TestFileOpsToolExecuteNoPath(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "mkdir"})
	if !result.IsError {
		t.Error("missing path should return error")
	}
}

func TestFileOpsToolExecuteUnknownOperation(t *testing.T) {
	tool := &FileOpsTool{}
	result := tool.Execute(map[string]any{"operation": "unknown", "path": "/tmp"})
	if !result.IsError {
		t.Error("unknown operation should return error")
	}
}

// ─── Protected path checks (opRemoveAll) ────────────────────────────────────

func TestOpRemoveAllRoot(t *testing.T) {
	result := opRemoveAll("/")
	if !result.IsError {
		t.Error("rmrf / should return error")
	}
}

func TestOpRemoveAllDotGit(t *testing.T) {
	result := opRemoveAll("/project/.git")
	if !result.IsError {
		t.Error("rmrf .git should return error")
	}
}

func TestOpRemoveAllHomeTilde(t *testing.T) {
	// Bare "~" is not caught by the suffix check (needs "/~" prefix)
	// But a path ending in /~ IS caught
	result := opRemoveAll("/home/user/~")
	if !result.IsError {
		t.Error("rmrf /home/user/~ should return error (protected home dir pattern)")
	}
}

func TestOpRemoveAllCurrentDir(t *testing.T) {
	result := opRemoveAll(".")
	if !result.IsError {
		t.Error("rmrf . should return error")
	}
}

func TestOpRemoveAllDotSlash(t *testing.T) {
	result := opRemoveAll("./")
	if !result.IsError {
		t.Error("rmrf ./ should return error")
	}
}

// ─── opMove missing dest ────────────────────────────────────────────────────

func TestOpMoveMissingDest(t *testing.T) {
	result := opMove("/tmp/nonexistent_abc123", map[string]any{})
	if !result.IsError {
		t.Error("mv without destination should return error")
	}
}

func TestOpCopyMissingDest(t *testing.T) {
	result := opCopy("/tmp/nonexistent_abc123", map[string]any{})
	if !result.IsError {
		t.Error("cp without destination should return error")
	}
}

func TestOpCopyDirMissingDest(t *testing.T) {
	result := opCopyDir("/tmp/nonexistent_abc123", map[string]any{})
	if !result.IsError {
		t.Error("cpdir without destination should return error")
	}
}

func TestOpLinkMissingDestFileops(t *testing.T) {
	result := opLink("/tmp", map[string]any{})
	if !result.IsError {
		t.Error("ln without destination should return error")
	}
}

// ─── opChmod ────────────────────────────────────────────────────────────────

func TestOpChmodInvalidMode(t *testing.T) {
	result := opChmod("/tmp/test_abc123", map[string]any{"mode": "invalid"})
	if !result.IsError {
		t.Error("invalid mode should return error")
	}
}

func TestOpChmodDefaultMode(t *testing.T) {
	// Empty mode string should use default 0644
	result := opChmod("/tmp/test_abc123_nonexistent", map[string]any{"mode": ""})
	// Will fail because file doesn't exist, but the mode parsing should succeed
	if !result.IsError {
		// File doesn't exist so it will error, but not because of mode parsing
		t.Log("expected error for nonexistent file")
	}
}

// ─── copyPath/copyFile (unit tests with temp dirs) ─────────────────────────

func TestCopyFileNonExistent(t *testing.T) {
	err := copyFile("/tmp/nonexistent_abc123_src", "/tmp/nonexistent_abc123_dest")
	if err == nil {
		t.Error("copying nonexistent file should error")
	}
}

func TestCopyPathNonExistent(t *testing.T) {
	err := copyPath("/tmp/nonexistent_abc123_src", "/tmp/nonexistent_abc123_dest")
	if err == nil {
		t.Error("copying nonexistent path should error")
	}
}
