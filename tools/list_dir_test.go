package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListDirSimple(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{"path": dir})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	if !strings.Contains(result.Output, "a.go") {
		t.Errorf("expected a.go in output: %s", result.Output)
	}
	if !strings.Contains(result.Output, "b.txt") {
		t.Errorf("expected b.txt in output: %s", result.Output)
	}
	if !strings.Contains(result.Output, "sub/") {
		t.Errorf("expected sub/ in output: %s", result.Output)
	}
}

func TestListDirRecursive(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "b.go"), []byte("package b"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "c.go"), []byte("package c"), 0644)

	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{
		"path":      dir,
		"recursive": true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	if !strings.Contains(result.Output, "a.go") {
		t.Errorf("expected a.go: %s", result.Output)
	}
	// Windows uses backslash separators
	hasSubB := strings.Contains(result.Output, "sub/b.go") || strings.Contains(result.Output, "sub\\b.go")
	hasSubDeepC := strings.Contains(result.Output, "sub/deep/c.go") || strings.Contains(result.Output, "sub\\deep\\c.go")
	if !hasSubB {
		t.Errorf("expected sub/b.go: %s", result.Output)
	}
	if !hasSubDeepC {
		t.Errorf("expected sub/deep/c.go: %s", result.Output)
	}
}

func TestListDirIgnoresGitAndNodeModules(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, "node_modules"), 0755)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)

	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{
		"path":      dir,
		"recursive": true,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if strings.Contains(result.Output, ".git") {
		t.Errorf("should not list .git: %s", result.Output)
	}
	if strings.Contains(result.Output, "node_modules") {
		t.Errorf("should not list node_modules: %s", result.Output)
	}
}

func TestListDirEmpty(t *testing.T) {
	dir := t.TempDir()
	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{"path": dir})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "empty") {
		t.Errorf("expected empty directory message, got: %s", result.Output)
	}
}

func TestListDirNotFound(t *testing.T) {
	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{"path": "/nonexistent/path"})
	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
}

func TestListDirNotADirectory(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "file.txt")
	os.WriteFile(fp, []byte("hello"), 0644)

	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{"path": fp})
	if !result.IsError {
		t.Error("expected error for file path")
	}
}

func TestListDirDefaultPath(t *testing.T) {
	tool := &ListDirTool{}
	result := tool.Execute(map[string]any{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("expected non-empty output for current directory")
	}
}
