package tools

import (
	"context"
	"fmt"
	"os"
	stdpath "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tool name constants for rule system integration.
// These are the internal names used by our tools.
const (
	FileReadToolName  = "read_file"
	FileWriteToolName = "write_file"
	FileEditToolName  = "edit_file"
	ExecToolName      = "exec"
	GitToolName       = "git"
)

// Upstream tool name aliases for rule system compatibility.
// Maps upstream tool names to our internal names.
var UpstreamToolAliases = map[string]string{
	"Read":  FileReadToolName,
	"Write": FileWriteToolName,
	"Edit":  FileEditToolName,
	"Bash":  ExecToolName,
}

// canonicalPath normalizes a file path for consistent registry lookups.
// It expands ~, resolves relative paths to absolute, converts backslashes to
// forward slashes, and lowercases the result. This ensures that different
// representations of the same file (e.g., "foo.txt" vs "./foo.txt" vs
// "E:\workspace\foo.txt") all map to the same key in the registry.
func canonicalPath(path string) string {
	// First expand ~/foo style paths
	expanded := expandPath(path)
	// Then resolve to absolute to handle relative paths like ./foo.txt or ../bar.txt
	if !filepath.IsAbs(expanded) {
		if abs, err := filepath.Abs(expanded); err == nil {
			expanded = abs
		}
	}
	return normalizeFilePath(expanded)
}

// InternalToUpstreamName maps an internal tool name to its upstream name.
// Returns the input unchanged if no alias exists.
func InternalToUpstreamName(internal string) string {
	if upstream, ok := UpstreamToolAliases[internal]; ok {
		return upstream
	}
	return internal
}
type ToolResult struct {
	Output   string
	IsError  bool
	Metadata ToolResultMetadata
}

// ToolResultOK creates a successful ToolResult.
func ToolResultOK(output string) ToolResult {
	return ToolResult{Output: output, IsError: false}
}

// ToolResultError creates an error ToolResult.
func ToolResultError(msg string) ToolResult {
	return ToolResult{Output: msg, IsError: true}
}

// WithMetadata returns the ToolResult with metadata set.
func (r ToolResult) WithMetadata(meta ToolResultMetadata) ToolResult {
	r.Metadata = meta
	return r
}

// ToolResultMetadata holds structured metadata about a tool execution.
type ToolResultMetadata struct {
	ToolName    string
	ExitCode    int
	ExitCodeSet bool  // true when ExitCode was explicitly set (distinguishes 0 from not-set)
	DurationMs  int64
	OutputLines int
	Truncated   bool
}

// NewToolResultMetadata creates metadata with tool name and exit code explicitly set.
func NewToolResultMetadata(toolName string, exitCode int) ToolResultMetadata {
	return ToolResultMetadata{ToolName: toolName, ExitCode: exitCode, ExitCodeSet: true}
}

// HasExitCode returns true if ExitCode was explicitly set.
func (m ToolResultMetadata) HasExitCode() bool {
	return m.ExitCodeSet
}

// IsError returns true if the tool execution resulted in an error.
func (m ToolResultMetadata) IsError() bool {
	if m.ExitCodeSet && m.ExitCode != 0 {
		return true
	}
	return false
}

// ToCompactSummary returns a one-line summary of the tool result for display.
func (m ToolResultMetadata) ToCompactSummary(output string) string {
	status := "ok"
	if m.ExitCodeSet && m.ExitCode != 0 {
		status = "error"
	}
	if !m.Truncated && strings.Contains(output, "Error:") {
		status = "error"
	}

	lineCount := m.OutputLines
	if lineCount == 0 {
		lineCount = strings.Count(output, "\n") + 1
	}

	durationStr := ""
	if m.DurationMs >= 1000 {
		durationStr = fmt.Sprintf(", %.1fs", float64(m.DurationMs)/1000.0)
	} else if m.DurationMs > 0 {
		durationStr = fmt.Sprintf(", %dms", m.DurationMs)
	}

	if m.ToolName == "" {
		return fmt.Sprintf("-> %s, %d lines%s", status, lineCount, durationStr)
	}
	return fmt.Sprintf("[%s] -> %s, %d lines%s", m.ToolName, status, lineCount, durationStr)
}

// PermissionBehavior represents the result of a tool's permission self-check.
// Matches upstream's PermissionResult behavior: allow, deny, ask, passthrough.
type PermissionBehavior int

const (
	// PermissionAllow grants permission without user interaction.
	PermissionAllow PermissionBehavior = iota
	// PermissionDeny is a hard denial that is bypass-immune (even in bypass mode).
	PermissionDeny
	// PermissionAsk requires user approval. When from a safetyCheck, it is bypass-immune.
	PermissionAsk
	// PermissionPassthrough defers to the framework's mode-based logic.
	PermissionPassthrough
)

// PermissionResult is the structured return type for CheckPermissions.
// Replaces the previous string return type to distinguish deny (bypass-immune)
// from ask (may be bypass-immune depending on DecisionReason).
type PermissionResult struct {
	Behavior             PermissionBehavior
	Message              string // human-readable reason (required for deny/ask)
	DecisionReason       string // "safetyCheck", "rule", "tool", or ""
	MatchedRule          string // optional: the rule that matched (for debugging)
	ClassifierApprovable bool   // true if auto-mode classifier may approve this ask; false if must always prompt
}

// PermissionResultAllow returns a simple allow result.
func PermissionResultAllow() PermissionResult {
	return PermissionResult{Behavior: PermissionAllow}
}

// PermissionResultDeny returns a hard denial (bypass-immune).
func PermissionResultDeny(msg string) PermissionResult {
	return PermissionResult{Behavior: PermissionDeny, Message: msg, DecisionReason: "tool"}
}

// PermissionResultAsk returns an ask result (user approval required).
// ClassifierApprovable defaults to true (auto-mode classifier may approve).
func PermissionResultAsk(msg string, reason string) PermissionResult {
	return PermissionResult{Behavior: PermissionAsk, Message: msg, DecisionReason: reason, ClassifierApprovable: true}
}

// PermissionResultAskNotClassifiable returns an ask result that the auto-mode
// classifier cannot approve (must always prompt user). Used for suspicious
// Windows path patterns that are too dangerous for automated approval.
func PermissionResultAskNotClassifiable(msg string, reason string) PermissionResult {
	return PermissionResult{Behavior: PermissionAsk, Message: msg, DecisionReason: reason, ClassifierApprovable: false}
}

// PermissionResultPassthrough returns a passthrough result (defer to framework).
func PermissionResultPassthrough() PermissionResult {
	return PermissionResult{Behavior: PermissionPassthrough}
}

// IsBypassImmune returns true if this result should NOT be overridden by bypass mode.
// Deny is always bypass-immune. Ask from safetyCheck is bypass-immune.
func (r PermissionResult) IsBypassImmune() bool {
	switch r.Behavior {
	case PermissionDeny:
		return true
	case PermissionAsk:
		return r.DecisionReason == "safetyCheck"
	default:
		return false
	}
}

// Tool is the interface all tools must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	CheckPermissions(params map[string]any) PermissionResult
	Execute(params map[string]any) ToolResult
}

