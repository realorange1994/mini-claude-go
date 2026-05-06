package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maxFileSize = 256 * 1024 // 256 KB, matching Claude Code official

// FileReadTool reads file contents with optional line range.
type FileReadTool struct {
	registry *Registry // may be nil if tracker is not available
}

func NewFileReadTool(registry *Registry) *FileReadTool {
	return &FileReadTool{registry: registry}
}

func (*FileReadTool) Name() string        { return "read_file" }
func (*FileReadTool) Description() string {
	return "Reads a file from the local filesystem. You can access any file directly by using this tool.\n\n" +
		"Usage:\n" +
		"- The file_path parameter must be an absolute path, not a relative path\n" +
		"- By default, it reads up to 2000 lines starting from the beginning of the file\n" +
		"- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters\n" +
		"- Results are returned using cat -n format, with line numbers starting at 1\n" +
		"- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs\n" +
		"- You must read a file before editing it with edit_file or write_file.\n" +
		"NEVER use exec with cat, head, or tail — always use this tool instead."
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

func (t *FileReadTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileReadTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		return ToolResult{Output: "Error: file_path is required", IsError: true}
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

	// Parse offset/limit early so we can skip the size check for partial reads.
	// If the user specified offset and/or limit, they are reading a portion — allow it
	// even for large files (matching upstream behavior).
	hasExplicitOffset := false
	hasExplicitLimit := false
	offset := 1
	if o, ok := params["offset"]; ok {
		hasExplicitOffset = true
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

	// limit: number of lines. -1 sentinel means "read entire file" (will be resolved after reading).
	limit := -1
	if lim, ok := params["limit"]; ok {
		hasExplicitLimit = true
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

	isPartialRequest := hasExplicitOffset && hasExplicitLimit

	// Only enforce file size limit for full-file reads.
	// Partial reads (with offset/limit) are allowed for large files.
	if !isPartialRequest && info.Size() > maxFileSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (>256 KB). Use offset and limit parameters to read specific portions."), IsError: true}
	}
	// Dedup: if we've already read this exact range and the file hasn't
	// changed on disk, return a stub instead of re-sending the full content.
	// Only dedup entries from a prior read (not edit/write entries).
	if t.registry != nil && limit >= 0 {
		if storedInfo, wasRead := t.registry.CheckFileRead(fp); wasRead && storedInfo.fromRead {
			if storedInfo.readOffset == offset && storedInfo.readLimit == limit {
				if currentMtime := info.ModTime(); currentMtime == storedInfo.mtime {
					return ToolResult{Output: "File unchanged since last read. The content from the earlier read_file tool_result in this conversation is still current — refer to that instead of re-reading."}
				}
			}
		}
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// Detect encoding from BOM (matching upstream: UTF-16 LE support)
	var content string
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		// UTF-16 LE BOM — decode to UTF-8 string
		u16s := bytesToUint16LE(data[2:])
		content = string(utf16.Decode(u16s))
	} else {
		content = string(data)
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	// Strip UTF-8 BOM (matching official Claude Code behavior)
	if strings.HasPrefix(content, "\xEF\xBB\xBF") {
		content = content[3:]
	}
	lines := strings.Split(content, "\n")
	// Remove trailing empty element from split
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	total := len(lines)

	// Resolve limit sentinel (-1 means entire file).
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

	// Mark file as read in registry so write/edit checks pass
	// Store full content for content-based staleness fallback (matching upstream).
	// Only store content for full-file reads (when end >= total).
	if t.registry != nil {
		readContent := ""
		isPartial := false
		if end >= total {
			readContent = content
		} else {
			isPartial = true
		}
		t.registry.MarkFileReadWithParams(fp, offset, limit, readContent, isPartial, true) // fromRead=true
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
