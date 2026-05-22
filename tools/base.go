package tools

import (
	"bytes"
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
// Maps upstream tool names to our internal names (bidirectional).
// Used by InternalToUpstreamName / UpstreamToInternalName for rule matching.
var UpstreamToolAliases = map[string]string{
	"Read":  FileReadToolName,
	"Write": FileWriteToolName,
	"Edit":  FileEditToolName,
	"Bash":  ExecToolName,
}

// ToolNameAliases maps common LLM tool name variations to our canonical names.
// This is a one-way mapping (alias → canonical), used by Registry.Resolve().
// LLMs frequently use non-canonical names (read, bash, cat, etc.) which would
// cause "unknown tool" errors without alias resolution. Keeping tool schemas
// stable preserves cache hit rates because the tool definition prefix doesn't
// change. Inspired by openclacky's TOOL_ALIASES in tool_registry.rb.
var ToolNameAliases = map[string]string{
	// file_reader / read_file aliases
	"read":       FileReadToolName,
	"filereader": FileReadToolName,
	"file_read":  FileReadToolName,
	"cat":        FileReadToolName,
	// write_file aliases
	"write":       FileWriteToolName,
	"create_file": FileWriteToolName,
	"file_write":  FileWriteToolName,
	// edit_file aliases
	"edit":            FileEditToolName,
	"replace":         FileEditToolName,
	"replace_in_file": FileEditToolName,
	"str_replace":     FileEditToolName,
	"file_edit":       FileEditToolName,
	// exec aliases
	"bash":        ExecToolName,
	"shell":       ExecToolName,
	"terminal":    ExecToolName,
	"execute":     ExecToolName,
	"run_command": ExecToolName,
	"run":         ExecToolName,
	"command":     ExecToolName,
	// grep aliases
	"search_files":    "grep",
	"search_in_files": "grep",
	"find_in_files":   "grep",
	"search_code":     "grep",
	// glob aliases
	"find_files":       "glob",
	"list_files":       "glob",
	"file_glob":        "glob",
	"search_filenames": "glob",
	// web_search aliases
	"search":          "web_search",
	"websearch":       "web_search",
	"internet_search": "web_search",
	"online_search":   "web_search",
	// web_fetch aliases
	"fetch":     "web_fetch",
	"webfetch":  "web_fetch",
	"browse":    "web_fetch",
	"url_fetch": "web_fetch",
	"http_get":  "web_fetch",
	// list_dir aliases
	"ls":             "list_dir",
	"list_directory": "list_dir",
	"dir":            "list_dir",
	// multi_edit aliases
	"multi_edit_file": "multi_edit",
	"batch_edit":      "multi_edit",
	// agent aliases
	"subagent":    "agent",
	"sub_agent":   "agent",
	"spawn_agent": "agent",
	// git aliases
	"git_tool": "git",
	// skill aliases
	"skill":     "read_skill",
	"run_skill": "read_skill",
	// memory aliases
	"remember": "memory_add",
	// task aliases
	"todo":         "task_create",
	"task_manager": "task_create",
	// ask_user aliases
	"ask_user":   "AskUserQuestion",
	"ask":        "AskUserQuestion",
	"user_input": "AskUserQuestion",
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
	if internal == "" {
		return ""
	}
	for upstream, internalName := range UpstreamToolAliases {
		if internalName == internal {
			return upstream
		}
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
// Includes panic recovery so that no tool panic can crash the process.
func ExecuteWithContext(ctx context.Context, tool Tool, params map[string]any) (result ToolResult) {
	defer func() {
		if r := recover(); r != nil {
			result = ToolResult{
				Output:  fmt.Sprintf("Error: tool %s panicked: %v", tool.Name(), r),
				IsError: true,
			}
		}
	}()

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
	mtime         time.Time // file modification time at read time
	readTime      time.Time // when the file was read
	readOffset    int       // offset used when reading (-1 if from edit/write, not a read_file call)
	readLimit     int       // limit used when reading (-1 if from edit/write, not a read_file call)
	content       string    // file content at read time (for content-based staleness fallback)
	isPartial     bool      // true if this entry represents a partial (offset/limit) read_file call
	isPartialView bool      // true for auto-injected files (CLAUDE.md, MEMORY.md) where content differs from disk
	fromRead      bool      // true if this entry was created by a read_file call (vs edit/write)
}

const (
	maxFileStateEntries = 100 // max entries in filesRead LRU cache (matching upstream FileStateCache)
	maxFileStateBytes   = 25 * 1024 * 1024 // 25MB approximate content cache limit
)

// Registry collects tool instances and provides lookup + API schema generation.
type Registry struct {
	tools     map[string]Tool
	filesRead map[string]fileReadInfo // tracks which files have been read by read_file (LRU, max 100 entries)
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

// Resolve resolves a tool name (possibly misspelt or aliased) to the canonical
// registered name, then returns the tool. Resolution order (matching openclacky's
// ToolRegistry#resolve):
//  1. Exact match in the registry
//  2. Case-insensitive match (e.g. "Read" → "read_file")
//  3. Alias lookup (e.g. "bash" → "exec", "cat" → "read_file")
//  4. Hyphen-to-underscore normalization (e.g. "file-read" → "file_read")
//
// Returns the resolved canonical name and the tool, or the original name and nil
// if nothing matched. This prevents "unknown tool" errors when LLMs use common
// name variations, while keeping tool schemas stable for cache hit rates.
func (r *Registry) Resolve(name string) (string, Tool) {
	// 1. Exact match
	if t, ok := r.tools[name]; ok {
		return name, t
	}

	downcased := strings.ToLower(name)

	// 2. Case-insensitive match
	for registeredName, tool := range r.tools {
		if strings.ToLower(registeredName) == downcased {
			return registeredName, tool
		}
	}

	// 3. Alias lookup
	if canonical, ok := ToolNameAliases[downcased]; ok {
		if t, ok := r.tools[canonical]; ok {
			return canonical, t
		}
	}
	// Alias lookup: upstream names (Read, Write, Edit, Bash)
	if canonical, ok := UpstreamToolAliases[name]; ok {
		if t, ok := r.tools[canonical]; ok {
			return canonical, t
		}
	}

	// 4. Hyphen-to-underscore normalization (e.g. "file-read" → "file_read")
	normalized := strings.ReplaceAll(downcased, "-", "_")
	if normalized != downcased {
		if t, ok := r.tools[normalized]; ok {
			return normalized, t
		}
		if canonical, ok := ToolNameAliases[normalized]; ok {
			if t, ok := r.tools[canonical]; ok {
				return canonical, t
			}
		}
	}

	return name, nil
}

// AllTools returns all registered tools, partitioned for prompt caching.
// Built-in tools come first (sorted alphabetically), then MCP-related tools
// (sorted alphabetically). This separation ensures that MCP tool changes
// don't shift built-in tools' positions in the serialized tool array,
// preserving the ~11K-token cached prefix. Inspired by upstream's
// toolPool.ts partitioned sorting: [...builtIn.sort(byName), ...mcp.sort(byName)].
func (r *Registry) AllTools() []Tool {
	var builtin, mcp []Tool
	for _, t := range r.tools {
		if isMCPToolName(t.Name()) {
			mcp = append(mcp, t)
		} else {
			builtin = append(builtin, t)
		}
	}
	sort.Slice(builtin, func(i, j int) bool { return builtin[i].Name() < builtin[j].Name() })
	sort.Slice(mcp, func(i, j int) bool { return mcp[i].Name() < mcp[j].Name() })
	out := make([]Tool, 0, len(r.tools))
	out = append(out, builtin...)
	out = append(out, mcp...)
	return out
}

// isMCPToolName identifies MCP-related tools that should be sorted separately
// from built-in tools. Partitioning prevents MCP tool changes from shifting
// built-in tool positions, which would break the cached tool schema prefix.
func isMCPToolName(name string) bool {
	return strings.HasPrefix(name, "mcp_")
}

// MarkFileRead records that a file has been read by read_file, storing its current mtime.
func (r *Registry) MarkFileRead(path string) {
	r.MarkFileReadWithParams(path, -1, -1, "", false, false, false) // edit/write: not partial, not partialView, not from read
}

// MarkFileReadWithContent records a file read with content for staleness fallback.
// Used by edit/write operations that know the post-write content.
func (r *Registry) MarkFileReadWithContent(path string, content string) {
	r.MarkFileReadWithParams(path, -1, -1, content, false, false, false) // edit/write: not partial, not partialView, not from read
}

// MarkFileReadAsPartialView marks a file as auto-injected with content that
// may differ from disk (e.g., CLAUDE.md, MEMORY.md in certain contexts).
// Edit/write tools will require a fresh read_file first.
func (r *Registry) MarkFileReadAsPartialView(path, content string) {
	r.MarkFileReadWithParams(path, -1, -1, content, false, true, false) // isPartialView=true
}

// MarkFileReadWithParams records that a file has been read, storing offset/limit
// and content for dedup detection and content-based staleness fallback.
// Use offset=-1, limit=-1, isPartial=false, isPartialView=false, fromRead=false for edit/write operations.
func (r *Registry) MarkFileReadWithParams(path string, offset, limit int, content string, isPartial bool, isPartialView bool, fromRead bool) {
	// Canonicalize the path to handle relative paths consistently.
	normalized := canonicalPath(path)
	r.mu.Lock()
	if info, err := os.Stat(path); err == nil {
		r.filesRead[normalized] = fileReadInfo{mtime: info.ModTime(), readTime: time.Now(), readOffset: offset, readLimit: limit, content: content, isPartial: isPartial, isPartialView: isPartialView, fromRead: fromRead}
	} else {
		r.filesRead[normalized] = fileReadInfo{readTime: time.Now(), readOffset: offset, readLimit: limit, content: content, isPartial: isPartial, isPartialView: isPartialView, fromRead: fromRead} // new file, no mtime yet
	}
	r.evictFileStateLRU()
	r.mu.Unlock()
}

// evictFileStateLRU evicts oldest entries when the filesRead cache exceeds
// maxFileStateEntries. Must be called with r.mu held.
func (r *Registry) evictFileStateLRU() {
	for len(r.filesRead) > maxFileStateEntries {
		// Find the entry with the oldest readTime
		var oldest string
		var oldestTime time.Time
		first := true
		for k, info := range r.filesRead {
			if first || info.readTime.Before(oldestTime) {
				oldest = k
				oldestTime = info.readTime
				first = false
			}
		}
		if oldest == "" {
			break
		}
		delete(r.filesRead, oldest)
	}
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

// GetCachedFileContent returns the cached content for a previously read file.
// Returns empty string if the file hasn't been read or has no cached content.
// Used by post-compact recovery to inject file content that the model saw
// at read time, matching upstream's readFileState.content pattern.
func (r *Registry) GetCachedFileContent(path string) string {
	r.mu.RLock()
	info, ok := r.filesRead[canonicalPath(path)]
	r.mu.RUnlock()
	if !ok {
		return ""
	}
	return info.content
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

	// isPartialView check: if the file was auto-injected (CLAUDE.md, MEMORY.md)
	// and its content differs from disk, editing would be based on stale data.
	// The model must do a fresh read_file first.
	if storedInfo.isPartialView {
		return "Error: file content was auto-injected and may differ from disk. You must do a fresh read_file before editing."
	}

	// Note: partial (offset/limit) reads do NOT block subsequent edits.
	// Matching upstream: the edit tool re-reads the full file from disk during
	// its call() phase, so partial reads are fully functional for edits.
	// The mtime check below handles concurrent modification detection for both
	// full and partial reads.

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
		if currentData, err := os.ReadFile(path); err == nil {
			// Decode the current file content the same way we decoded it
			// during read_file (strip BOM, normalize CRLF→LF, decode UTF-16).
			// This ensures we compare normalized content against normalized content,
			// not raw bytes against normalized string.
			currentContent, _ := DecodeFileContent(currentData)
			if currentContent == storedInfo.content {
				// Content unchanged despite timestamp change — safe to proceed
				return ""
			}
			// Also compare raw bytes as a secondary check (handles cases where
			// stored content came from edit/write which stores pre-encode content)
			if bytes.Equal(currentData, []byte(storedInfo.content)) {
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
// Tools are returned in alphabetical order by name to ensure deterministic
// ordering for prompt caching across requests.
func (r *Registry) APISchemas() []map[string]any {
	tools := r.AllTools() // already sorted
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{
			"name":         t.Name(),
			"description":  t.Description(),
			"input_schema": t.InputSchema(),
		})
	}
	return out
}