// ContextTool is an optional interface that tools can implement to support
// context-based cancellation during execution.
type ContextTool interface {
	Tool
	ExecuteContext(ctx context.Context, params map[string]any) ToolResult
}

// ExecuteWithContext calls ExecuteContext if the tool implements ContextTool,
// otherwise falls back to Execute (ignoring the context).
func ExecuteWithContext(ctx context.Context, tool Tool, params map[string]any) ToolResult {
	if ct, ok := tool.(ContextTool); ok {
		return ct.ExecuteContext(ctx, params)
	}
	return tool.Execute(params)
}

// ValidateParams checks that required parameters are present and enum values are valid.
func ValidateParams(tool Tool, params map[string]any) error {
	schema := tool.InputSchema()

	// Check required parameters
	required, ok := schema["required"].([]string)
	if ok {
		for _, key := range required {
			if _, exists := params[key]; !exists {
				return fmt.Errorf("missing required parameter: %q", key)
			}
		}
	}

	// Check enum values
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	for key, propVal := range props {
		prop, ok := propVal.(map[string]any)
		if !ok {
			continue
		}
		argVal, exists := params[key]
		if !exists {
			continue
		}

		// Enum validation
		if enum, ok := prop["enum"].([]any); ok {
			valid := false
			for _, e := range enum {
				if fmt.Sprintf("%v", e) == fmt.Sprintf("%v", argVal) {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("parameter %q must be one of %v, got %v", key, enum, argVal)
			}
		}
	}
	return nil
}

// fileReadInfo tracks both the file's mtime (for staleness checks) and when it was read (for recency sorting).
// Also tracks the offset and limit used during read, for dedup detection (file_unchanged stub).
type fileReadInfo struct {
	mtime      time.Time // file modification time at read time
	readTime   time.Time // when the file was read
	readOffset int       // offset used when reading (-1 if from edit/write, not a read_file call)
	readLimit  int       // limit used when reading (-1 if from edit/write, not a read_file call)
	content    string    // file content at read time (for content-based staleness fallback)
	isPartial  bool      // true if this entry represents a partial (offset/limit) read_file call
	fromRead   bool      // true if this entry was created by a read_file call (vs edit/write)
}

// Registry collects tool instances and provides lookup + API schema generation.
type Registry struct {
	tools     map[string]Tool
	filesRead map[string]fileReadInfo // tracks which files have been read by read_file
	mu        sync.RWMutex            // protects filesRead
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:     make(map[string]Tool),
		filesRead: make(map[string]fileReadInfo),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns the tool by name and whether it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// AllTools returns all registered tools.
func (r *Registry) AllTools() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// MarkFileRead records that a file has been read by read_file, storing its current mtime.
func (r *Registry) MarkFileRead(path string) {
	r.MarkFileReadWithParams(path, -1, -1, "", false, false) // edit/write: not partial, not from read
}

// MarkFileReadWithContent records a file read with content for staleness fallback.
// Used by edit/write operations that know the post-write content.
func (r *Registry) MarkFileReadWithContent(path string, content string) {
	r.MarkFileReadWithParams(path, -1, -1, content, false, false) // edit/write: not partial, not from read
}

// MarkFileReadWithParams records that a file has been read, storing offset/limit
// and content for dedup detection and content-based staleness fallback.
// Use offset=-1, limit=-1, isPartial=false, fromRead=false for edit/write operations.
func (r *Registry) MarkFileReadWithParams(path string, offset, limit int, content string, isPartial bool, fromRead bool) {
	// Canonicalize the path to handle relative paths consistently.
	normalized := canonicalPath(path)
	r.mu.Lock()
	if info, err := os.Stat(path); err == nil {
		r.filesRead[normalized] = fileReadInfo{mtime: info.ModTime(), readTime: time.Now(), readOffset: offset, readLimit: limit, content: content, isPartial: isPartial, fromRead: fromRead}
	} else {
		r.filesRead[normalized] = fileReadInfo{readTime: time.Now(), readOffset: offset, readLimit: limit, content: content, isPartial: isPartial, fromRead: fromRead} // new file, no mtime yet
	}
	r.mu.Unlock()
}

// HasFileBeenRead checks if a file has been read by read_file.
func (r *Registry) HasFileBeenRead(path string) bool {
	r.mu.RLock()
	_, ok := r.filesRead[canonicalPath(path)]
	r.mu.RUnlock()
	return ok
}

// CheckFileRead returns the stored read info and whether the file has been read.
// Used by the dedup check in read_file to avoid re-sending unchanged content.
func (r *Registry) CheckFileRead(path string) (fileReadInfo, bool) {
	r.mu.RLock()
	info, ok := r.filesRead[canonicalPath(path)]
	r.mu.RUnlock()
	return info, ok
}

// CheckFileStale returns an error message if the file was modified since last read.
// Returns empty string if the file is safe to edit.
func (r *Registry) CheckFileStale(path string) string {
	// Canonicalize the path to handle relative paths consistently.
	normalized := canonicalPath(path)

	// New file creation: file doesn't exist yet, allow without read
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ""
	}

	r.mu.RLock()
	storedInfo, wasRead := r.filesRead[normalized]
	r.mu.RUnlock()
	if !wasRead {
		return "Error: file has not been read. For existing files, you MUST use read_file first before write_file/edit_file. For new files, write_file works directly without reading. To modify an existing file: 1) read_file to read it, 2) edit_file for small changes or write_file for complete rewrites."
	}

	// Partial-view check: if the file was only partially read (with
	// offset/limit), the model must do a fresh full read before editing.
	if storedInfo.isPartial {
		return "Error: file was only partially read (with offset/limit). You must do a fresh full read (read_file without offset/limit parameters) before editing."
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "" // file was deleted, not a staleness issue
		}
		return fmt.Sprintf("Error: cannot check file status: %v", err)
	}

	// File hasn't been modified since we read it
	if info.ModTime() == storedInfo.mtime {
		return ""
	}

	// Timestamp changed. On Windows, timestamps can change without content changes
	// (cloud sync, antivirus, etc.). For full reads where we have stored content,
	// compare content as a fallback to avoid false positives.
	isFullRead := !storedInfo.isPartial
	if isFullRead && storedInfo.content != "" {
		if currentContent, err := os.ReadFile(path); err == nil {
			if string(currentContent) == storedInfo.content {
				// Content unchanged despite timestamp change — safe to proceed
				return ""
			}
		}
	}

	return "Error: file has been modified since read, either by the user or by a linter. Read it again before attempting to write it."
}

