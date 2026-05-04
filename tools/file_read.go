package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxFileSize = 256 * 1024 // 256 KB, matching Claude Code official

// FileReadTool reads file contents with optional line range.
type FileReadTool struct{}

func (*FileReadTool) Name() string        { return "read_file" }
func (*FileReadTool) Description() string {
	return "ALWAYS use this tool to read files. NEVER use exec with cat, head, or tail. " +
		"You MUST read a file before editing it with edit_file. " +
		"Returns numbered lines for easy reference."
}

func (*FileReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to read.",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "The line number to start reading from. Only provide if the file is too large to read at once.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "The number of lines to read. Only provide if the file is too large to read at once.",
			},
		},
		"required": []string{"file_path"},
	}
}

func (*FileReadTool) CheckPermissions(params map[string]any) string { return "" }

func (*FileReadTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		pathStr, _ = params["path"].(string)
	}
	fp := expandPath(pathStr)

	// SECURITY: Block UNC paths before any filesystem I/O to prevent NTLM credential leaks.
	// UNC paths like \\server\share\ would trigger SMB authentication, potentially leaking
	// credentials to an untrusted network server.
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

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
	// Block device files that would block indefinitely or produce infinite output
	// (matching official Claude Code behavior)
	if isDeviceFile(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: cannot read device file: %s", pathStr), IsError: true}
	}
	// Reject binary file extensions (matching official Claude Code behavior)
	// PDF, images, and SVG are handled separately in the official, but rejected here
	// with a clear message instead of garbage content or size-limit errors
	ext := strings.ToLower(filepath.Ext(fp))
	if isBinaryExtension(ext) {
		return ToolResult{Output: fmt.Sprintf("Error: binary file not supported: %s", ext), IsError: true}
	}
	if info.Size() > maxFileSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (>256 KB). Use offset and limit parameters to read specific portions."), IsError: true}
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	// Strip UTF-8 BOM (matching official Claude Code behavior)
	if strings.HasPrefix(content, "\xEF\xBB\xBF") {
		content = content[3:]
	}
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

	total := len(lines)

	limit := total // default: read entire file (matching Claude Code official)
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
		limit = total
	}

	if total == 0 {
		return ToolResult{
			Output: fmt.Sprintf("<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>"),
		}
	}
	if offset > total {
		return ToolResult{
			Output: fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", offset, total),
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
		numbered.WriteString(fmt.Sprintf("%d\t%s\n", lineNum, line))
	}

	result := numbered.String()

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
	// On Windows, bare drive letter like "E:" means current dir on that drive.
	// Normalize to "E:\" to reference the drive root.
	if len(p) == 2 && p[1] == ':' && (p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') {
		p = p + string(filepath.Separator)
	}
	return filepath.Clean(p)
}

// isBinaryExtension checks if a file extension is a binary format that should be rejected.
// Official Claude Code proactively rejects binary extensions to avoid reading garbage content.
func isBinaryExtension(ext string) bool {
	binaryExts := map[string]bool{
		// Executables
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".com": true,
		// Archives
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
		".7z": true, ".rar": true, ".tgz": true, ".zst": true, ".lz4": true,
		".cab": true, ".iso": true, ".img": true, ".dmg": true,
		// Images (without image processing support)
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
		".tiff": true, ".ico": true, ".webp": true, ".svgz": true,
		".avif": true, ".apng": true,
		// Audio/Video
		".mp3": true, ".mp4": true, ".wav": true, ".ogg": true, ".avi": true,
		".mov": true, ".mkv": true, ".flac": true, ".flv": true, ".wmv": true,
		".webm": true, ".aac": true, ".wma": true, ".m4a": true,
		// Data/compiled
		".pyc": true, ".pyo": true, ".o": true, ".obj": true, ".a": true,
		".lib": true, ".class": true, ".jar": true, ".war": true,
		".dat": true, ".bin": true, ".db": true, ".sqlite": true,
		".pdf": true, ".docx": true, ".xlsx": true, ".pptx": true,
		".woff": true, ".woff2": true, ".eot": true, ".ttf": true,
	}
	return binaryExts[ext]
}

// isDeviceFile checks if a path is a special device file that should be blocked from reading.
// These files would block indefinitely (/dev/zero, /dev/stdin) or produce infinite output.
// Matches official Claude Code behavior.
func isDeviceFile(path string) bool {
	// Normalize to forward slashes and lowercase for comparison
	normalized := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))

	// Check for Unix device files
	devicePaths := []string{
		"/dev/zero", "/dev/random", "/dev/urandom", "/dev/full",
		"/dev/stdin", "/dev/tty", "/dev/console",
		"/dev/stdout", "/dev/stderr",
		"/dev/fd/0", "/dev/fd/1", "/dev/fd/2",
	}
	for _, dp := range devicePaths {
		if normalized == dp || strings.HasSuffix(normalized, dp) {
			return true
		}
	}

	// Check for /proc/self/fd/ and /proc/<pid>/fd/ patterns
	if strings.Contains(normalized, "/proc/") && strings.Contains(normalized, "/fd/") {
		return true
	}

	return false
}

// isUncPath checks if a path is a UNC network path (\\server\share or //server/share).
// Accessing UNC paths triggers SMB authentication, potentially leaking NTLM credentials
// to an untrusted network server. Matches official Claude Code behavior.
func isUncPath(path string) bool {
	// Normalize backslashes to forward slashes for consistent prefix checking
	normalized := strings.ReplaceAll(path, "\\", "/")
	return strings.HasPrefix(normalized, "//") || strings.HasPrefix(normalized, "\\\\")
}
