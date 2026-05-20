package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileEncodingTool reads, writes, and edits files with arbitrary text encodings.
// Supports: GBK, GB18030, Latin-1, Windows-1252, Shift-JIS, Big5, EUC-KR, etc.
type FileEncodingTool struct {
	registry *Registry
}

func NewFileEncodingTool(registry *Registry) *FileEncodingTool {
	return &FileEncodingTool{registry: registry}
}

func (*FileEncodingTool) Name() string { return "file_encoding" }

func (*FileEncodingTool) Description() string {
	return "Read, write, or edit files with arbitrary text encodings (GBK, GB18030, Latin-1, " +
		"Shift-JIS, Big5, EUC-KR, etc.).\n\n" +
		"IMPORTANT: This is a fallback tool. ALWAYS prefer read_file + edit_file/multi_edit + " +
		"write_file for coding and file manipulation. Only use file_encoding when those tools " +
		"fail due to encoding issues (garbled text, non-UTF-8 detection, unsupported charset). " +
		"Do not use this tool for routine coding tasks unless the existing tools report encoding " +
		"errors.\n\n" +
		"Auto-detects encoding if not specified. Encoding detection uses byte-pattern analysis " +
		"from golang.org/x/net/html/charset.\n\n" +
		"Usage:\n" +
		"- detect: Only detect the encoding, returns encoding name and a preview.\n" +
			"- For read/edit/multi_edit: auto-detects encoding if not specified.\n" +
		"- For write: follows write_file convention — you MUST use file_encoding read first for existing files. New files default to UTF-8.\n" +
		"- edit/multi_edit: follows edit_file/multi_edit convention — read-before-write validation.\n\n" +
		"Common encoding names: gbk, gb18030, big5, shift_jis, euc_jp, euc_kr, " +
		"iso-8859-1 (latin-1), windows-1252, windows-1251."
}

func (*FileEncodingTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file to operate on.",
			},
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"read", "write", "edit", "multi_edit", "detect"},
				"description": "Operation to perform: read (decode and return content), write (encode and write content), edit (single search-and-replace), multi_edit (multiple search-and-replace edits applied atomically), detect (detect encoding only). Default: read.",
			},
			"encoding": map[string]any{
				"type":        "string",
				"description": "Encoding name. Default: utf-8. For read/edit/multi_edit, auto-detects if omitted. For write, defaults to utf-8. Examples: gbk, gb18030, big5, shift_jis, euc_jp, euc_kr, iso-8859-1, windows-1252.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Text content to write. REQUIRED for 'write' operation. For 'edit', use old_string/new_string instead.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to find and replace. REQUIRED for 'edit' operation. Must be non-empty.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text. REQUIRED for 'edit' operation. Falls back to content if not provided.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace all occurrences of old_string (for edit operation, default: false).",
			},
			"edits": map[string]any{
				"type":        "array",
				"description": "List of {old_string, new_string} edit operations (required for multi_edit operation).",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old_string": map[string]any{
							"type":        "string",
							"description": "Exact text to find.",
						},
						"new_string": map[string]any{
							"type":        "string",
							"description": "Text to replace it with.",
						},
						"replace_all": map[string]any{
							"type":        "boolean",
							"description": "Replace all occurrences of this old_string (default: false).",
						},
					},
					"required": []string{"old_string", "new_string"},
				},
			},
		},
		"required": []string{"path"},
	}
}

func (*FileEncodingTool) CheckPermissions(params map[string]any) PermissionResult {
	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		return PermissionResultPassthrough()
	}
	return CheckPathSafetyForAutoEdit(pathStr)
}

func (e *FileEncodingTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["path"].(string)
	if pathStr == "" {
		return ToolResult{Output: "Error: path is required", IsError: true}
	}

	op, _ := params["operation"].(string)
	if op == "" {
		op = "read"
	}

	switch op {
	case "detect":
		return e.detect(pathStr)
	case "read":
		return e.read(pathStr, params)
	case "write":
		return e.write(pathStr, params)
	case "edit":
		return e.edit(pathStr, params)
	case "multi_edit":
		return e.multiEdit(pathStr, params)
	default:
		return ToolResult{Output: fmt.Sprintf("Error: unknown operation %q. Supported: read, write, edit, multi_edit, detect", op), IsError: true}
	}
}

