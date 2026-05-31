// Package edittool provides the Edit tool implementation with replace, insert, and delete modes.
// Aligned to pi's tools/edit.ts.
package edittool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"miniclaudecode-go/pkg/core/tools/editdiff"
	"miniclaudecode-go/pkg/core/tools/pathutils"
)

// EditType defines the edit operation type.
type EditType string

const (
	EditReplace EditType = "replace"
	EditInsert  EditType = "insert"
	EditDelete  EditType = "delete"
)

// EditEntry represents a single targeted replacement in the edits[] array.
// Aligned to pi's replaceEditSchema: {oldText, newText}.
type EditEntry struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// EditInput is the input for the Edit tool.
// Supports both the new edits[] array format (aligned to TS) and the legacy
// single-edit format (type/old_string/new_string/insert_line/start_line/end_line).
type EditInput struct {
	// New-style: edits[] array for multiple targeted replacements in one call.
	Edits []EditEntry `json:"edits,omitempty"`

	// Legacy: single-edit fields
	Type       EditType `json:"type,omitempty"`
	Path       string   `json:"path"`
	OldString  string   `json:"old_string,omitempty"`
	NewString  string   `json:"new_string,omitempty"`
	InsertLine int      `json:"insert_line,omitempty"`
	StartLine  int      `json:"start_line,omitempty"`
	EndLine    int      `json:"end_line,omitempty"`
}

// EditDetails holds metadata about an edit operation.
type EditDetails struct {
	FirstChangedLine int  `json:"first_changed_line,omitempty"`
	EditsApplied     int  `json:"edits_applied,omitempty"`
	IsNewFile        bool `json:"is_new_file,omitempty"`
}

// EditResult holds the result of an edit operation.
type EditResult struct {
	Output  string      `json:"output"`
	Details EditDetails `json:"details"`
	Diff    string      `json:"diff,omitempty"`
	Patch   string      `json:"patch,omitempty"`
}

// EditOperations allows pluggable operations for remote/SSH.
type EditOperations interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
}

// LocalEditOperations implements EditOperations for local filesystem.
type LocalEditOperations struct{}

func (LocalEditOperations) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (LocalEditOperations) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// Execute performs an Edit operation.
func Execute(input EditInput, cwd string, ops EditOperations) (*EditResult, error) {
	if input.Path == "" {
		return nil, fmt.Errorf("missing required parameter: path")
	}

	// Resolve path relative to cwd
	absPath := input.Path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(cwd, absPath)
	}
	absPath = filepath.Clean(absPath)

	// Security: ensure path is within cwd
	if err := pathutils.ValidatePath(absPath, cwd); err != nil {
		return nil, err
	}

	// New-style: edits[] array (aligned to TS edit tool)
	if len(input.Edits) > 0 {
		return executeEdits(input, absPath)
	}

	// Legacy: single-edit mode
	if input.Type == "" {
		input.Type = EditReplace
	}

	switch input.Type {
	case EditReplace:
		return executeReplace(input, absPath, ops)
	case EditInsert:
		return executeInsert(input, absPath, ops)
	case EditDelete:
		return executeDelete(input, absPath, ops)
	default:
		return nil, fmt.Errorf("unknown edit type: %s", input.Type)
	}
}

// executeEdits applies the edits[] array using editdiff.ApplyEditToFile.
// This is the primary path aligned to the TS edit tool.
func executeEdits(input EditInput, absPath string) (*EditResult, error) {
	edits := make([]editdiff.Edit, len(input.Edits))
	for i, e := range input.Edits {
		edits[i] = editdiff.Edit{OldText: e.OldText, NewText: e.NewText}
	}
	result, err := editdiff.ApplyEditToFile(absPath, edits)
	if err != nil {
		return nil, err
	}
	return &EditResult{
		Output: fmt.Sprintf("Successfully replaced %d block(s) in %s", len(input.Edits), absPath),
		Details: EditDetails{
			FirstChangedLine: result.FirstChangedLine,
			EditsApplied:     result.EditsApplied,
		},
		Diff:  result.Diff,
		Patch: result.Patch,
	}, nil
}

func executeReplace(input EditInput, absPath string, ops EditOperations) (*EditResult, error) {
	if input.OldString != "" {
		// Use editdiff for fuzzy matching, BOM handling, and diff generation
		result, err := editdiff.ApplyEditToFile(absPath, []editdiff.Edit{
			{OldText: input.OldString, NewText: input.NewString},
		})
		if err != nil {
			return nil, err
		}
		return &EditResult{
			Output: fmt.Sprintf("Applied edit to %s", absPath),
			Details: EditDetails{
				FirstChangedLine: result.FirstChangedLine,
				EditsApplied:     result.EditsApplied,
			},
			Diff:  result.Diff,
			Patch: result.Patch,
		}, nil
	}

	if input.StartLine > 0 && input.EndLine > 0 {
		data, err := ops.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		content := string(data)
		lines := strings.Split(content, "\n")
		if input.StartLine > len(lines) || input.EndLine > len(lines) {
			return nil, fmt.Errorf("line range out of bounds (file has %d lines)", len(lines))
		}
		newLines := append(lines[:input.StartLine-1], append(strings.Split(input.NewString, "\n"), lines[input.EndLine:]...)...)
		newContent := strings.Join(newLines, "\n")
		if err := ops.WriteFile(absPath, []byte(newContent), 0666); err != nil {
			return nil, fmt.Errorf("failed to write file: %w", err)
		}
		return &EditResult{
			Output: fmt.Sprintf("Applied range edit to %s (lines %d-%d)", absPath, input.StartLine, input.EndLine),
			Details: EditDetails{
				FirstChangedLine: input.StartLine,
				EditsApplied:     1,
			},
		}, nil
	}

	return nil, fmt.Errorf("replace requires either old_string or start_line+end_line")
}

func executeInsert(input EditInput, absPath string, ops EditOperations) (*EditResult, error) {
	data, err := ops.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	insertAt := input.InsertLine
	if insertAt < 1 {
		insertAt = 1
	}
	if insertAt > len(lines) {
		insertAt = len(lines)
	}

	newLines := append(lines[:insertAt], append(strings.Split(input.NewString, "\n"), lines[insertAt:]...)...)
	newContent := strings.Join(newLines, "\n")

	if err := ops.WriteFile(absPath, []byte(newContent), 0666); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return &EditResult{
		Output: fmt.Sprintf("Inserted content at line %d in %s", insertAt, absPath),
		Details: EditDetails{
			FirstChangedLine: insertAt,
			EditsApplied:     1,
		},
	}, nil
}

func executeDelete(input EditInput, absPath string, ops EditOperations) (*EditResult, error) {
	if input.OldString == "" {
		return nil, fmt.Errorf("delete requires old_string")
	}
	// Delete is replace with empty new_string
	return executeReplace(EditInput{
		Type:      EditReplace,
		Path:      input.Path,
		OldString: input.OldString,
		NewString: "",
	}, absPath, ops)
}

// FormatEditOutput formats an edit result for display.
func FormatEditOutput(result *EditResult) string {
	output := result.Output
	if result.Diff != "" {
		output += "\n" + result.Diff
	}
	return output
}