// ClearFilesRead clears the read-file tracking (e.g., on /clear).
func (r *Registry) ClearFilesRead() {
	r.mu.Lock()
	r.filesRead = make(map[string]fileReadInfo)
	r.mu.Unlock()
}

// GetRecentlyReadFiles returns the paths of files that have been read,
// sorted by most recently read first. Returns up to maxFiles paths.
func (r *Registry) GetRecentlyReadFiles(maxFiles int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	type pathTime struct {
		path     string
		readTime time.Time
	}
	var entries []pathTime
	for p, info := range r.filesRead {
		entries = append(entries, pathTime{p, info.readTime})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].readTime.After(entries[j].readTime)
	})

	limit := maxFiles
	if limit > len(entries) {
		limit = len(entries)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = entries[i].path
	}
	return result
}

// normalizeFilePath normalizes a path for consistent comparison.
// Cleans . and .. components, converts backslashes, and lowercases.
func normalizeFilePath(filePath string) string {
	p := strings.ReplaceAll(filePath, "\\", "/")
	// Clean . and .. components using path.Clean (works on forward-slash paths)
	p = stdpath.Clean(p)
	// Re-normalize in case path.Clean introduced backslashes on Windows
	p = strings.ReplaceAll(p, "\\", "/")
	return strings.ToLower(p)
}

