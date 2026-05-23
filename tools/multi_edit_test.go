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
		"file_path": fp,
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
		"file_path": fp,
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
		"file_path": fp,
		"edits":     []any{},
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

func TestMultiEditMultipleMatchesWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "dup.go")
	// File contains "func foo()" twice
	os.WriteFile(fp, []byte("package main\n\nfunc foo() {}\n\nfunc foo() {}\n"), 0644)

	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{
		"file_path": fp,
		"edits": []any{
			map[string]any{"old_string": "func foo()", "new_string": "func Foo()"},
		},
	})
	if !result.IsError {
		t.Fatal("expected error for multiple matches without replace_all")
	}
	if !strings.Contains(result.Output, "multiple matches") {
		t.Errorf("error should mention multiple matches, got: %s", result.Output)
	}

	// File should be unchanged (atomic rollback)
	data, _ := os.ReadFile(fp)
	if !strings.Contains(string(data), "func foo()") || strings.Count(string(data), "func foo()") != 2 {
		t.Errorf("file should be unchanged after failed edit:\n%s", data)
	}
}

func TestMultiEditMultipleMatchesWithReplaceAll(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "dup2.go")
	// File contains "func foo()" twice
	os.WriteFile(fp, []byte("package main\n\nfunc foo() {}\n\nfunc foo() {}\n"), 0644)

	tool := &MultiEditTool{}
	result := tool.Execute(map[string]any{
		"file_path": fp,
		"edits": []any{
			map[string]any{"old_string": "func foo()", "new_string": "func Foo()", "replace_all": true},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	if strings.Contains(string(data), "func foo()") {
		t.Errorf("all occurrences should be replaced, got:\n%s", data)
	}
	if strings.Count(string(data), "func Foo()") != 2 {
		t.Errorf("expected 2 occurrences of Foo(), got:\n%s", data)
	}
}

// ─── Regression: multi_edit two-phase validation (Bug 7) ──────────────────────
// Previously, the "dry run" loop actually applied edits to content progressively.
// If a later edit failed, the earlier edits had already modified the content variable.
// Now Phase 1 validates on a clone (testContent), Phase 2 applies to real content.
// This ensures the original content is untouched if any edit fails validation.

func TestMultiEditTwoPhaseValidationPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "twophase.go")
	original := "package main\n\nfunc alpha() {}\n\nfunc beta() {}\n"
	os.WriteFile(fp, []byte(original), 0644)

	tool := &MultiEditTool{}
	// First edit succeeds on testContent, second edit fails
	result := tool.Execute(map[string]any{
		"file_path": fp,
		"edits": []any{
			map[string]any{"old_string": "func alpha()", "new_string": "func Alpha()"},
			map[string]any{"old_string": "func nonexistent()", "new_string": "func Nope()"},
		},
	})
	if !result.IsError {
		t.Fatal("expected error for second edit failing")
	}

	// File should be completely unchanged (not partially modified)
	data, _ := os.ReadFile(fp)
	if string(data) != original {
		t.Errorf("file should be completely unchanged after failed multi_edit:\n%s", data)
	}
}

func TestMultiEditChainedEditsStillWork(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "chained.go")
	os.WriteFile(fp, []byte("package main\n\nconst foo = \"hello\"\n\nfunc bar() { return 2 }\n"), 0644)

	tool := &MultiEditTool{}
	// Multiple independent edits that both succeed
	result := tool.Execute(map[string]any{
		"file_path": fp,
		"edits": []any{
			map[string]any{"old_string": "func bar()", "new_string": "func Bar()"},
			map[string]any{"old_string": "\"hello\"", "new_string": "\"world\""},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	if !strings.Contains(string(data), "func Bar()") {
		t.Errorf("expected first edit applied, got:\n%s", data)
	}
	if !strings.Contains(string(data), "\"world\"") {
		t.Errorf("expected second edit applied, got:\n%s", data)
	}
}

func TestMultiEditOverlappingDetectionOnOriginal(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "overlap.go")
	os.WriteFile(fp, []byte("package main\n\nfunc foo() {}\n\nfunc bar() {}\n"), 0644)

	tool := &MultiEditTool{}
	// Edit 1 replaces "func foo()" with "func bar()", edit 2 tries to find "func bar()"
	// which is now a substring of edit 1's output — overlapping detection should catch this
	result := tool.Execute(map[string]any{
		"file_path": fp,
		"edits": []any{
			map[string]any{"old_string": "func foo()", "new_string": "func bar()"},
			map[string]any{"old_string": "func bar()", "new_string": "func baz()"},
		},
	})
	if !result.IsError {
		t.Fatal("expected error for overlapping edit")
	}
	if !strings.Contains(result.Output, "substring") {
		t.Errorf("expected overlapping edit detection message, got: %s", result.Output)
	}

	// File should be unchanged
	data, _ := os.ReadFile(fp)
	if !strings.Contains(string(data), "func foo()") || !strings.Contains(string(data), "func bar()") {
		t.Errorf("file should be unchanged after overlapping edit rejection:\n%s", data)
	}
}
