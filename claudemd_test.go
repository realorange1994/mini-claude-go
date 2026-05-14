package main

import (
	"strings"
	"testing"
)

// Ported from upstream: src/utils/__tests__/claudemd.test.ts
// Tests for stripHtmlComments, isMemoryFilePath, getLargeMemoryFiles

func TestStripHtmlComments(t *testing.T) {
	t.Run("strips block-level HTML comments (own line)", func(t *testing.T) {
		content, stripped := StripHtmlComments("text\n<!-- block comment -->\nmore")
		if stripped != true {
			t.Error("expected stripped=true")
		}
		if contains(content, "block comment") {
			t.Error("comment should be stripped")
		}
	})

	t.Run("returns stripped: false when no comments", func(t *testing.T) {
		content, stripped := StripHtmlComments("no comments here")
		if stripped != false {
			t.Errorf("expected stripped=false, got %v", stripped)
		}
		if content != "no comments here" {
			t.Errorf("expected unchanged content, got %q", content)
		}
	})

	t.Run("returns stripped: true when block comments exist", func(t *testing.T) {
		_, stripped := StripHtmlComments("hello\n<!-- world -->\nend")
		if stripped != true {
			t.Error("expected stripped=true for block comments")
		}
	})

	t.Run("handles empty string", func(t *testing.T) {
		content, stripped := StripHtmlComments("")
		if content != "" {
			t.Errorf("expected empty content, got %q", content)
		}
		if stripped != false {
			t.Error("expected stripped=false for empty string")
		}
	})

	t.Run("handles multiple block comments", func(t *testing.T) {
		content, stripped := StripHtmlComments("a\n<!-- c1 -->\nb\n<!-- c2 -->\nc")
		if contains(content, "c1") {
			t.Error("comment c1 should be stripped")
		}
		if contains(content, "c2") {
			t.Error("comment c2 should be stripped")
		}
		if stripped != true {
			t.Error("expected stripped=true")
		}
	})

	t.Run("preserves code block content", func(t *testing.T) {
		content, _ := StripHtmlComments("text\n```html\n<!-- not stripped -->\n```\nmore")
		if !contains(content, "<!-- not stripped -->") {
			t.Error("comment inside code block should be preserved")
		}
	})

	t.Run("preserves inline comments within paragraphs", func(t *testing.T) {
		content, stripped := StripHtmlComments("text <!-- inline --> more")
		if !contains(content, "<!-- inline -->") {
			t.Error("inline comment should be preserved")
		}
		if stripped != false {
			t.Error("expected stripped=false for inline-only comment")
		}
	})

	t.Run("leaves unclosed HTML comment unchanged", func(t *testing.T) {
		content, stripped := StripHtmlComments("<!-- no close some text")
		if content != "<!-- no close some text" {
			t.Errorf("expected unchanged content, got %q", content)
		}
		if stripped != false {
			t.Error("expected stripped=false for unclosed comment")
		}
	})

	t.Run("strips comment and keeps same-line residual content", func(t *testing.T) {
		content, stripped := StripHtmlComments("<!-- note -->some text")
		if !contains(content, "some text") {
			t.Error("expected residual content to be kept")
		}
		if contains(content, "<!--") {
			t.Error("comment should be stripped")
		}
		if stripped != true {
			t.Error("expected stripped=true")
		}
	})
}

func TestIsMemoryFilePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"returns true for CLAUDE.md path", "/project/CLAUDE.md", true},
		{"returns true for CLAUDE.local.md path", "/project/CLAUDE.local.md", true},
		{"returns true for .claude/rules/ path", "/project/.claude/rules/foo.md", true},
		{"returns false for regular file", "/project/src/main.ts", false},
		{"returns false for unrelated .md file", "/project/README.md", false},
		{"returns false for .claude directory non-rules file", "/project/.claude/settings.json", false},
		{"returns false for lowercase claude.md (case-sensitive match)", "/project/claude.md", false},
		{"returns false for non-.md file in .claude/rules/", ".claude/rules/foo.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMemoryFilePath(tt.path)
			if got != tt.want {
				t.Errorf("IsMemoryFilePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGetLargeMemoryFiles(t *testing.T) {
	t.Run("returns files exceeding threshold", func(t *testing.T) {
		largeContent := repeatStr("x", MaxMemoryCharacterCount+1)
		files := []MemoryFileInfo{
			{Path: "/project/CLAUDE.md", Type: "Project", Content: "small"},
			{Path: "/big.md", Type: "Project", Content: largeContent},
		}
		result := GetLargeMemoryFiles(files)
		if len(result) != 1 {
			t.Fatalf("expected 1 file, got %d", len(result))
		}
		if result[0].Path != "/big.md" {
			t.Errorf("expected /big.md, got %s", result[0].Path)
		}
	})

	t.Run("returns empty array when all files are small", func(t *testing.T) {
		files := []MemoryFileInfo{
			{Path: "/a.md", Type: "Project", Content: "small"},
			{Path: "/b.md", Type: "Project", Content: "also small"},
		}
		result := GetLargeMemoryFiles(files)
		if len(result) != 0 {
			t.Errorf("expected empty array, got %d files", len(result))
		}
	})

	t.Run("correctly identifies threshold boundary", func(t *testing.T) {
		atThreshold := repeatStr("x", MaxMemoryCharacterCount)
		overThreshold := repeatStr("x", MaxMemoryCharacterCount+1)
		files := []MemoryFileInfo{
			{Path: "/a.md", Type: "Project", Content: atThreshold},
			{Path: "/b.md", Type: "Project", Content: overThreshold},
		}
		result := GetLargeMemoryFiles(files)
		if len(result) != 1 {
			t.Errorf("expected 1 file (boundary is not exceeding), got %d", len(result))
		}
	})

	t.Run("returns empty array for empty input", func(t *testing.T) {
		result := GetLargeMemoryFiles([]MemoryFileInfo{})
		if len(result) != 0 {
			t.Errorf("expected empty array for empty input, got %d", len(result))
		}
	})
}

func repeatStr(s string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(s)
	}
	return b.String()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && (s == substr || len(s) > len(substr) && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	// Simple substring search
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
