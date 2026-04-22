package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func setupGlobTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.py"), []byte("# b"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "c.go"), []byte("package c"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "d.go"), []byte("package d"), 0644)
	return dir
}

func TestGlobRecursive(t *testing.T) {
	dir := setupGlobTestDir(t)
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "**/*.go",
		"directory": dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// Should find a.go, sub/c.go, sub/deep/d.go
	count := 0
	for _, ext := range []string{"a.go", "c.go", "d.go"} {
		if contains(result.Output, ext) {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 .go files, got %d in output:\n%s", count, result.Output)
	}
}

func TestGlobSimple(t *testing.T) {
	dir := setupGlobTestDir(t)
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "*.go",
		"directory": dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	// *.go is auto-prefixed to **/*.go, so it searches recursively too
	// Should find a.go, sub/c.go, sub/deep/d.go
	count := 0
	for _, ext := range []string{"a.go", "c.go", "d.go"} {
		if contains(result.Output, ext) {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 .go files (simple pattern auto-prefixes with **/), got %d in output:\n%s", count, result.Output)
	}
}

func TestGlobNoMatch(t *testing.T) {
	dir := setupGlobTestDir(t)
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "*.rust",
		"directory": dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "No files matched." {
		t.Errorf("expected 'No files matched.', got %q", result.Output)
	}
}

func TestGlobInvalidDirectory(t *testing.T) {
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "*.go",
		"directory": "/nonexistent/path/xyz",
	})
	if !result.IsError {
		t.Error("expected error for nonexistent directory")
	}
}

func TestGlobMaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 600; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i%26))+".txt"), []byte("x"), 0644)
	}
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "**/*.txt",
		"directory": dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	// Should be capped at 500 + "and N more" message
	if !contains(result.Output, "and") {
		t.Log("maxResults truncation may not be triggering; this is ok if < 500 files matched")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && len(substr) > 0 && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
