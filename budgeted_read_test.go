package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBudgeted_SmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("hello world"), 0644)

	result, err := ReadBudgeted(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Truncated {
		t.Error("expected no truncation for small file")
	}
	if result.Text != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Text)
	}
}

func TestReadBudgeted_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	// Create a file with ~400 tokens (1600 chars)
	content := strings.Repeat("This is a test line with enough chars.\n", 40)
	os.WriteFile(path, []byte(content), 0644)

	result, err := ReadBudgeted(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected truncation for large file")
	}
	if !strings.Contains(result.Text, "⚠️ Truncated") {
		t.Error("expected truncation hint")
	}
	if !strings.Contains(result.Text, "offset=") {
		t.Error("expected offset hint")
	}
}

func TestReadBudgeted_NonExistentFile(t *testing.T) {
	_, err := ReadBudgeted("/nonexistent/file.md", 100)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestReadBudgetedSectionAware_SmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "## Section 1\nbody content\n## Section 2\nmore content"
	os.WriteFile(path, []byte(content), 0644)

	result, err := ReadBudgetedSectionAware(path, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Truncated {
		t.Error("expected no truncation for small file")
	}
}

func TestReadBudgetedSectionAware_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	// Create structured content with sections
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("## Section " + strings.Repeat("x", 10) + "\n")
		sb.WriteString("_italic description_\n")
		sb.WriteString("- See file.md (10 entries)\n")
		sb.WriteString(strings.Repeat("Body content line with enough chars.\n", 10))
	}
	os.WriteFile(path, []byte(sb.String()), 0644)

	result, err := ReadBudgetedSectionAware(path, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected truncation for large file")
	}
	if !strings.Contains(result.Text, "## Section") {
		t.Error("expected section headers to be preserved")
	}
}

func TestReadBudgetedSectionAware_SkeletonMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	// Create very large structured content
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("## Section " + strings.Repeat("x", 50) + "\n")
		sb.WriteString("_italic description_\n")
		sb.WriteString(strings.Repeat("Body content line with enough chars.\n", 20))
	}
	os.WriteFile(path, []byte(sb.String()), 0644)

	result, err := ReadBudgetedSectionAware(path, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Error("expected truncation")
	}
	if !strings.Contains(result.Text, "Only structure shown") {
		t.Error("expected skeleton mode hint")
	}
}

func TestEstimateTokensBudgeted(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hi", 1},
		{"hello", 2},
		{"hello world", 3},
	}

	for _, tt := range tests {
		result := estimateTokensBudgeted(tt.input)
		if result != tt.expected {
			t.Errorf("estimateTokensBudgeted(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestParseSections(t *testing.T) {
	text := "preamble line\n## Section 1\n_italic_\n- See file.md (5)\nbody\n## Section 2\nmore body"
	preamble, sections := ParseSections(text)

	if len(preamble) != 1 {
		t.Errorf("expected 1 preamble line, got %d", len(preamble))
	}
	if len(sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Header != "## Section 1" {
		t.Errorf("expected '## Section 1', got %q", sections[0].Header)
	}
	if sections[0].Italic != "_italic_" {
		t.Errorf("expected '_italic_', got %q", sections[0].Italic)
	}
	if len(sections[0].IndexLines) != 1 {
		t.Errorf("expected 1 index line, got %d", len(sections[0].IndexLines))
	}
}
