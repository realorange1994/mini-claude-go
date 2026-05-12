package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestNotebook(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	nb := nbDocument{
		NbFormat:      4,
		NbFormatMinor: 5,
		Metadata:      map[string]any{},
		Cells: []nbCell{
			{CellType: "markdown", Source: []string{"# Title\n"}, ID: "cell-0", Metadata: map[string]any{}},
			{CellType: "code", Source: []string{"print('hello')\n"}, ID: "cell-1", Metadata: map[string]any{}, Outputs: []any{}, ExecutionCount: nil},
			{CellType: "code", Source: []string{"x = 1\n"}, ID: "cell-2", Metadata: map[string]any{}, Outputs: []any{}, ExecutionCount: nil},
		},
	}
	data, _ := json.MarshalIndent(nb, "", "  ")
	fp := filepath.Join(dir, "test.ipynb")
	os.WriteFile(fp, data, 0644)
	return fp
}

func TestNotebookEditName(t *testing.T) {
	tool := &NotebookEditTool{}
	if tool.Name() != "notebook_edit" {
		t.Errorf("expected name 'notebook_edit', got '%s'", tool.Name())
	}
}

func TestNotebookEditRejectsNonIpynb(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)

	result := tool.Execute(map[string]any{
		"notebook_path": "test.py",
		"cell_id":       "cell-0",
		"new_source":    "hello",
		"edit_mode":     "replace",
	})
	if !result.IsError {
		t.Error("expected error for non-.ipynb file")
	}
	if !containsStr(result.Output, ".ipynb") {
		t.Errorf("expected .ipynb in error, got: %s", result.Output)
	}
}

func TestNotebookEditRejectsNoPath(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)

	result := tool.Execute(map[string]any{
		"cell_id":   "cell-0",
		"new_source": "hello",
	})
	if !result.IsError {
		t.Error("expected error for missing path")
	}
}

func TestNotebookEditRequiresReadFirst(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"new_source":    "new content",
		"edit_mode":     "replace",
	})
	if !result.IsError {
		t.Error("expected error for file not read first")
	}
	if !containsStr(result.Output, "read_file") {
		t.Errorf("expected read_file hint in error, got: %s", result.Output)
	}
}

func TestNotebookEditReplaceCell(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	// Mark file as read (fromRead=true, simulating read_file)
	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-1",
		"new_source":    "print('world')\n",
		"edit_mode":     "replace",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
	if !containsStr(result.Output, "Replaced cell") {
		t.Errorf("expected 'Replaced cell' in output, got: %s", result.Output)
	}

	// Verify the notebook was saved correctly
	data, _ := os.ReadFile(fp)
	var nb nbDocument
	json.Unmarshal(data, &nb)
	if len(nb.Cells) != 3 {
		t.Errorf("expected 3 cells, got %d", len(nb.Cells))
	}
	src := sourceString(nb.Cells[1].Source)
	if !containsStr(src, "world") {
		t.Errorf("expected 'world' in cell 1 source, got: %s", src)
	}
}

func TestNotebookEditReplaceWithCellType(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-1",
		"new_source":    "# Markdown cell\n",
		"edit_mode":     "replace",
		"cell_type":     "markdown",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	var nb nbDocument
	json.Unmarshal(data, &nb)
	if nb.Cells[1].CellType != "markdown" {
		t.Errorf("expected cell_type 'markdown', got: %s", nb.Cells[1].CellType)
	}
}

func TestNotebookEditInsertCell(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-1",
		"new_source":    "import os\n",
		"edit_mode":     "insert",
		"cell_type":     "code",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
	if !containsStr(result.Output, "Inserted new cell") {
		t.Errorf("expected 'Inserted new cell' in output, got: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	var nb nbDocument
	json.Unmarshal(data, &nb)
	if len(nb.Cells) != 4 {
		t.Errorf("expected 4 cells after insert, got %d", len(nb.Cells))
	}
	// New cell should be at index 1
	src := sourceString(nb.Cells[1].Source)
	if !containsStr(src, "import os") {
		t.Errorf("expected 'import os' in new cell, got: %s", src)
	}
}

func TestNotebookEditDeleteCell(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-1",
		"edit_mode":     "delete",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}
	if !containsStr(result.Output, "Deleted cell") {
		t.Errorf("expected 'Deleted cell' in output, got: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	var nb nbDocument
	json.Unmarshal(data, &nb)
	if len(nb.Cells) != 2 {
		t.Errorf("expected 2 cells after delete, got %d", len(nb.Cells))
	}
}

func TestNotebookEditDeleteNotFound(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-999",
		"edit_mode":     "delete",
	})
	if !result.IsError {
		t.Error("expected error for cell not found in delete mode")
	}
}

func TestNotebookEditInsertRequiresCellType(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"new_source":    "hello",
		"edit_mode":     "insert",
	})
	if !result.IsError {
		t.Error("expected error for missing cell_type in insert mode")
	}
}

func TestNotebookEditReplaceRequiresNewSource(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"edit_mode":     "replace",
	})
	if !result.IsError {
		t.Error("expected error for missing new_source in replace mode")
	}
}

func TestNotebookEditAutoPromoteReplaceToInsert(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	// Replace a cell that doesn't exist — should auto-promote to insert at end
	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-999",
		"new_source":    "# New cell\n",
		"edit_mode":     "replace",
		"cell_type":     "markdown",
	})
	if result.IsError {
		t.Errorf("expected success (auto-promote to insert), got: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	var nb nbDocument
	json.Unmarshal(data, &nb)
	if len(nb.Cells) != 4 {
		t.Errorf("expected 4 cells after auto-promote insert, got %d", len(nb.Cells))
	}
}

func TestNotebookEditInvalidEditMode(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"new_source":    "hello",
		"edit_mode":     "invalid",
	})
	if !result.IsError {
		t.Error("expected error for invalid edit_mode")
	}
}

func TestNotebookEditBackupCreated(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"new_source":    "new content",
		"edit_mode":     "replace",
	})

	// Check backup file exists
	if _, err := os.Stat(fp + ".bak"); err != nil {
		t.Error("expected backup file (.ipynb.bak) to be created")
	}
}

func TestNotebookEditClearsOutputsOnCodeReplace(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	// Replace a code cell — outputs should be cleared
	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-1",
		"new_source":    "x = 42\n",
		"edit_mode":     "replace",
	})
	if result.IsError {
		t.Errorf("expected success, got: %s", result.Output)
	}

	data, _ := os.ReadFile(fp)
	var nb nbDocument
	json.Unmarshal(data, &nb)
	cell := nb.Cells[1]
	if cell.Outputs != nil {
		t.Errorf("expected outputs to be cleared after replace, got: %v", cell.Outputs)
	}
}

func TestNotebookEditFileModifiedSinceRead(t *testing.T) {
	r := NewRegistry()
	tool := NewNotebookEditTool(r)
	fp := createTestNotebook(t)

	r.MarkFileReadWithParams(fp, -1, -1, "", false, true)

	// Modify the file after it was read — set mtime to future
	futureTime := time.Now().Add(1 * time.Hour)
	os.Chtimes(fp, futureTime, futureTime)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"new_source":    "new content",
		"edit_mode":     "replace",
	})
	if !result.IsError {
		t.Error("expected error for file modified since read")
	}
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}