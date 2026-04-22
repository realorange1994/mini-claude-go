package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxFileSize = 2 * 1024 * 1024 // 2 MB
const readFileDefaultLimit = 2000   // default lines when limit not set
const readFileMaxChars = 15000      // max chars in output

// FileReadTool reads file contents with optional line range.
type FileReadTool struct{}

func (*FileReadTool) Name() string        { return "read_file" }
func (*FileReadTool) Description() string { return "Read the contents of a file. Returns numbered lines for easy reference." }

func (*FileReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "1-based start line (optional).",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Number of lines to read (optional).",
			},
		},
		"required": []string{"path"},
	}
}

func (*FileReadTool) CheckPermissions(params map[string]any) string { return "" }

func (*FileReadTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	fp := expandPath(pathStr)

	info, err := os.Stat(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", pathStr), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}
	}
	if info.IsDir() {
		return ToolResult{Output: fmt.Sprintf("Error: not a file: %s", pathStr), IsError: true}
	}
	if info.Size() > maxFileSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (>%d bytes)", maxFileSize), IsError: true}
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	// Remove trailing empty element from split
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	offset := 1
	if o, ok := params["offset"]; ok {
		switch v := o.(type) {
		case float64:
			offset = int(v)
		case int:
			offset = v
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				offset = n
			}
		}
	}
	if offset < 1 {
		offset = 1
	}

	limit := readFileDefaultLimit
	if lim, ok := params["limit"]; ok {
		switch v := lim.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
	}
	if limit <= 0 {
		limit = readFileDefaultLimit
	}

	total := len(lines)
	if offset > total {
		return ToolResult{
			Output: fmt.Sprintf("Error: offset %d is beyond end of file (%d lines)", offset, total),
			IsError: true,
		}
	}

	start := offset - 1
	end := start + limit
	if end > total {
		end = total
	}
	selected := lines[start:end]

	var numbered strings.Builder
	for i, line := range selected {
		lineNum := offset + i
		numbered.WriteString(fmt.Sprintf("%d| %s\n", lineNum, line))
	}

	result := numbered.String()

	// Truncate if too many chars
	if len(result) > readFileMaxChars {
		result = result[:readFileMaxChars] + "\n\n[OUTPUT TRUNCATED]"
	}

	// Add pagination hint
	if end < total {
		result += fmt.Sprintf("\n\n(Showing lines %d-%d of %d. Use offset=%d to continue.)", offset, end, total, end+1)
	} else {
		result += fmt.Sprintf("\n\n(End of file - %d lines total)", total)
	}

	return ToolResult{Output: strings.TrimRight(result, "\n")}
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	return filepath.Clean(p)
}
