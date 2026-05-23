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
		"cell_id":    "cell-0",
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
	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

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

// ============================================================================
// Upstream Quality: Cell ID Parsing Edge Cases (port from parseCellId tests)
// ============================================================================

func TestFindCellIndexCellNFormat(t *testing.T) {
	cells := []nbCell{
		{CellType: "code", Source: []string{"a = 1\n"}, ID: "cell-0"},
		{CellType: "code", Source: []string{"b = 2\n"}, ID: "cell-1"},
		{CellType: "markdown", Source: []string{"# Title\n"}, ID: "cell-2"},
	}

	tests := []struct {
		name   string
		cellID string
		want   int
	}{
		{"cell-0", "cell-0", 0},
		{"cell-1", "cell-1", 1},
		{"cell-2", "cell-2", 2},
		{"cell-100 (out of range)", "cell-100", -1},
		{"cell- (no number)", "cell-", -1},
		{"cell-abc (non-numeric)", "cell-abc", -1},
		{"other-format", "other-format", -1},
		// Note: empty string "" matches cell 0 via source substring fallback
		// (strings.Contains(src, "") is always true), so Go returns 0 not -1
		{"empty string", "", 0},
		// Note: "cell-0-extra" -- fmt.Sscanf("cell-%d") parses idx=0 (trailing text ignored)
		{"cell-0-extra (trailing text)", "cell-0-extra", 0},
		{"cell--1 (negative)", "cell--1", -1},
		{"actual cell ID match", "cell-0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findCellIndex(cells, tt.cellID)
			if got != tt.want {
				t.Errorf("findCellIndex(cells, %q) = %d, want %d", tt.cellID, got, tt.want)
			}
		})
	}
}

func TestFindCellIndexLeadingZeros(t *testing.T) {
	// fmt.Sscanf parses "007" as 7, so cell-007 should match index 7
	cells := make([]nbCell, 10)
	for i := range cells {
		cells[i] = nbCell{CellType: "code", Source: []string{""}, ID: ""}
	}

	got := findCellIndex(cells, "cell-007")
	if got != 7 {
		t.Errorf("findCellIndex with cell-007 = %d, want 7", got)
	}
}

func TestFindCellIndexActualCellID(t *testing.T) {
	cells := []nbCell{
		{CellType: "code", Source: []string{"a = 1\n"}, ID: "abc-123"},
		{CellType: "code", Source: []string{"b = 2\n"}, ID: "xyz-456"},
	}

	// Match by actual cell ID (not cell-N format)
	got := findCellIndex(cells, "xyz-456")
	if got != 1 {
		t.Errorf("findCellIndex by actual ID = %d, want 1", got)
	}
}

func TestFindCellIndexSourcePrefixMatch(t *testing.T) {
	cells := []nbCell{
		{CellType: "code", Source: []string{"import numpy as np\n"}, ID: ""},
		{CellType: "code", Source: []string{"import pandas as pd\n"}, ID: ""},
	}

	// Source prefix match: "import numpy" should match first cell
	got := findCellIndex(cells, "import numpy")
	if got != 0 {
		t.Errorf("findCellIndex by source prefix = %d, want 0", got)
	}
}

func TestFindCellIndexSourceSubstringMatch(t *testing.T) {
	cells := []nbCell{
		{CellType: "code", Source: []string{"x = 1\n"}, ID: ""},
		{CellType: "code", Source: []string{"y = x + 1\n"}, ID: ""},
	}

	// Source substring match
	got := findCellIndex(cells, "x + 1")
	if got != 1 {
		t.Errorf("findCellIndex by source substring = %d, want 1", got)
	}
}

func TestFindCellIndexPriorityCellIDOverSource(t *testing.T) {
	// cell-N format should take priority over source substring matching
	cells := []nbCell{
		{CellType: "code", Source: []string{"cell-0 is here\n"}, ID: "real-id-0"},
		{CellType: "code", Source: []string{"cell-1 is here\n"}, ID: "real-id-1"},
	}

	// cell-0 should match index 0 via cell-N format, not source substring
	got := findCellIndex(cells, "cell-0")
	if got != 0 {
		t.Errorf("findCellIndex should prefer cell-N format: got %d, want 0", got)
	}
}

func TestSourceStringString(t *testing.T) {
	got := sourceString("hello world")
	if got != "hello world" {
		t.Errorf("sourceString(string) = %q, want %q", got, "hello world")
	}
}

func TestSourceStringSliceAny(t *testing.T) {
	src := []any{"line1\n", "line2\n", "line3"}
	got := sourceString(src)
	want := "line1\nline2\nline3"
	if got != want {
		t.Errorf("sourceString([]any) = %q, want %q", got, want)
	}
}

func TestSourceStringSliceString(t *testing.T) {
	src := []string{"a\n", "b\n"}
	got := sourceString(src)
	if got != "a\nb\n" {
		t.Errorf("sourceString([]string) = %q, want %q", got, "a\nb\n")
	}
}

func TestSourceStringNil(t *testing.T) {
	got := sourceString(nil)
	if got != "" {
		t.Errorf("sourceString(nil) = %q, want empty", got)
	}
}

