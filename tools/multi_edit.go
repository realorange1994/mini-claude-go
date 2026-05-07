package tools

import (
	"fmt"
	"os"
	"strings"
)

// DESANITIZATIONS maps sanitized token strings to their original API format.
// These are applied when old_string fails to match — the LLM outputs sanitized
// versions of XML-like tokens that need to be restored before matching.
var DESANITIZATIONS = map[string]string{
	"<fnr>":           "<function_results>",
	"<n>":             "<name>",
	"</n>":            "</name>",
	"<o>":             "<output>",
	"</o>":            "</output>",
	"<e>":             "<error>",
	"</e>":            "</error>",
	"<s>":             "<system>",
	"</s>":            "</system>",
	"<r>":             "<result>",
	"</r>":            "</result>",
	"< META_START >":  "<META_START>",
	"< META_END >":    "<META_END>",
	"< EOT >":         "<EOT>",
	"< META >":        "<META>",
	"< SOS >":         "<SOS>",
	"\n\nH:":          "\n\nHuman:",
	"\n\nA:":            "\n\nAssistant:",
}

// MultiEditTool applies multiple search/replace edits atomically.
// If any old_string is not found, the entire operation is aborted.
type MultiEditTool struct {
	registry *Registry
}

func NewMultiEditTool(registry *Registry) *MultiEditTool {
	return &MultiEditTool{registry: registry}
}

func (*MultiEditTool) Name() string        { return "multi_edit" }
func (*MultiEditTool) Description() string { return "Apply multiple search/replace edits to a file atomically. If any edit fails, all are rolled back. You must read the file first with read_file before editing. Accepts a list of {old_string, new_string} pairs." }

func (*MultiEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to edit.",
			},
			"edits": map[string]any{
				"type":        "array",
				"description": "List of {old_string, new_string} edit operations.",
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
		"required": []string{"file_path", "edits"},
	}
}

func (*MultiEditTool) CheckPermissions(params map[string]any) PermissionResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		return PermissionResultPassthrough()
	}
	return CheckPathSafetyForAutoEdit(pathStr)
}