func (e *FileEncodingTool) detect(pathStr string) ToolResult {
	fp := expandPath(pathStr)
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

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	encName, certain := DetectCharset(data, "")
	if encName == "unknown" {
		encName = "unknown (not UTF-8, not detectable)"
		certain = false
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Detected encoding: %s (certain: %v)", encName, certain))
	lines = append(lines, fmt.Sprintf("File size: %d bytes", len(data)))

	// Preview: decode and show first 200 characters
	if IsSupportedEncoding(encName) {
		decoded, err := DecodeWithEncoding(data, encName)
		if err == nil {
			decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
			preview := decoded
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			lines = append(lines, "")
			lines = append(lines, "Preview (first 200 chars):")
			lines = append(lines, preview)
		}
	}

	return ToolResult{Output: strings.Join(lines, "\n")}
}

func (e *FileEncodingTool) read(pathStr string, params map[string]any) ToolResult {
	fp := expandPath(pathStr)
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

	data, err := os.ReadFile(fp)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	encName := getEncodingParam(params)
	if encName == "" {
		encName, _ = DetectCharset(data, "")
		if encName == "unknown" {
			return ToolResult{Output: "Error: Could not auto-detect encoding. This file does not appear to be UTF-8 and no BOM was found. Specify the encoding parameter explicitly.", IsError: true}
		}
	}

	decoded, err := DecodeWithEncoding(data, encName)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error decoding file with encoding %s: %v", encName, err), IsError: true}
	}

	// Normalize line endings
	decoded = strings.ReplaceAll(decoded, "\r\n", "\n")
	lines := strings.Split(decoded, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	total := len(lines)

	var numbered strings.Builder
	for i, line := range lines {
		numbered.WriteString(fmt.Sprintf("%d\t%s\n", i+1, line))
	}

	result := numbered.String()
	if total > 0 {
		result += fmt.Sprintf("\n\n(End of file - %d lines total, encoding: %s)", total, encName)
	}

	return ToolResult{Output: strings.TrimRight(result, "\n")}
}

func (e *FileEncodingTool) write(pathStr string, params map[string]any) ToolResult {
	fp := expandPath(pathStr)
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

	content, _ := params["content"].(string)
	if content == "" {
		return ToolResult{Output: "Error: content is required for write operation", IsError: true}
	}

	// Size limit (matching write_file convention)
	const maxWriteSize = 10 * 1024 * 1024 // 10MB
	if len(content) > maxWriteSize {
		return ToolResult{Output: fmt.Sprintf("Error: content too large (%d bytes, max %d bytes)", len(content), maxWriteSize), IsError: true}
	}

	// Read-before-write validation and concurrent modification detection
	if e.registry != nil {
		if staleMsg := e.registry.CheckFileStale(fp); staleMsg != "" {
			return ToolResult{Output: staleMsg, IsError: true}
		}
	}

	encName := getEncodingParam(params)
	if encName == "" {
		// If file already exists, preserve its encoding; otherwise default to utf-8
		existingData, err := os.ReadFile(fp)
		if err == nil && len(existingData) > 0 {
			detected, _ := DetectCharset(existingData, "")
			if detected != "unknown" && detected != "" {
				encName = detected
			}
		}
		if encName == "" {
			encName = "utf-8"
		}
	}

	// Check if encoding is supported
	if !IsSupportedEncoding(encName) {
		return ToolResult{Output: fmt.Sprintf("Error: unsupported encoding %q. Supported: gbk, gb18030, big5, shift_jis, euc_jp, euc_kr, iso-8859-1, windows-1252, and more.", encName), IsError: true}
	}

	encoded, err := EncodeWithEncoding(content, encName)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error encoding content with encoding %s: %v", encName, err), IsError: true}
	}

	// Create parent directories
	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error creating directory: %v", err), IsError: true}
	}

	if err := WriteFileAtomically(fp, encoded); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}

	if e.registry != nil {
		e.registry.MarkFileReadWithContent(fp, content)
	}

	out := fmt.Sprintf("Wrote %d chars to %s (encoding: %s)", len(content), fp, encName)
	// Large file warning (matching write_file convention)
	const warnThreshold = 1024 * 1024 // 1MB
	if len(content) > warnThreshold {
		sizeMB := float64(len(content)) / (1024 * 1024)
		out += fmt.Sprintf("\n[WARN] Large file written (%.1f MB). Confirm with the user before proceeding.", sizeMB)
	}
	return ToolResult{Output: out}
}

