package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ─── Apply Patch Tool (MiMo-Code 5) ────────────────────────────────────────
//
// Structured patch application tool that accepts unified patch text.
// Supports add, update, delete, and move operations across multiple files.
//
// MiMo-Code source: tool/apply_patch.ts (308 lines)

// PatchOperation represents the type of patch operation.
type PatchOperation string

const (
	PatchAdd    PatchOperation = "add"
	PatchUpdate PatchOperation = "update"
	PatchDelete PatchOperation = "delete"
	PatchMove   PatchOperation = "move"
)

// PatchFile represents a single file patch.
type PatchFile struct {
	Operation PatchOperation
	Path      string
	NewPath   string // for move operations
	Content   string // new content for add/update
	Diff      string // diff for update operations
}

// PatchResult holds the result of applying a patch.
type PatchResult struct {
	FilesChanged int
	Additions    int
	Deletions    int
	Changes      []PatchChange
	Errors       []PatchError
}

// PatchChange represents a file change from a patch.
type PatchChange struct {
	Path       string
	Operation  PatchOperation
	Additions  int
	Deletions  int
}

// PatchError represents an error applying a patch.
type PatchError struct {
	Path    string
	Message string
}

// ApplyPatch applies a structured patch to the filesystem.
func ApplyPatch(patchText string, projectDir string) (*PatchResult, error) {
	files, err := ParsePatch(patchText)
	if err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}

	result := &PatchResult{}

	for _, file := range files {
		fullPath := filepath.Join(projectDir, file.Path)

		switch file.Operation {
		case PatchAdd:
			if err := applyAdd(fullPath, file.Content); err != nil {
				result.Errors = append(result.Errors, PatchError{Path: file.Path, Message: err.Error()})
			} else {
				additions := countLines(file.Content)
				result.Changes = append(result.Changes, PatchChange{
					Path:      file.Path,
					Operation: PatchAdd,
					Additions: additions,
				})
				result.Additions += additions
				result.FilesChanged++
			}

		case PatchUpdate:
			additions, deletions, err := applyUpdate(fullPath, file.Content)
			if err != nil {
				result.Errors = append(result.Errors, PatchError{Path: file.Path, Message: err.Error()})
			} else {
				result.Changes = append(result.Changes, PatchChange{
					Path:      file.Path,
					Operation: PatchUpdate,
					Additions: additions,
					Deletions: deletions,
				})
				result.Additions += additions
				result.Deletions += deletions
				result.FilesChanged++
			}

		case PatchDelete:
			if err := applyDelete(fullPath); err != nil {
				result.Errors = append(result.Errors, PatchError{Path: file.Path, Message: err.Error()})
			} else {
				result.Changes = append(result.Changes, PatchChange{
					Path:      file.Path,
					Operation: PatchDelete,
				})
				result.FilesChanged++
			}

		case PatchMove:
			newFullPath := filepath.Join(projectDir, file.NewPath)
			if err := applyMove(fullPath, newFullPath); err != nil {
				result.Errors = append(result.Errors, PatchError{Path: file.Path, Message: err.Error()})
			} else {
				result.Changes = append(result.Changes, PatchChange{
					Path:      file.Path,
					Operation: PatchMove,
				})
				result.FilesChanged++
			}
		}
	}

	return result, nil
}

// ParsePatch parses a patch text into file operations.
func ParsePatch(patchText string) ([]PatchFile, error) {
	var files []PatchFile
	lines := strings.Split(patchText, "\n")

	var currentFile *PatchFile
	var contentLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for operation markers
		if strings.HasPrefix(trimmed, "*** Begin Patch") {
			continue
		}
		if strings.HasPrefix(trimmed, "*** End Patch") {
			if currentFile != nil {
				currentFile.Content = strings.Join(contentLines, "\n")
				files = append(files, *currentFile)
				currentFile = nil
				contentLines = nil
			}
			continue
		}

		// Check for file operation lines
		if strings.HasPrefix(trimmed, "*** Add File:") {
			if currentFile != nil {
				currentFile.Content = strings.Join(contentLines, "\n")
				files = append(files, *currentFile)
			}
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Add File:"))
			currentFile = &PatchFile{Operation: PatchAdd, Path: path}
			contentLines = nil
			continue
		}
		if strings.HasPrefix(trimmed, "*** Update File:") {
			if currentFile != nil {
				currentFile.Content = strings.Join(contentLines, "\n")
				files = append(files, *currentFile)
			}
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Update File:"))
			currentFile = &PatchFile{Operation: PatchUpdate, Path: path}
			contentLines = nil
			continue
		}
		if strings.HasPrefix(trimmed, "*** Delete File:") {
			if currentFile != nil {
				currentFile.Content = strings.Join(contentLines, "\n")
				files = append(files, *currentFile)
			}
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Delete File:"))
			currentFile = &PatchFile{Operation: PatchDelete, Path: path}
			contentLines = nil
			continue
		}
		if strings.HasPrefix(trimmed, "*** Move File:") {
			if currentFile != nil {
				currentFile.Content = strings.Join(contentLines, "\n")
				files = append(files, *currentFile)
			}
			parts := strings.TrimSpace(strings.TrimPrefix(trimmed, "*** Move File:"))
			splitParts := strings.SplitN(parts, " to ", 2)
			if len(splitParts) == 2 {
				currentFile = &PatchFile{
					Operation: PatchMove,
					Path:      strings.TrimSpace(splitParts[0]),
					NewPath:   strings.TrimSpace(splitParts[1]),
				}
			}
			contentLines = nil
			continue
		}

		// Content lines
		if currentFile != nil {
			contentLines = append(contentLines, line)
		}
	}

	// Handle last file
	if currentFile != nil {
		currentFile.Content = strings.Join(contentLines, "\n")
		files = append(files, *currentFile)
	}

	return files, nil
}

// applyAdd creates a new file with the given content.
func applyAdd(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// applyUpdate replaces file content and returns addition/deletion counts.
func applyUpdate(path string, newContent string) (int, int, error) {
	oldContent, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("read file: %w", err)
	}

	oldLines := countLines(string(oldContent))
	newLines := countLines(newContent)

	additions := newLines - oldLines
	deletions := 0
	if additions < 0 {
		deletions = -additions
		additions = 0
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return 0, 0, fmt.Errorf("write file: %w", err)
	}

	return additions, deletions, nil
}

// applyDelete deletes a file.
func applyDelete(path string) error {
	return os.Remove(path)
}

// applyMove moves a file from one path to another.
func applyMove(oldPath, newPath string) error {
	dir := filepath.Dir(newPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return os.Rename(oldPath, newPath)
}

// countLines counts the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// FormatPatchResult formats a patch result for display.
func FormatPatchResult(result *PatchResult) string {
	if result == nil {
		return "No patch applied."
	}

	var sb string
	sb += fmt.Sprintf("## Patch Applied\n\n")
	sb += fmt.Sprintf("**%d files changed, +%d additions, -%d deletions**\n\n", result.FilesChanged, result.Additions, result.Deletions)

	for _, change := range result.Changes {
		sb += fmt.Sprintf("- `%s` (%s): +%d -%d\n", change.Path, change.Operation, change.Additions, change.Deletions)
	}

	if len(result.Errors) > 0 {
		sb += "\n### Errors\n\n"
		for _, err := range result.Errors {
			sb += fmt.Sprintf("- `%s`: %s\n", err.Path, err.Message)
		}
	}

	return sb
}
