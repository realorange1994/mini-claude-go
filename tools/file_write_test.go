package tools

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── FileWriteTool interface ────────────────────────────────────────────────

func TestFileWriteToolName(t *testing.T) {
	tool := &FileWriteTool{}
	if tool.Name() != "write_file" {
		t.Errorf("expected 'write_file', got %q", tool.Name())
	}
}

func TestFileWriteToolSchema(t *testing.T) {
	tool := &FileWriteTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 2 {
		t.Errorf("expected 2 required params, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["file_path"]; !ok {
		t.Error("schema should have file_path property")
	}
	if _, ok := props["content"]; !ok {
		t.Error("schema should have content property")
	}
}

func TestFileWriteToolDescription(t *testing.T) {
	tool := &FileWriteTool{}
	desc := tool.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

// ─── FileWriteTool CheckPermissions ─────────────────────────────────────────

func TestFileWriteToolCheckPermissionsEmpty(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result.Behavior != PermissionPassthrough {
		t.Error("empty path should passthrough")
	}
}

func TestFileWriteToolCheckPermissionsSafe(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.CheckPermissions(map[string]any{"file_path": "main.go"})
	if result.Behavior != PermissionPassthrough {
		t.Error("safe path should passthrough")
	}
}

func TestFileWriteToolCheckPermissionsDangerous(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.CheckPermissions(map[string]any{"file_path": ".gitconfig"})
	if result.Behavior != PermissionAsk {
		t.Errorf("dangerous path should ask, got %v", result.Behavior)
	}
}

func TestFileWriteToolCheckPermissionsUNC(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.CheckPermissions(map[string]any{"file_path": `\\server\share\file.txt`})
	if result.Behavior != PermissionAsk {
		t.Errorf("UNC path should ask, got %v", result.Behavior)
	}
}

func TestFileWriteToolCheckPermissionsClaudeConfig(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.CheckPermissions(map[string]any{"file_path": ".claude/settings.json"})
	if result.Behavior != PermissionAsk {
		t.Error("claude config should ask")
	}
}

// ─── FileWriteTool Execute ──────────────────────────────────────────────────

func TestFileWriteToolExecuteNoPath(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.Execute(map[string]any{"content": "test"})
	if !result.IsError {
		t.Error("missing file_path should return error")
	}
}

func TestFileWriteToolExecuteContentTooLarge(t *testing.T) {
	tool := &FileWriteTool{}
	tooBig := make([]byte, 10*1024*1024+1)
	for i := range tooBig {
		tooBig[i] = 'x'
	}
	result := tool.Execute(map[string]any{
		"file_path": "/tmp/test.txt",
		"content":   string(tooBig),
	})
	if !result.IsError {
		t.Error("too large content should return error")
	}
}

func TestFileWriteToolExecuteUNC(t *testing.T) {
	tool := &FileWriteTool{}
	result := tool.Execute(map[string]any{
		"file_path": `\\server\share\file.txt`,
		"content":   "test",
	})
	if !result.IsError {
		t.Error("UNC path should return error")
	}
}

func TestFileWriteToolExecuteWriteNewFile(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	tool := NewFileWriteTool(reg)
	path := filepath.Join(dir, "newfile.txt")
	result := tool.Execute(map[string]any{
		"file_path": path,
		"content":   "hello world",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read back file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestFileWriteToolExecuteCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	tool := NewFileWriteTool(reg)
	path := filepath.Join(dir, "a", "b", "c", "file.txt")
	result := tool.Execute(map[string]any{
		"file_path": path,
		"content":   "nested",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("file should exist after write")
	}
}

func TestFileWriteToolExecuteRegistryMarksStale(t *testing.T) {
	// When registry has stale info, write should fail
	dir := t.TempDir()
	reg := NewRegistry()
	tool := NewFileWriteTool(reg)
	path := filepath.Join(dir, "stale.txt")

	// First read to register
	reg.MarkFileReadWithContent(path, "old content")

	// Write should succeed (no stale check on first write)
	result := tool.Execute(map[string]any{
		"file_path": path,
		"content":   "new content",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

// ─── Atomic write (temp+rename) ─────────────────────────────────────────────

func TestFileWriteToolAtomicWriteNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	tool := NewFileWriteTool(reg)
	path := filepath.Join(dir, "atomic.txt")

	result := tool.Execute(map[string]any{
		"file_path": path,
		"content":   "atomic content",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// Verify file content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "atomic content" {
		t.Errorf("expected 'atomic content', got %q", string(data))
	}

	// Verify no leftover .tmp files
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp.*"))
	if len(matches) > 0 {
		t.Errorf("leftover temp files found: %v", matches)
	}
}

func TestFileWriteToolAtomicWriteOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry()
	tool := NewFileWriteTool(reg)
	path := filepath.Join(dir, "existing.txt")

	// Create existing file
	os.WriteFile(path, []byte("old content"), 0644)
	// Mark as read so write-before-read check passes
	reg.MarkFileReadWithContent(path, "old content")

	// Overwrite should succeed via rename
	result := tool.Execute(map[string]any{
		"file_path": path,
		"content":   "new content",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("expected 'new content', got %q", string(data))
	}
}

// ─── NewFileWriteTool ───────────────────────────────────────────────────────

func TestNewFileWriteTool(t *testing.T) {
	r := NewRegistry()
	tool := NewFileWriteTool(r)
	if tool == nil {
		t.Fatal("NewFileWriteTool should not return nil")
	}
}
