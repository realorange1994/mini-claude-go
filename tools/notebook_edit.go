package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NotebookEditTool provides cell-level editing for Jupyter notebooks (.ipynb).
type NotebookEditTool struct {
	registry *Registry
}

func NewNotebookEditTool(registry *Registry) *NotebookEditTool {
	return &NotebookEditTool{registry: registry}
}

func (*NotebookEditTool) Name() string { return "notebook_edit" }

func (*NotebookEditTool) Description() string {
	return "Edit a Jupyter Notebook (.ipynb) at the cell level. Supports replace, insert, and delete operations on cells. Use read_file first to see the current notebook structure and cell IDs."
}

func (*NotebookEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"notebook_path": map[string]any{
				"type":        "string",
				"description": "Path to the Jupyter Notebook file (.ipynb). Must be read with read_file first.",
			},
			"cell_id": map[string]any{
				"type":        "string",
				"description": "Cell ID to operate on. Can be the actual cell ID or index format 'cell-N' (e.g., 'cell-0', 'cell-1'). For insert mode, specifies the position to insert before.",
			},
			"new_source": map[string]any{
				"type":        "string",
				"description": "New source code/text for the cell. Required for replace and insert modes.",
			},
			"cell_type": map[string]any{
				"type":        "string",
				"description": "Cell type: 'code' or 'markdown'. Optional for replace (keeps existing if not specified), required for insert.",
				"enum":        []string{"code", "markdown"},
			},
			"edit_mode": map[string]any{
				"type":        "string",
				"description": "Edit mode: 'replace' (default), 'insert' (insert before target cell), or 'delete' (remove target cell).",
				"enum":        []string{"replace", "insert", "delete"},
			},
		},
		"required": []string{"notebook_path", "cell_id"},
	}
}

func (*NotebookEditTool) CheckPermissions(params map[string]any) PermissionResult {
	path, _ := params["notebook_path"].(string)
	if path == "" {
		return PermissionResultDeny("notebook_path is required")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".ipynb") {
		return PermissionResultDeny("notebook_edit only works on .ipynb files")
	}
	return PermissionResultPassthrough()
}

type nbCell struct {
	CellType       string `json:"cell_type"`
	Source         any    `json:"source"`
	Outputs        []any  `json:"outputs,omitempty"`
	ExecutionCount *any   `json:"execution_count,omitempty"`
	ID             string `json:"id,omitempty"`
	Metadata       any    `json:"metadata,omitempty"`
}

type nbDocument struct {
	Cells       []nbCell `json:"cells"`
	NbFormat    int      `json:"nbformat"`
	NbFormatMinor int    `json:"nbformat_minor"`
	Metadata    any      `json:"metadata,omitempty"`
}

func (t *NotebookEditTool) ExecuteContext(ctx context.Context, params map[string]any) ToolResult {
	select {
	case <-ctx.Done():
		return ToolResult{Output: fmt.Sprintf("Error: notebook_edit timed out: %v", ctx.Err()), IsError: true}
	default:
	}

	notebookPath, _ := params["notebook_path"].(string)
	if notebookPath == "" {
		return ToolResult{Output: "Error: notebook_path is required", IsError: true}
	}

	if !strings.HasSuffix(strings.ToLower(notebookPath), ".ipynb") {
		return ToolResult{Output: "Error: notebook_edit only works on .ipynb files", IsError: true}
	}

	fp := filepath.Clean(notebookPath)
	if !filepath.IsAbs(fp) {
		if wd, err := os.Getwd(); err == nil {
			fp = filepath.Join(wd, fp)
		}
	}

	info, err := os.Stat(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: notebook not found: %s (%v)", notebookPath, err), IsError: true}
	}

	if info.Size() > 10*1024*1024 {
		return ToolResult{Output: fmt.Sprintf("Error: notebook too large (%d bytes, max 10MB)", info.Size()), IsError: true}
	}

	// Read-before-edit check
	if t.registry != nil {
		if storedInfo, wasRead := t.registry.CheckFileRead(fp); wasRead && storedInfo.fromRead {
			if info.ModTime().After(storedInfo.mtime) {
				return ToolResult{
					Output: fmt.Sprintf("Error: notebook was modified since you last read it at %s. Read it again with read_file to get the current content.", notebookPath),
					IsError: true,
				}
			}
		} else {
			return ToolResult{
				Output: fmt.Sprintf("Error: you must read the notebook with read_file before editing it. Run: read_file(path='%s')", notebookPath),
				IsError: true,
			}
		}
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading notebook: %v", err), IsError: true}
	}

	var nb nbDocument
	if err := json.Unmarshal(data, &nb); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: invalid notebook format: %v", err), IsError: true}
	}

	if nb.NbFormat < 4 {
		return ToolResult{Output: fmt.Sprintf("Error: unsupported notebook format (nbformat=%d, requires 4+)", nb.NbFormat), IsError: true}
	}

	// Parse parameters
	cellID, _ := params["cell_id"].(string)
	newSource, _ := params["new_source"].(string)
	cellType, _ := params["cell_type"].(string)
	editMode, _ := params["edit_mode"].(string)
	if editMode == "" {
		editMode = "replace"
	}

	switch editMode {
	case "replace", "insert", "delete":
	default:
		return ToolResult{Output: fmt.Sprintf("Error: invalid edit_mode '%s'. Must be 'replace', 'insert', or 'delete'.", editMode), IsError: true}
	}

	if editMode == "insert" && cellType == "" {
		return ToolResult{Output: "Error: cell_type is required for insert mode. Must be 'code' or 'markdown'.", IsError: true}
	}

	if (editMode == "replace" || editMode == "insert") && newSource == "" {
		return ToolResult{Output: "Error: new_source is required for replace and insert modes.", IsError: true}
	}

	// Find the target cell
	targetIndex := findCellIndex(nb.Cells, cellID)

	// Execute the edit
	var result ToolResult
	switch editMode {
	case "replace":
		if targetIndex == -1 {
			// Auto-promote to insert if cell not found
			result = insertCellOp(&nb, len(nb.Cells), newSource, cellType)
		} else {
			result = replaceCellOp(&nb, targetIndex, newSource, cellType)
		}
	case "insert":
		if targetIndex == -1 {
			targetIndex = len(nb.Cells)
		}
		result = insertCellOp(&nb, targetIndex, newSource, cellType)
	case "delete":
		if targetIndex == -1 {
			return ToolResult{Output: fmt.Sprintf("Error: cell '%s' not found in notebook", cellID), IsError: true}
		}
		result = deleteCellOp(&nb, targetIndex)
	}

	if result.IsError {
		return result
	}

	// Ensure all cells have IDs
	ensureCellIDs(&nb)

	// Write the notebook
	if err := writeNotebookFile(fp, &nb); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error saving notebook: %v", err), IsError: true}
	}

	// Update registry cache
	if t.registry != nil {
		if data, err := os.ReadFile(fp); err == nil {
			t.registry.MarkFileReadWithContent(fp, string(data))
		}
	}

	return result
}

