package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiEditSuccess(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "test.go")
	os.WriteFile(fp, []byte("package main\n\nfunc foo() {}\n\nfunc bar() {}\n"), 0644)

	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{
		"path": fp,
		"edits": []any{
			map[string]any{"old_string": "func foo()", "new_string": "func Foo()"},
			map[string]any{"old_string": "func bar()", "new_string": "func Bar()"},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	if !strings.Contains(string(data), "func Foo()") {
		t.Errorf("expected Foo(), got:\n%s", data)
	}
	if !strings.Contains(string(data), "func Bar()") {
		t.Errorf("expected Bar(), got:\n%s", data)
	}
}

func TestMultiEditAtomicRollback(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "atomic.go")
	original := "package main\n\nfunc foo() {}\n\nfunc bar() {}\n"
	os.WriteFile(fp, []byte(original), 0644)

	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{
		"path": fp,
		"edits": []any{
			map[string]any{"old_string": "func foo()", "new_string": "func Foo()"},
			map[string]any{"old_string": "func missing()", "new_string": "func nope()"},
		},
	})
	if !result.IsError {
		t.Fatal("expected error for edit with non-existent old_string")
	}

	// File should be unchanged (atomic rollback)
	data, _ := os.ReadFile(fp)
	if string(data) != original {
		t.Errorf("file should be unchanged after failed atomic edit:\n%s", data)
	}
}

func TestMultiEditFileNotFound(t *testing.T) {
	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{
		"path": "/nonexistent/file.go",
		"edits": []any{
			map[string]any{"old_string": "x", "new_string": "y"},
		},
	})
	if !result.IsError {
		t.Error("expected error for nonexistent file")
	}
}

func TestMultiEditEmptyEdits(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "empty.go")
	os.WriteFile(fp, []byte("package main"), 0644)

	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{
		"path": fp,
		"edits": []any{},
	})
	if !result.IsError {
		t.Error("expected error for empty edits array")
	}
}

func TestMultiEditMissingRequiredParams(t *testing.T) {
	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing required params")
	}
}