func (m *MultiEditTool) Execute(params map[string]any) ToolResult {
	pathStr, _ := params["file_path"].(string)
	if pathStr == "" {
		return ToolResult{Output: "Error: file_path is required", IsError: true}
	}

	fp := expandPath(pathStr)

	// SECURITY: Block UNC paths before any filesystem I/O to prevent NTLM credential leaks.
	if isUncPath(fp) {
		return ToolResult{Output: fmt.Sprintf("Error: UNC path access deferred: %s", pathStr), IsError: true}
	}

	// Read-before-write validation and concurrent modification detection.
	if m.registry != nil {
		if staleMsg := m.registry.CheckFileStale(fp); staleMsg != "" {
			return ToolResult{Output: staleMsg, IsError: true}
		}
	}

	editsRaw, ok := params["edits"]
	if !ok {
		return ToolResult{Output: "Error: edits is required", IsError: true}
	}
	editsSlice, ok := editsRaw.([]any)
	if !ok {
		return ToolResult{Output: "Error: edits must be an array", IsError: true}
	}
	if len(editsSlice) == 0 {
		return ToolResult{Output: "Error: edits must not be empty", IsError: true}
	}

	type edit struct {
		old        string
		new        string
		replaceAll bool
	}
	var edits []edit
	for i, e := range editsSlice {
		m, ok := e.(map[string]any)
		if !ok {
			return ToolResult{Output: fmt.Sprintf("Error: edit %d must be an object", i+1), IsError: true}
		}
		oldStr, _ := m["old_string"].(string)
		newStr, _ := m["new_string"].(string)
		replaceAll, _ := m["replace_all"].(bool)
		if oldStr == "" {
			return ToolResult{Output: fmt.Sprintf("Error: edit %d: old_string must not be empty", i+1), IsError: true}
		}
		edits = append(edits, edit{old: oldStr, new: newStr, replaceAll: replaceAll})
	}

	// 1 GiB file size guard (matching official Claude Code behavior)
	// Stat first to avoid loading huge files into memory
	const maxEditSize = 1 << 30 // 1 GiB
	if info, err := os.Stat(fp); err == nil && info.Size() > maxEditSize {
		return ToolResult{Output: fmt.Sprintf("Error: file too large (%d bytes, max %d bytes). Use offset/limit to read portions.", info.Size(), maxEditSize), IsError: true}
	}
	data, err := os.ReadFile(fp)
	if os.IsNotExist(err) {
		return ToolResult{Output: fmt.Sprintf("Error: file not found: %s", fp), IsError: true}
	}
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}
	}

	// Normalize CRLF
	content := string(data)
	hasCRLF := strings.Contains(content, "\r\n")
	if hasCRLF {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		for i := range edits {
			edits[i].old = strings.ReplaceAll(edits[i].old, "\r\n", "\n")
			edits[i].new = strings.ReplaceAll(edits[i].new, "\r\n", "\n")
		}
	}

	// Normalize curly quotes to straight quotes (matching official)
	content = normalizeQuotes(content)
	for i := range edits {
		edits[i].old = normalizeQuotes(edits[i].old)
		edits[i].new = normalizeQuotes(edits[i].new)
	}

	// Track applied new strings for overlapping edit detection
	var appliedNewStrings []string

	// Dry run: validate all edits and detect overlapping
	for i, e := range edits {
		oldTrimmed := strings.TrimRight(e.old, "\n")

		// Overlapping edit detection: old_string must not be a substring of any previously applied new_string
		for _, prevNew := range appliedNewStrings {
			if oldTrimmed != "" && strings.Contains(prevNew, oldTrimmed) {
				return ToolResult{
					Output: fmt.Sprintf("Error: edit %d failed: old_string is a substring of a new_string from a previous edit", i+1),
					IsError: true,
				}
			}
		}

		// Find the edit location
		idx := findEditLocation(content, e.old)
		if idx < 0 {
			// Try desanitized version of old_string
			desanitizedOld := desanitize(e.old)
			desanitizedNew := desanitize(e.new)
			idx = findEditLocation(content, desanitizedOld)
			if idx >= 0 {
				edits[i].old = desanitizedOld
				edits[i].new = desanitizedNew
			}
		}
		if idx < 0 {
			return ToolResult{
				Output: fmt.Sprintf("Error: edit %d failed: old_text not found: %q", i+1, truncate(e.old, 80)),
				IsError: true,
			}
		}

		// Apply in test content
		if e.replaceAll {
			content = strings.ReplaceAll(content, e.old, e.new)
		} else {
			content = strings.Replace(content, e.old, e.new, 1)
		}
		appliedNewStrings = append(appliedNewStrings, e.new)
	}

	// Apply atomically
	if hasCRLF {
		content = RestoreCRLF(content)
	}
	if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
		return ToolResult{Output: fmt.Sprintf("Error writing file: %v", err), IsError: true}
	}

	// Mark file as read so subsequent edit/write checks still work
	if m.registry != nil {
		m.registry.MarkFileRead(fp)
	}

	return ToolResult{Output: fmt.Sprintf("Applied %d edits to %s", len(edits), fp)}
}

// findEditLocation finds old_string in content, first trying exact match, then
// with trailing whitespace stripped.
func findEditLocation(content, old string) int {
	idx := strings.Index(content, old)
	if idx >= 0 {
		return idx
	}
	// Try with trailing newlines stripped (matching official)
	trimmed := strings.TrimRight(old, "\n")
	if trimmed != old {
		return strings.Index(content, trimmed)
	}
	return -1
}

// desanitize applies all known sanitization reversals to a string.
func desanitize(s string) string {
	result := s
	for from, to := range DESANITIZATIONS {
		result = strings.ReplaceAll(result, from, to)
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	end := maxLen
	// Adjust to safe UTF-8 boundary
	for end > 0 && (s[end]&0xc0) == 0x80 {
		end--
	}
	return s[:end] + "..."
}