// ============================================================================
// Upstream Quality: Notebook Cell Merging (port from mapNotebookCells tests)
// ============================================================================

func TestNotebookEditMultipleCellsPreserved(t *testing.T) {
	// Port of "merges adjacent text blocks" invariant: multiple cells should
	// be preserved as separate entities after edit operations
	r := NewRegistry()
	tool := NewNotebookEditTool(r)

	dir := t.TempDir()
	nb := nbDocument{
		NbFormat:      4,
		NbFormatMinor: 5,
		Cells: []nbCell{
			{CellType: "code", Source: []string{"a = 1\n"}, ID: "cell-0"},
			{CellType: "code", Source: []string{"b = 2\n"}, ID: "cell-1"},
			{CellType: "code", Source: []string{"c = 3\n"}, ID: "cell-2"},
		},
	}
	data, _ := json.MarshalIndent(nb, "", "  ")
	fp := filepath.Join(dir, "test.ipynb")
	os.WriteFile(fp, data, 0644)
	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

	// Edit middle cell
	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-1",
		"new_source":    "b = 20\n",
		"edit_mode":     "replace",
	})
	if result.IsError {
		t.Fatalf("expected success, got: %s", result.Output)
	}

	// Verify all cells preserved
	data, _ = os.ReadFile(fp)
	var saved nbDocument
	json.Unmarshal(data, &saved)
	if len(saved.Cells) != 3 {
		t.Errorf("expected 3 cells preserved, got %d", len(saved.Cells))
	}
	if sourceString(saved.Cells[0].Source) != "a = 1\n" {
		t.Errorf("cell 0 should be unchanged, got: %q", sourceString(saved.Cells[0].Source))
	}
	if !containsStr(sourceString(saved.Cells[1].Source), "b = 20") {
		t.Errorf("cell 1 should be updated, got: %q", sourceString(saved.Cells[1].Source))
	}
	if sourceString(saved.Cells[2].Source) != "c = 3\n" {
		t.Errorf("cell 2 should be unchanged, got: %q", sourceString(saved.Cells[2].Source))
	}
}

func TestNotebookEditPreservesCellTypes(t *testing.T) {
	// Markdown cells should keep their type when not explicitly changed
	r := NewRegistry()
	tool := NewNotebookEditTool(r)

	dir := t.TempDir()
	nb := nbDocument{
		NbFormat:      4,
		NbFormatMinor: 5,
		Cells: []nbCell{
			{CellType: "markdown", Source: []string{"# Title\n"}, ID: "cell-0"},
			{CellType: "code", Source: []string{"print(1)\n"}, ID: "cell-1"},
			{CellType: "markdown", Source: []string{"## Conclusion\n"}, ID: "cell-2"},
		},
	}
	data, _ := json.MarshalIndent(nb, "", "  ")
	fp := filepath.Join(dir, "test.ipynb")
	os.WriteFile(fp, data, 0644)
	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

	// Edit markdown cell without specifying cell_type
	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-0",
		"new_source":    "# New Title\n",
		"edit_mode":     "replace",
	})
	if result.IsError {
		t.Fatalf("expected success, got: %s", result.Output)
	}

	data, _ = os.ReadFile(fp)
	var saved nbDocument
	json.Unmarshal(data, &saved)
	if saved.Cells[0].CellType != "markdown" {
		t.Errorf("markdown cell type should be preserved, got: %s", saved.Cells[0].CellType)
	}
	if saved.Cells[2].CellType != "markdown" {
		t.Errorf("untouched markdown cell type should be preserved, got: %s", saved.Cells[2].CellType)
	}
}

func TestNotebookEditInsertAtEnd(t *testing.T) {
	// Insert at position beyond last cell should append
	r := NewRegistry()
	tool := NewNotebookEditTool(r)

	dir := t.TempDir()
	nb := nbDocument{
		NbFormat:      4,
		NbFormatMinor: 5,
		Cells: []nbCell{
			{CellType: "code", Source: []string{"a = 1\n"}, ID: "cell-0"},
		},
	}
	data, _ := json.MarshalIndent(nb, "", "  ")
	fp := filepath.Join(dir, "test.ipynb")
	os.WriteFile(fp, data, 0644)
	r.MarkFileReadWithParams(fp, -1, -1, "", false, false, true)

	result := tool.Execute(map[string]any{
		"notebook_path": fp,
		"cell_id":       "cell-999", // beyond range
		"new_source":    "b = 2\n",
		"edit_mode":     "insert",
		"cell_type":     "code",
	})
	if result.IsError {
		t.Fatalf("expected success, got: %s", result.Output)
	}

	data, _ = os.ReadFile(fp)
	var saved nbDocument
	json.Unmarshal(data, &saved)
	if len(saved.Cells) != 2 {
		t.Fatalf("expected 2 cells after insert at end, got %d", len(saved.Cells))
	}
	// New cell should be at the end
	if sourceString(saved.Cells[1].Source) != "b = 2\n" {
		t.Errorf("inserted cell should be at end, got: %q", sourceString(saved.Cells[1].Source))
	}
}