func (e *FileEncodingTool) edit(pathStr string, params map[string]any) ToolResult {
	fp := expandPath(pathStr)
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

	oldStr, _ := params["old_string"].(string)
	if oldStr == "" {
		return ToolResult{Output: "Error: old_string is required for edit operation", IsError: true}
	}
	newStr := getParam(params, "new_string", "content")
	replaceAll, _ := params["replace_all"].(bool)

	// Check for identical old/new strings
	if oldStr == newStr {
		return ToolResult{Output: "Error: old_string and new_string must be different", IsError: true}
	}

	// Read-before-write validation and concurrent modification detection
	if e.registry != nil {
		if staleMsg := e.registry.CheckFileStale(fp); staleMsg != "" {
			return ToolResult{Output: staleMsg, IsError: true}
		}
	}

	// Reject .ipynb files
	if strings.HasSuffix(strings.ToLower(fp), ".ipynb") {
		return ToolResult{Output: "Error: file is a Jupyter Notebook (.ipynb). Jupyter notebooks cannot be edited with the file_encoding tool — use the notebook tool instead.", IsError: true}
	}

	// 1 GiB file size guard
	const maxEditSize = 1 << 30 // 1 GiB
	if info, err := os.Stat(fp); err == nil && info.Size() > maxEditSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (%d bytes, max %d bytes). Use offset/limit to read portions.", info.Size(), maxEditSize), IsError: true}
	}

	// Read file
	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", pathStr), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// Detect encoding
	encName := getEncodingParam(params)
	if encName == "" {
		encName, _ = DetectCharset(data, "")
		if encName == "unknown" {
			return ToolResult{Output: "Error: Could not auto-detect encoding. Specify the encoding parameter explicitly.", IsError: true}
		}
	}

	// Decode
	content, err := DecodeWithEncoding(data, encName)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error decoding file with encoding %s: %v", encName, err), IsError: true}
	}

	// Detect CRLF and original encoding metadata for write-back
	fileMeta := FileMetadata{Encoding: encName}
	hasCRLF := strings.Contains(content, "\r\n")
	if hasCRLF {
		fileMeta.LineEndings = LineEndingCRLF
	}

	// Strip trailing whitespace from new_string (except .md/.mdx)
	ext := strings.ToLower(filepath.Ext(fp))
	if ext != ".md" && ext != ".mdx" {
		newStr = stripTrailingWhitespace(newStr)
	}

	// Normalize CRLF for matching
	if hasCRLF {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		oldStr = strings.ReplaceAll(oldStr, "\r\n", "\n")
		newStr = strings.ReplaceAll(newStr, "\r\n", "\n")
	}

	// Normalize curly quotes for matching (matching edit_file behavior)
	contentNorm := normalizeQuotes(content)
	oldStrNorm := normalizeQuotes(oldStr)
	newStrNorm := normalizeQuotes(newStr)

	// Apply replacement
	count := strings.Count(contentNorm, oldStrNorm)
	if count == 0 {
		// Try desanitized version (matching edit_file: reverse sanitized tokens)
		desanitizedOld := desanitize(oldStrNorm)
		desanitizedNew := desanitize(newStrNorm)
		if desanitizedOld != oldStrNorm {
			count = strings.Count(contentNorm, desanitizedOld)
			if count > 0 {
				oldStrNorm = desanitizedOld
				newStrNorm = desanitizedNew
			}
		}
	}
	if count == 0 {
		return ToolResult{Output: fmt.Sprintf("Error: old_text not found in %s. Verify the file content.", pathStr), IsError: true}
	}
	if count > 1 && !replaceAll {
		return ToolResult{
			Output: fmt.Sprintf("Warning: old_text appears %d times. Provide more context or set replace_all=true.", count),
			IsError: true,
		}
	}

	// Apply quote style preservation (matching edit_file)
	styledNewStr := preserveQuoteStyle(contentNorm, oldStr, newStr, oldStrNorm)

	// Apply replacement — handle deletion line trailing \n (matching edit_file)
	if styledNewStr == "" && !strings.HasSuffix(oldStrNorm, "\n") {
		oldWithLF := oldStrNorm + "\n"
		if replaceAll {
			contentNorm = strings.ReplaceAll(contentNorm, oldWithLF, styledNewStr)
		} else if idx := strings.Index(contentNorm, oldWithLF); idx >= 0 {
			contentNorm = contentNorm[:idx] + styledNewStr + contentNorm[idx+len(oldWithLF):]
		} else {
			contentNorm = applyReplacement(contentNorm, oldStrNorm, styledNewStr, replaceAll)
		}
	} else {
		contentNorm = applyReplacement(contentNorm, oldStrNorm, styledNewStr, replaceAll)
	}

	// Restore CRLF if original file had it
	if hasCRLF {
		contentNorm = restoreCRLF(contentNorm)
	}

	// Encode back with original encoding
	encoded, err := EncodeWithEncoding(contentNorm, encName)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error encoding content with encoding %s: %v", encName, err), IsError: true}
	}

	if err := WriteFileAtomically(fp, encoded); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}

	if e.registry != nil {
		e.registry.MarkFileReadWithContent(fp, contentNorm)
	}

	return ToolResult{Output: fmt.Sprintf("Successfully edited %s (encoding: %s, applied 1 edit)", fp, encName)}
}