// IsPathAllowed checks that a resolved file path is within the working directory.
// Returns an error message if the path escapes the project, or empty string if allowed.
func IsPathAllowed(path string) string {
	resolved := expandPath(path)
	// Make absolute if not already
	if !filepath.IsAbs(resolved) {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		resolved = filepath.Join(wd, resolved)
	}
	resolved = filepath.Clean(resolved)

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	absWd, err := filepath.Abs(wd)
	if err != nil {
		return ""
	}
	absWd = filepath.Clean(absWd)

	// Resolve symlinks on both sides for robustness
	if evaled, err := filepath.EvalSymlinks(absWd); err == nil {
		absWd = evaled
	}
	if evaled, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = evaled
	}

	rel, err := filepath.Rel(absWd, resolved)
	if err != nil {
		return fmt.Sprintf("Error: path %q is outside the project directory", path)
	}
	// filepath.Rel uses .. to indicate paths above the base
	if strings.HasPrefix(rel, "..") {
		return fmt.Sprintf("Error: path %q is outside the project directory", path)
	}
	return ""
}

// RestoreCRLF restores CRLF line endings in text that was normalized to LF.
// Uses O(n) algorithm: only adds \r before \n that wasn't already preceded by \r.
func RestoreCRLF(s string) string {
	var b strings.Builder
	b.Grow(len(s) + len(s)/10)
	prevCR := false
	for _, c := range s {
		if c == '\n' && !prevCR {
			b.WriteByte('\r')
		}
		prevCR = c == '\r'
		b.WriteRune(c)
	}
	return b.String()
}

// APISchemas builds the tool definitions for the Anthropic API.
func (r *Registry) APISchemas() []map[string]any {
	out := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, map[string]any{
			"name":         t.Name(),
			"description":  t.Description(),
			"input_schema": t.InputSchema(),
		})
	}
	return out
}