package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePatch_AddFile(t *testing.T) {
	patch := `*** Begin Patch
*** Add File: test.go
package main
*** End Patch`

	files, err := ParsePatch(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Operation != PatchAdd {
		t.Errorf("expected add, got %s", files[0].Operation)
	}
	if files[0].Path != "test.go" {
		t.Errorf("expected test.go, got %s", files[0].Path)
	}
}

func TestParsePatch_UpdateFile(t *testing.T) {
	patch := `*** Begin Patch
*** Update File: main.go
new content
*** End Patch`

	files, err := ParsePatch(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Operation != PatchUpdate {
		t.Errorf("expected update, got %s", files[0].Operation)
	}
}

func TestParsePatch_DeleteFile(t *testing.T) {
	patch := `*** Begin Patch
*** Delete File: old.go
*** End Patch`

	files, err := ParsePatch(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Operation != PatchDelete {
		t.Errorf("expected delete, got %s", files[0].Operation)
	}
}

func TestParsePatch_MoveFile(t *testing.T) {
	patch := `*** Begin Patch
*** Move File: old.go to new.go
*** End Patch`

	files, err := ParsePatch(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Operation != PatchMove {
		t.Errorf("expected move, got %s", files[0].Operation)
	}
	if files[0].NewPath != "new.go" {
		t.Errorf("expected new.go, got %s", files[0].NewPath)
	}
}

func TestApplyPatch_AddFile(t *testing.T) {
	dir := t.TempDir()
	patch := `*** Begin Patch
*** Add File: test.go
package main
*** End Patch`

	result, err := ApplyPatch(patch, dir)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", result.FilesChanged)
	}

	// Verify file exists
	path := filepath.Join(dir, "test.go")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist")
	}
}

func TestApplyPatch_UpdateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	os.WriteFile(path, []byte("old content"), 0644)

	patch := `*** Begin Patch
*** Update File: main.go
new content
*** End Patch`

	result, err := ApplyPatch(patch, dir)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", result.FilesChanged)
	}

	// Verify content
	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Errorf("expected 'new content', got %q", string(data))
	}
}

func TestApplyPatch_DeleteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.go")
	os.WriteFile(path, []byte("content"), 0644)

	patch := `*** Begin Patch
*** Delete File: old.go
*** End Patch`

	result, err := ApplyPatch(patch, dir)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", result.FilesChanged)
	}

	// Verify file deleted
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestApplyPatch_MoveFile(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.go")
	os.WriteFile(oldPath, []byte("content"), 0644)

	patch := `*** Begin Patch
*** Move File: old.go to new.go
*** End Patch`

	result, err := ApplyPatch(patch, dir)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", result.FilesChanged)
	}

	// Verify move
	newPath := filepath.Join(dir, "new.go")
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("expected new file to exist")
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("expected old file to be deleted")
	}
}

func TestFormatPatchResult(t *testing.T) {
	result := &PatchResult{
		FilesChanged: 2,
		Additions:    10,
		Deletions:    5,
		Changes: []PatchChange{
			{Path: "main.go", Operation: PatchUpdate, Additions: 8, Deletions: 5},
			{Path: "utils.go", Operation: PatchAdd, Additions: 2},
		},
	}

	output := FormatPatchResult(result)
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello", 1},
		{"hello\nworld", 2},
		{"a\nb\nc", 3},
	}

	for _, tt := range tests {
		result := countLines(tt.input)
		if result != tt.expected {
			t.Errorf("countLines(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}
