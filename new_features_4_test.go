package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── Image/Modality Filter Tests ────────────────────────────────────────────

func TestGetDefaultCapabilities_Claude(t *testing.T) {
	caps := GetDefaultCapabilities("claude-sonnet-4-20250514")
	if !caps.SupportsImages {
		t.Error("expected Claude to support images")
	}
	if !caps.SupportsPDF {
		t.Error("expected Claude to support PDF")
	}
}

func TestGetDefaultCapabilities_GPT4(t *testing.T) {
	caps := GetDefaultCapabilities("gpt-4o")
	if !caps.SupportsImages {
		t.Error("expected GPT-4o to support images")
	}
}

func TestGetDefaultCapabilities_Unknown(t *testing.T) {
	caps := GetDefaultCapabilities("unknown-model")
	if caps.SupportsImages {
		t.Error("expected unknown model to not support images")
	}
}

func TestFilterUnsupportedParts(t *testing.T) {
	caps := ModelCapabilitiesInput{SupportsImages: false}
	parts := []ContentPart{
		{Type: "text", Text: "hello"},
		{Type: "image", MimeType: "image/png"},
	}

	result := FilterUnsupportedParts(parts, caps)
	if len(result) != 2 {
		t.Errorf("expected 2 parts, got %d", len(result))
	}
	if result[1].Type != "text" {
		t.Error("expected image to be replaced with text")
	}
}

func TestLimitImages(t *testing.T) {
	parts := []ContentPart{
		{Type: "text", Text: "hello"},
		{Type: "image"},
		{Type: "image"},
		{Type: "image"},
	}

	result := LimitImages(parts, 2, 0)
	// 1 text + 2 images (dropped 1) + drop message = 4
	if len(result) < 3 {
		t.Errorf("expected at least 3 parts, got %d", len(result))
	}
}

// ─── Instruction Resolution Tests ───────────────────────────────────────────

func TestInstructionResolver_New(t *testing.T) {
	dir := t.TempDir()
	r := NewInstructionResolver(dir)
	if r == nil {
		t.Error("expected non-nil resolver")
	}
}

func TestInstructionResolver_Resolve(t *testing.T) {
	dir := t.TempDir()
	r := NewInstructionResolver(dir)

	// Create CLAUDE.md
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Instructions"), 0644)

	files := r.Resolve(dir)
	if len(files) == 0 {
		t.Error("expected at least 1 instruction file")
	}
}

func TestInstructionResolver_ResolveWithFallback(t *testing.T) {
	dir := t.TempDir()
	r := NewInstructionResolver(dir)

	// Create sparse AGENTS.md
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("short"), 0644)
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Detailed instructions"), 0644)

	files := r.ResolveWithFallback(dir)
	if len(files) < 2 {
		t.Errorf("expected at least 2 files, got %d", len(files))
	}
}

func TestFormatInstructionFiles(t *testing.T) {
	files := []*InstructionFile{
		{Path: "/path/CLAUDE.md", Type: "CLAUDE"},
	}

	output := FormatInstructionFiles(files)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestFormatInstructionFiles_Empty(t *testing.T) {
	output := FormatInstructionFiles(nil)
	if output != "No instruction files found." {
		t.Errorf("expected 'No instruction files found.', got %q", output)
	}
}

// ─── Team Collaboration Tests ───────────────────────────────────────────────