func (t *NotebookEditTool) Execute(params map[string]any) ToolResult {
	return t.ExecuteContext(context.Background(), params)
}

func findCellIndex(cells []nbCell, cellID string) int {
	// Match by actual cell ID
	for i, cell := range cells {
		if cell.ID == cellID {
			return i
		}
	}

	// Match by "cell-N" index format
	var idx int
	if _, err := fmt.Sscanf(cellID, "cell-%d", &idx); err == nil {
		if idx >= 0 && idx < len(cells) {
			return idx
		}
		return -1
	}

	// Match by source prefix/substring (best effort)
	for i, cell := range cells {
		src := sourceString(cell.Source)
		if strings.HasPrefix(src, cellID) || strings.Contains(src, cellID) {
			return i
		}
	}

	return -1
}

func sourceString(src any) string {
	switch v := src.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "")
	case []string:
		return strings.Join(v, "")
	default:
		return ""
	}
}

func replaceCellOp(nb *nbDocument, index int, newSource, cellType string) ToolResult {
	if index < 0 || index >= len(nb.Cells) {
		return ToolResult{Output: fmt.Sprintf("Error: cell index %d out of range (0-%d)", index, len(nb.Cells)-1), IsError: true}
	}

	cell := &nb.Cells[index]
	cell.Source = normalizeSource(newSource)

	if cellType != "" {
		cell.CellType = cellType
	}

	// Clear outputs for code cells
	if cell.CellType == "code" {
		cell.Outputs = nil
		cell.ExecutionCount = nil
	}

	srcPreview := trunc(newSource, 100)
	return ToolResult{
		Output: fmt.Sprintf("Replaced cell %d (%s):\n%s\n\nNotebook saved successfully.", index, nb.Cells[index].CellType, srcPreview),
	}
}

func insertCellOp(nb *nbDocument, index int, newSource, cellType string) ToolResult {
	if index < 0 || index > len(nb.Cells) {
		return ToolResult{Output: fmt.Sprintf("Error: insert index %d out of range (0-%d)", index, len(nb.Cells)), IsError: true}
	}

	newCell := nbCell{
		CellType: cellType,
		Source:   normalizeSource(newSource),
	}

	nb.Cells = append(nb.Cells, nbCell{})
	copy(nb.Cells[index+1:], nb.Cells[index:])
	nb.Cells[index] = newCell

	srcPreview := trunc(newSource, 100)
	return ToolResult{
		Output: fmt.Sprintf("Inserted new cell at position %d (%s):\n%s\n\nNotebook saved successfully.", index, cellType, srcPreview),
	}
}

func deleteCellOp(nb *nbDocument, index int) ToolResult {
	if index < 0 || index >= len(nb.Cells) {
		return ToolResult{Output: fmt.Sprintf("Error: cell index %d out of range (0-%d)", index, len(nb.Cells)-1), IsError: true}
	}

	deleted := nb.Cells[index]
	nb.Cells = append(nb.Cells[:index], nb.Cells[index+1:]...)

	return ToolResult{
		Output: fmt.Sprintf("Deleted cell %d (%s):\n%s\n\nNotebook saved successfully.", index, deleted.CellType, trunc(sourceString(deleted.Source), 100)),
	}
}

func ensureCellIDs(nb *nbDocument) {
	for i := range nb.Cells {
		if nb.Cells[i].ID == "" {
			nb.Cells[i].ID = fmt.Sprintf("cell-%d", i)
		}
	}
}

func normalizeSource(source string) any {
	if source == "" {
		return []string{}
	}
	lines := strings.SplitAfter(source, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func writeNotebookFile(path string, nb *nbDocument) error {
	// Create backup
	if data, err := os.ReadFile(path); err == nil {
		os.WriteFile(path+".bak", data, 0644)
	}

	data, err := json.MarshalIndent(nb, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize notebook: %w", err)
	}

	return WriteFileAtomically(path, data)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
