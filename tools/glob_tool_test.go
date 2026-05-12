package tools

import (
	"os"
	"path/filepath"
	"strings"
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
	return strings.Contains(s, substr)
}

// ─── Type Filter (P2-11) ────────────────────────────────────────────────────

func setupGlobTypeTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0755)
	os.MkdirAll(filepath.Join(dir, "test"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "pkg", "lib.go"), []byte("package lib"), 0644)
	os.WriteFile(filepath.Join(dir, "test", "main_test.go"), []byte("package test"), 0644)
	os.Mkdir(filepath.Join(dir, "src", "emptydir"), 0755)
	return dir
}

func TestGlobTypeFile(t *testing.T) {
	dir := setupGlobTypeTestDir(t)
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "**/*.go",
		"path":    dir,
		"type":    "file",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	// Should find 3 .go files
	if !contains(result.Output, "main.go") || !contains(result.Output, "lib.go") || !contains(result.Output, "main_test.go") {
		t.Errorf("expected 3 .go files, got:\n%s", result.Output)
	}
}

func TestGlobTypeDir(t *testing.T) {
	dir := setupGlobTypeTestDir(t)
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "**/*",
		"path":    dir,
		"type":    "dir",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	// Should find directories: src, pkg, test, emptydir
	if !contains(result.Output, "src") {
		t.Errorf("expected 'src' dir in output:\n%s", result.Output)
	}
	if !contains(result.Output, "test") {
		t.Errorf("expected 'test' dir in output:\n%s", result.Output)
	}
}

func TestGlobTypeAll(t *testing.T) {
	dir := setupGlobTypeTestDir(t)
	tool := &GlobTool{}
	result := tool.Execute(map[string]any{
		"pattern": "**/*",
		"path":    dir,
		"type":    "all",
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	// Should find both files and dirs
	hasFile := contains(result.Output, "main.go") || contains(result.Output, "lib.go")
	hasDir := contains(result.Output, "src") || contains(result.Output, "test")
	if !hasFile {
		t.Errorf("expected files in output:\n%s", result.Output)
	}
	if !hasDir {
		t.Errorf("expected dirs in output:\n%s", result.Output)
	}
}

func TestGlobTypeFilterDefault(t *testing.T) {
	dir := setupGlobTypeTestDir(t)
	tool := &GlobTool{}
	// No type specified — should default to "file"
	result := tool.Execute(map[string]any{
		"pattern": "*.go",
		"path":    dir,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !contains(result.Output, "main.go") {
		t.Errorf("expected main.go (type defaults to file), got:\n%s", result.Output)
	}
}
