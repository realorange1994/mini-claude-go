// Package writetool provides the Write tool implementation.
// Aligned to pi's tools/write.ts.
package writetool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"miniclaudecode-go/pkg/core/tools/editdiff"
)

// WriteInput is the input for the Write tool.
// Aligned to pi's WriteToolInput.
type WriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// WriteResult holds the result of a Write operation.
type WriteResult struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytes_written"`
	Diff         string `json:"diff,omitempty"`
	Patch        string `json:"patch,omitempty"`
	Created      bool   `json:"created"` // true if file was created (didn't exist)
}

// WriteOperations allows pluggable I/O for remote execution.
type WriteOperations interface {
	ReadFile(absolutePath string) ([]byte, error)
	WriteFile(absolutePath string, data []byte, perm os.FileMode) error
	Stat(absolutePath string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
}

// LocalWriteOperations implements WriteOperations for the local filesystem.
type LocalWriteOperations struct{}

func (LocalWriteOperations) ReadFile(absolutePath string) ([]byte, error) {
	return os.ReadFile(absolutePath)
}

func (LocalWriteOperations) WriteFile(absolutePath string, data []byte, perm os.FileMode) error {
	return os.WriteFile(absolutePath, data, perm)
}

func (LocalWriteOperations) Stat(absolutePath string) (os.FileInfo, error) {
	return os.Stat(absolutePath)
}

func (LocalWriteOperations) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Execute performs the Write operation.
// Aligned to pi's Write tool execute() function.
func Execute(input WriteInput, cwd string, ops WriteOperations) (*WriteResult, error) {
	if input.Path == "" {
		return nil, fmt.Errorf("missing required parameter: path")
	}

	// Resolve path
	absolutePath := resolveWritePath(input.Path, cwd)

	// Check if file already exists
	_, err := ops.Stat(absolutePath)
	created := os.IsNotExist(err)

	// Create parent directories if needed
	dir := filepath.Dir(absolutePath)
	if err := ops.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Read existing content for diff generation
	var oldContent string
	if !created {
		data, err := ops.ReadFile(absolutePath)
		if err != nil {
			oldContent = ""
		} else {
			oldContent = string(data)
		}
	}

	// Normalize line endings for comparison
	newContent := input.Content
	oldContentNorm := strings.ReplaceAll(oldContent, "\r\n", "\n")
	newContentNorm := strings.ReplaceAll(newContent, "\r\n", "\n")

	// No-change check
	if oldContentNorm == newContentNorm && !created {
		return &WriteResult{
			Path:         absolutePath,
			BytesWritten: len(newContent),
			Created:      false,
		}, nil
	}

	// Write the file (use 0666 so umask determines final permissions)
	if err := ops.WriteFile(absolutePath, []byte(newContent), 0666); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Generate diff/patch
	diff := editdiff.GenerateDiffString(oldContentNorm, newContentNorm)
	patch := editdiff.GenerateUnifiedPatch(filepath.Base(absolutePath), oldContentNorm, newContentNorm)

	return &WriteResult{
		Path:         absolutePath,
		BytesWritten: len(newContent),
		Diff:         diff,
		Patch:        patch,
		Created:      created,
	}, nil
}

// resolveWritePath resolves a path for writing.
func resolveWritePath(path, cwd string) string {
	// Expand ~
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	// If already absolute, return as-is
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	// Resolve relative to cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Clean(filepath.Join(cwd, path))
}
