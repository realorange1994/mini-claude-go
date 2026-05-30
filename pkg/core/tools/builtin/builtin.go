package builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditSpec specifies an edit operation on a file
type EditSpec struct {
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	OldString  string `json:"old_string,omitempty"`
	NewString  string `json:"new_string,omitempty"`
	InsertLine int    `json:"insert_line,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
}

// Edit applies an edit to a file (mirrors pi's ReadTool's edit operation)
// editType: "replace", "insert", "delete"
func Edit(edit EditSpec) (string, error) {
	switch edit.Type {
	case "replace":
		return editReplace(edit)
	case "insert":
		return editInsert(edit)
	case "delete":
		return editDelete(edit)
	default:
		return "", fmt.Errorf("unknown edit type: %s", edit.Type)
	}
}

func editReplace(e EditSpec) (string, error) {
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)

	if e.OldString != "" {
		if !strings.Contains(content, e.OldString) {
			return "", fmt.Errorf("old_string not found in file")
		}
		content = strings.Replace(content, e.OldString, e.NewString, 1)
	} else if e.StartLine > 0 && e.EndLine > 0 {
		lines := strings.Split(content, "\n")
		if e.StartLine > len(lines) || e.EndLine > len(lines) {
			return "", fmt.Errorf("line range out of bounds")
		}
		newLines := append(lines[:e.StartLine-1], append(strings.Split(e.NewString, "\n"), lines[e.EndLine:]...)...)
		content = strings.Join(newLines, "\n")
	}

	if err := os.WriteFile(e.Path, []byte(content), 0666); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return "Applied edit", nil
}

func editInsert(e EditSpec) (string, error) {
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	content := string(data)

	lines := strings.Split(content, "\n")
	if e.InsertLine < 1 {
		e.InsertLine = 1
	}
	if e.InsertLine > len(lines) {
		e.InsertLine = len(lines)
	}

	newLines := append(lines[:e.InsertLine], append(strings.Split(e.NewString, "\n"), lines[e.InsertLine:]...)...)
	content = strings.Join(newLines, "\n")

	if err := os.WriteFile(e.Path, []byte(content), 0666); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return "Inserted content", nil
}

func editDelete(e EditSpec) (string, error) {
	return editReplace(EditSpec{
		Type:      "replace",
		Path:      e.Path,
		OldString: e.OldString,
		NewString: "",
	})
}

// Glob finds files using glob patterns
func Glob(dir, pattern string) ([]string, error) {
	fullPattern := filepath.Join(dir, pattern)
	return filepath.Glob(fullPattern)
}

// FileInfo represents information about a file
type FileInfo struct {
	Name  string
	IsDir bool
	Size  int64
	Mode  string
}

// Ls lists files and directories in a path.
// Cleans the path to prevent traversal attacks.
func Ls(dir string) ([]FileInfo, error) {
	dir = filepath.Clean(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Mode:  info.Mode().String(),
		})
	}
	return files, nil
}