func (e *FileEncodingTool) multiEdit(pathStr string, params map[string]any) ToolResult {
	fp := expandPath(pathStr)
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

	// Read-before-write validation and concurrent modification detection
	if e.registry != nil {
		if staleMsg := e.registry.CheckFileStale(fp); staleMsg != "" {
			return ToolResult{Output: staleMsg, IsError: true}
		}
	}

	// Reject .ipynb files
	if strings.HasSuffix(strings.ToLower(fp), ".ipynb") {
		return ToolResult{Output: "Error: file is a Jupyter Notebook (.ipynb). Jupyter notebooks cannot be edited with the file_encoding tool — use the notebook tool instead.", IsError: true}
	}

	// 1 GiB file size guard
	const maxEditSize = 1 << 30 // 1 GiB
	if info, err := os.Stat(fp); err == nil && info.Size() > maxEditSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (%d bytes, max %d bytes). Use offset/limit to read portions.", info.Size(), maxEditSize), IsError: true}
	}

	// Parse edits
	editsRaw, ok := params["edits"]
	if !ok {
		return ToolResult{Output: "Error: edits is required for multi_edit operation", IsError: true}
	}
	editsSlice, ok := editsRaw.([]any)
	if !ok {
		return ToolResult{Output: "Error: edits must be an array", IsError: true}
	}
	if len(editsSlice) == 0 {
		return ToolResult{Output: "Error: edits must not be empty", IsError: true}
	}

	type editOp struct {
		old        string
		new        string
		replaceAll bool
	}
	var edits []editOp
	for i, ev := range editsSlice {
		m, ok := ev.(map[string]any)
		if !ok {
			return ToolResult{Output: fmt.Sprintf("Error: edit %d must be an object", i+1), IsError: true}
		}
		oldStr, _ := m["old_string"].(string)
		newStr, _ := m["new_string"].(string)
		replaceAll, _ := m["replace_all"].(bool)
		if oldStr == "" {
			return ToolResult{Output: fmt.Sprintf("Error: edit %d: old_string must not be empty", i+1), IsError: true}
		}
		edits = append(edits, editOp{old: oldStr, new: newStr, replaceAll: replaceAll})
	}

	// Read file
	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", pathStr), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// Detect encoding
	encName := getEncodingParam(params)
	if encName == "" {
		encName, _ = DetectCharset(data, "")
		if encName == "unknown" {
			return ToolResult{Output: "Error: Could not auto-detect encoding. Specify the encoding parameter explicitly.", IsError: true}
		}
	}

	// Decode
	content, err := DecodeWithEncoding(data, encName)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error decoding file with encoding %s: %v", encName, err), IsError: true}
	}

	// Normalize CRLF for matching
	hasCRLF := strings.Contains(content, "\r\n")
	if hasCRLF {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		for i := range edits {
			edits[i].old = strings.ReplaceAll(edits[i].old, "\r\n", "\n")
			edits[i].new = strings.ReplaceAll(edits[i].new, "\r\n", "\n")
		}
	}

	// Strip trailing whitespace from new_string (except .md/.mdx)
	ext := strings.ToLower(filepath.Ext(fp))
	if ext != ".md" && ext != ".mdx" {
		for i := range edits {
			edits[i].new = stripTrailingWhitespace(edits[i].new)
		}
	}

	// Normalize curly quotes (matching multi_edit behavior)
	content = normalizeQuotes(content)
	for i := range edits {
		edits[i].old = normalizeQuotes(edits[i].old)
		edits[i].new = normalizeQuotes(edits[i].new)
	}

	// Track applied new strings for overlapping edit detection
	var appliedNewStrings []string

	// Dry run + apply all edits sequentially
	for i, ev := range edits {
		oldTrimmed := strings.TrimRight(ev.old, "\n")

		// Overlapping edit detection
		for _, prevNew := range appliedNewStrings {
			if oldTrimmed != "" && strings.Contains(prevNew, oldTrimmed) {
				return ToolResult{
					Output: fmt.Sprintf("Error: edit %d failed: old_string is a substring of a new_string from a previous edit", i+1),
					IsError: true,
				}
			}
		}

		// Find location (exact match, then with trailing newlines stripped)
		idx := findEditLocation(content, ev.old)
		if idx < 0 {
			// Try desanitized version
			desanitizedOld := desanitize(ev.old)
			desanitizedNew := desanitize(ev.new)
			idx = findEditLocation(content, desanitizedOld)
			if idx >= 0 {
				edits[i].old = desanitizedOld
				edits[i].new = desanitizedNew
			}
		}
		if idx < 0 {
			return ToolResult{
				Output: fmt.Sprintf("Error: edit %d failed: old_text not found: %q", i+1, truncate(ev.old, 80)),
				IsError: true,
			}
		}

		// Reject ambiguous edits when replace_all is false
		if !ev.replaceAll {
			cnt := countOccurrences(content, ev.old)
			if cnt > 1 {
				return ToolResult{
					Output: fmt.Sprintf("Error: edit %d failed: old_string has multiple matches; set replace_all to true or provide more context", i+1),
					IsError: true,
				}
			}
		}

		// Apply edit
		if ev.replaceAll {
			content = strings.ReplaceAll(content, ev.old, ev.new)
		} else {
			content = strings.Replace(content, ev.old, ev.new, 1)
		}
		appliedNewStrings = append(appliedNewStrings, ev.new)
	}

	// Restore CRLF if original file had it
	if hasCRLF {
		content = restoreCRLF(content)
	}

	// Encode back and write atomically
	encoded, err := EncodeWithEncoding(content, encName)
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error encoding content with encoding %s: %v", encName, err), IsError: true}
	}

	if err := WriteFileAtomically(fp, encoded); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}

	if e.registry != nil {
		e.registry.MarkFileReadWithContent(fp, content)
	}

	return ToolResult{Output: fmt.Sprintf("Successfully edited %s (encoding: %s, applied %d edits)", fp, encName, len(edits))}
}

func getEncodingParam(params map[string]any) string {
	enc, _ := params["encoding"].(string)
	enc = strings.ToLower(strings.TrimSpace(enc))
	// Normalize common aliases
	switch enc {
	case "latin-1", "latin1", "iso8859-1":
		return "iso-8859-1"
	case "gb2312":
		return "gbk"
	case "cp1252", "cp-1252", "win1252":
		return "windows-1252"
	case "cp936", "cp-936":
		return "gbk"
	case "cp950", "cp-950":
		return "big5"
	}
	return enc
}

func getParam(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := params[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}