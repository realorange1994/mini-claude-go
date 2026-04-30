package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pmezard/go-difflib/difflib"

	"miniclaudecode-go/tools"
)

// ─── file_history ───

type FileHistoryTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryTool) Name() string       { return "file_history" }
func (t *FileHistoryTool) Description() string  { return "List version history for a file or all files" }
func (t *FileHistoryTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path to show history for. Omit to list all files."},
			"pattern": map[string]any{"type": "string", "description": "Glob pattern to filter files (when listing all)"},
			"offset":  map[string]any{"type": "integer", "minimum": 0, "description": "Pagination offset for file list"},
			"limit":   map[string]any{"type": "integer", "minimum": 1, "default": 10, "description": "Max files to list"},
		},
	}
}
func (t *FileHistoryTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, hasPath := params["path"].(string)
	if hasPath && pathVal != "" {
		return t.showFileHistory(pathVal)
	}
	return t.listAllFiles(params)
}

func (t *FileHistoryTool) showFileHistory(path string) tools.ToolResult {
	fullPath := expandPath(path)
	snaps := t.History.ListSnapshots(fullPath)
	if len(snaps) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("No history for %s", fullPath), IsError: false}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (%d versions)\n", snaps[0].FilePath, len(snaps)))

	i := 0
	for i < len(snaps) {
		if i+1 < len(snaps) && snaps[i].Checksum == snaps[i+1].Checksum && snaps[i].Checksum != "" {
			desc := snaps[i+1].Description
			if desc == "" {
				desc = snaps[i].Description
			}
			version := i + 1
			line := fmt.Sprintf("  [v%d] %s - %d bytes", version, snaps[i].Timestamp.Format("2006-01-02 15:04:05"), len(snaps[i].Content))
			if desc != "" {
				line += " -- " + desc
			}
			line += " (merged)"
			sb.WriteString(line + "\n")
			i += 2
			continue
		}
		snap := snaps[i]
		version := i + 1
		desc := snap.Description
		if desc == "" && len(snap.Content) == 0 {
			desc = "(file did not exist)"
		}
		if snap.Deleted {
			desc = "(file deleted)"
		}
		line := fmt.Sprintf("  [v%d] %s - %d bytes", version, snap.Timestamp.Format("2006-01-02 15:04:05"), len(snap.Content))
		if desc != "" {
			line += " -- " + desc
		}
		if i == len(snaps)-1 {
			line += " (current)"
		}
		sb.WriteString(line + "\n")
		i++
	}
	return tools.ToolResult{Output: sb.String()}
}

func (t *FileHistoryTool) listAllFiles(params map[string]any) tools.ToolResult {
	files := t.History.ListAllFiles()
	if pattern, ok := params["pattern"].(string); ok && pattern != "" {
		var filtered []string
		for _, f := range files {
			if matched, _ := filepath.Match(pattern, filepath.Base(f)); matched {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	offset := 0
	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}
	limit := 10
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}
	if offset >= len(files) {
		offset = 0
	}
	end := offset + limit
	if end > len(files) {
		end = len(files)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Files with history (%d total):\n", len(files)))
	for _, f := range files[offset:end] {
		count := t.History.SnapshotCount(f)
		snaps := t.History.ListSnapshots(f)
		latest := ""
		if len(snaps) > 0 {
			latest = snaps[len(snaps)-1].Description
		}
		if latest != "" {
			sb.WriteString(fmt.Sprintf("  %s -- %d versions, latest: %s\n", f, count, latest))
		} else {
			sb.WriteString(fmt.Sprintf("  %s -- %d versions\n", f, count))
		}
	}
	return tools.ToolResult{Output: sb.String()}
}

// ─── file_history_read ───

type FileHistoryReadTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryReadTool) Name() string       { return "file_history_read" }
func (t *FileHistoryReadTool) Description() string  { return "Read content of a specific version from file history" }
func (t *FileHistoryReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path"},
			"version": map[string]any{"type": "integer", "minimum": 1, "description": "Version number (1=oldest). Omit for current."},
			"offset":  map[string]any{"type": "integer", "minimum": 1, "default": 1, "description": "Line offset (1-indexed)"},
			"limit":   map[string]any{"type": "integer", "minimum": 1, "default": 2000, "description": "Max lines to show"},
		},
		"required": []string{"path"},
	}
}
func (t *FileHistoryReadTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryReadTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	fullPath := expandPath(pathVal)
	snaps := t.History.ListSnapshots(fullPath)
	if len(snaps) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("No history for %s", fullPath), IsError: true}
	}
	version := len(snaps)
	if v, ok := params["version"].(float64); ok {
		version = int(v)
	}
	if version < 1 || version > len(snaps) {
		return tools.ToolResult{Output: fmt.Sprintf("Version %d out of range (1-%d)", version, len(snaps)), IsError: true}
	}
	snap := snaps[version-1]
	lines := strings.Split(snap.Content, "\n")
	offset := 1
	if v, ok := params["offset"].(float64); ok {
		offset = int(v)
	}
	limit := 2000
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}
	if offset < 1 {
		offset = 1
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	end := offset + limit
	if end > len(lines)+1 {
		end = len(lines) + 1
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (v%d, %s)\n", snap.FilePath, version, snap.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Lines %d-%d of %d:\n", offset, end-1, len(lines)))
	for i := offset - 1; i < end-1 && i < len(lines); i++ {
		sb.WriteString(fmt.Sprintf("%6d\t%s\n", i+1, lines[i]))
	}
	return tools.ToolResult{Output: sb.String()}
}

// ─── file_history_grep ───

type FileHistoryGrepTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryGrepTool) Name() string       { return "file_history_grep" }
func (t *FileHistoryGrepTool) Description() string  { return "Search within file history versions using regex" }
func (t *FileHistoryGrepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":     map[string]any{"type": "string", "description": "Regex pattern to search for"},
			"path":        map[string]any{"type": "string", "description": "File path. Omit to search all files."},
			"version":     map[string]any{"type": "integer", "minimum": 1, "description": "Specific version to search"},
			"context":     map[string]any{"type": "integer", "minimum": 0, "default": 2, "description": "Context lines around match"},
			"ignore_case": map[string]any{"type": "boolean", "default": false, "description": "Case-insensitive search"},
		},
		"required": []string{"pattern"},
	}
}
func (t *FileHistoryGrepTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryGrepTool) Execute(params map[string]any) tools.ToolResult {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"pattern\"", IsError: true}
	}
	ignoreCase := false
	if v, ok := params["ignore_case"].(bool); ok {
		ignoreCase = v
	}
	rePattern := pattern
	if ignoreCase {
		rePattern = "(?i)" + rePattern
	}
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Invalid regex: %v", err), IsError: true}
	}
	context := 2
	if v, ok := params["context"].(float64); ok {
		context = int(v)
	}
	pathVal, hasPath := params["path"].(string)
	if hasPath && pathVal != "" {
		return t.grepFile(pathVal, re, context, params)
	}
	return t.grepAllFiles(re, context)
}

func (t *FileHistoryGrepTool) grepFile(path string, re *regexp.Regexp, context int, params map[string]any) tools.ToolResult {
	fullPath := expandPath(path)
	snaps := t.History.ListSnapshots(fullPath)
	if len(snaps) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("No history for %s", fullPath), IsError: true}
	}
	version := len(snaps)
	if v, ok := params["version"].(float64); ok {
		version = int(v)
	}
	if version < 1 || version > len(snaps) {
		return tools.ToolResult{Output: fmt.Sprintf("Version %d out of range", version), IsError: true}
	}
	snap := snaps[version-1]
	lines := strings.Split(snap.Content, "\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (v%d)\n", snap.FilePath, version))
	matchCount := 0
	for i, line := range lines {
		if re.MatchString(line) {
			matchCount++
			start := i - context
			if start < 0 {
				start = 0
			}
			end := i + context + 1
			if end > len(lines) {
				end = len(lines)
			}
			for j := start; j < end; j++ {
				prefix := "  "
				if j == i {
					prefix = "> "
				}
				sb.WriteString(fmt.Sprintf("%s%d: %s\n", prefix, j+1, lines[j]))
			}
		}
	}
	sb.WriteString(fmt.Sprintf("%d matches\n", matchCount))
	return tools.ToolResult{Output: sb.String()}
}

func (t *FileHistoryGrepTool) grepAllFiles(re *regexp.Regexp, context int) tools.ToolResult {
	files := t.History.ListAllFiles()
	var sb strings.Builder
	totalMatches := 0
	for _, fp := range files {
		snaps := t.History.ListSnapshots(fp)
		if len(snaps) == 0 {
			continue
		}
		snap := snaps[len(snaps)-1]
		lines := strings.Split(snap.Content, "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				totalMatches++
				preview := line
				if len(preview) > 200 {
					preview = preview[:200]
				}
				sb.WriteString(fmt.Sprintf("%s:%d: %s\n", fp, i+1, preview))
			}
		}
	}
	sb.WriteString(fmt.Sprintf("%d matches across %d files\n", totalMatches, len(files)))
	return tools.ToolResult{Output: sb.String()}
}

// ─── file_restore ───

type FileRestoreTool struct {
	History *SnapshotHistory
}

func (t *FileRestoreTool) Name() string       { return "file_restore" }
func (t *FileRestoreTool) Description() string  { return "Restore a file to its previous version (undo last change)" }
func (t *FileRestoreTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "File path to restore"},
		},
		"required": []string{"path"},
	}
}
func (t *FileRestoreTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileRestoreTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	fullPath := expandPath(pathVal)
	content, err := t.History.RestoreLast(fullPath)
	if err != nil {
		return tools.ToolResult{Output: err.Error(), IsError: true}
	}
	preview := content
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return tools.ToolResult{Output: fmt.Sprintf("Restored %s to previous version:\n%s", fullPath, preview)}
}

// ─── file_rewind ───

type FileRewindTool struct {
	History *SnapshotHistory
}

func (t *FileRewindTool) Name() string       { return "file_rewind" }
func (t *FileRewindTool) Description() string  { return "Rewind a file N versions back" }
func (t *FileRewindTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":  map[string]any{"type": "string", "description": "File path to rewind"},
			"steps": map[string]any{"type": "integer", "minimum": 1, "description": "Number of versions to rewind"},
		},
		"required": []string{"path", "steps"},
	}
}
func (t *FileRewindTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileRewindTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	steps := 1
	if v, ok := params["steps"].(float64); ok {
		steps = int(v)
	} else if v, ok := params["steps"].(int); ok {
		steps = v
	}
	if steps < 1 {
		return tools.ToolResult{Output: "Error: steps must be at least 1", IsError: true}
	}
	fullPath := expandPath(pathVal)
	content, err := t.History.RewindSteps(fullPath, steps)
	if err != nil {
		return tools.ToolResult{Output: err.Error(), IsError: true}
	}
	preview := content
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return tools.ToolResult{Output: fmt.Sprintf("Rewound %s %d step(s):\n%s", fullPath, steps, preview)}
}

// ─── file_history_diff ───

type FileHistoryDiffTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryDiffTool) Name() string       { return "file_history_diff" }
func (t *FileHistoryDiffTool) Description() string  { return "Show diff between two versions of a file. Supports unified diff, stat, name-only modes, and chain diff (from→to→to2)" }
func (t *FileHistoryDiffTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":     map[string]any{"type": "string", "description": "File path"},
			"from":     map[string]any{"type": "string", "default": "last1", "description": "From version specifier (v3, current, last2, tag)"},
			"to":       map[string]any{"type": "string", "default": "current", "description": "To version specifier"},
			"to2":      map[string]any{"type": "string", "description": "Optional third version for chain diff (from→to→to2)"},
			"mode":     map[string]any{"type": "string", "enum": []string{"unified", "stat", "name-only"}, "default": "unified", "description": "Diff output mode"},
			"context":  map[string]any{"type": "integer", "minimum": 0, "default": 3, "description": "Context lines (unified mode only)"},
		},
		"required": []string{"path"},
	}
}
func (t *FileHistoryDiffTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryDiffTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	fullPath := expandPath(pathVal)
	snaps := t.History.ListSnapshots(fullPath)
	if len(snaps) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("No history for %s", fullPath), IsError: true}
	}
	fromSpec := "last1"
	if v, ok := params["from"].(string); ok && v != "" {
		fromSpec = v
	} else if v, ok := params["from"].(float64); ok {
		fromSpec = fmt.Sprintf("%d", int(v))
	} else if v, ok := params["from"].(int); ok {
		fromSpec = fmt.Sprintf("%d", v)
	}
	toSpec := "current"
	if v, ok := params["to"].(string); ok && v != "" {
		toSpec = v
	} else if v, ok := params["to"].(float64); ok {
		toSpec = fmt.Sprintf("%d", int(v))
	} else if v, ok := params["to"].(int); ok {
		toSpec = fmt.Sprintf("%d", v)
	}
	fromVer, err := t.History.ResolveVersion(fullPath, fromSpec)
	if err != nil {
		if v, e := strconv.Atoi(fromSpec); e == nil {
			fromVer = v
		} else {
			return tools.ToolResult{Output: fmt.Sprintf("Cannot resolve 'from': %v", err), IsError: true}
		}
	}
	toVer, err := t.History.ResolveVersion(fullPath, toSpec)
	if err != nil {
		if v, e := strconv.Atoi(toSpec); e == nil {
			toVer = v
		} else {
			return tools.ToolResult{Output: fmt.Sprintf("Cannot resolve 'to': %v", err), IsError: true}
		}
	}
	if fromVer < 1 || fromVer > len(snaps) {
		return tools.ToolResult{Output: fmt.Sprintf("From version %d out of range (1-%d)", fromVer, len(snaps)), IsError: true}
	}
	if toVer < 1 || toVer > len(snaps) {
		return tools.ToolResult{Output: fmt.Sprintf("To version %d out of range (1-%d)", toVer, len(snaps)), IsError: true}
	}
	mode := "unified"
	if m, ok := params["mode"].(string); ok && m != "" {
		mode = m
	}
	to2Spec, hasTo2 := params["to2"].(string)
	if hasTo2 && to2Spec != "" {
		return t.chainDiff(fullPath, snaps, fromVer, toVer, to2Spec, mode)
	}
	fromSnap := snaps[fromVer-1]
	toSnap := snaps[toVer-1]
	if fromSnap.Checksum == toSnap.Checksum {
		return tools.ToolResult{Output: "No differences between v" + fmt.Sprintf("%d", fromVer) + " and v" + fmt.Sprintf("%d", toVer)}
	}
	return t.diffOutput(fullPath, fromVer, toVer, fromSnap.Content, toSnap.Content, mode, params)
}

func (t *FileHistoryDiffTool) diffOutput(fullPath string, fromVer, toVer int, fromContent, toContent, mode string, params map[string]any) tools.ToolResult {
	switch mode {
	case "stat":
		fromLines := strings.Split(fromContent, "\n")
		toLines := strings.Split(toContent, "\n")
		added := len(toLines) - len(fromLines)
		if added < 0 {
			return tools.ToolResult{Output: fmt.Sprintf("%s: v%d→v%d | %d lines removed", filepath.Base(fullPath), fromVer, toVer, -added)}
		}
		return tools.ToolResult{Output: fmt.Sprintf("%s: v%d→v%d | %d lines added", filepath.Base(fullPath), fromVer, toVer, added)}
	case "name-only":
		return tools.ToolResult{Output: fmt.Sprintf("%s (v%d → v%d)", filepath.Base(fullPath), fromVer, toVer)}
	default:
		contextLines := 3
		if v, ok := params["context"].(float64); ok {
			contextLines = int(v)
		}
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(fromContent),
			B:        difflib.SplitLines(toContent),
			FromFile: fmt.Sprintf("v%d", fromVer),
			ToFile:   fmt.Sprintf("v%d", toVer),
			Context:  contextLines,
		}
		result, err := difflib.GetUnifiedDiffString(diff)
		if err != nil {
			return tools.ToolResult{Output: fmt.Sprintf("Diff error: %v", err), IsError: true}
		}
		if result == "" {
			return tools.ToolResult{Output: "No differences found"}
		}
		added := 0
		removed := 0
		for _, line := range strings.Split(result, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				added++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				removed++
			}
		}
		return tools.ToolResult{Output: fmt.Sprintf("%s\n+%d -%d lines changed", result, added, removed)}
	}
}

func (t *FileHistoryDiffTool) chainDiff(fullPath string, snaps []FileSnapshot, fromVer, toVer int, to2Spec, mode string) tools.ToolResult {
	to2Ver, err := t.History.ResolveVersion(fullPath, to2Spec)
	if err != nil {
		if v, e := strconv.Atoi(to2Spec); e == nil {
			to2Ver = v
		} else {
			return tools.ToolResult{Output: fmt.Sprintf("Cannot resolve 'to2': %v", err), IsError: true}
		}
	}
	if to2Ver < 1 || to2Ver > len(snaps) {
		return tools.ToolResult{Output: fmt.Sprintf("To2 version %d out of range (1-%d)", to2Ver, len(snaps)), IsError: true}
	}
	if mode == "stat" {
		fromSnap := snaps[fromVer-1]
		toSnap := snaps[toVer-1]
		to2Snap := snaps[to2Ver-1]
		part1 := t.diffOutput(fullPath, fromVer, toVer, fromSnap.Content, toSnap.Content, "stat", nil)
		part2 := t.diffOutput(fullPath, toVer, to2Ver, toSnap.Content, to2Snap.Content, "stat", nil)
		return tools.ToolResult{Output: fmt.Sprintf("Chain diff v%d→v%d→v%d:\n  %s\n  %s", fromVer, toVer, to2Ver, part1.Output, part2.Output)}
	}
	if mode == "name-only" {
		return tools.ToolResult{Output: fmt.Sprintf("%s (v%d → v%d → v%d)", filepath.Base(fullPath), fromVer, toVer, to2Ver)}
	}
	fromSnap := snaps[fromVer-1]
	toSnap := snaps[toVer-1]
	to2Snap := snaps[to2Ver-1]
	part1 := t.diffOutput(fullPath, fromVer, toVer, fromSnap.Content, toSnap.Content, "unified", nil)
	part2 := t.diffOutput(fullPath, toVer, to2Ver, toSnap.Content, to2Snap.Content, "unified", nil)
	return tools.ToolResult{Output: fmt.Sprintf("Chain diff v%d→v%d→v%d:\n\n--- v%d → v%d ---\n%s\n\n--- v%d → v%d ---\n%s", fromVer, toVer, to2Ver, fromVer, toVer, part1.Output, toVer, to2Ver, part2.Output)}
}

// ─── file_history_summary ───

type FileHistorySummaryTool struct {
	History *SnapshotHistory
}

func (t *FileHistorySummaryTool) Name() string       { return "file_history_summary" }
func (t *FileHistorySummaryTool) Description() string  { return "Overview of all files with version history" }
func (t *FileHistorySummaryTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"since": map[string]any{"type": "string", "description": "Duration filter (e.g. '1h', '30m', '2d')"},
		},
	}
}
func (t *FileHistorySummaryTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistorySummaryTool) Execute(params map[string]any) tools.ToolResult {
	var since time.Time
	if s, ok := params["since"].(string); ok && s != "" {
		since = parseDuration(s)
	}
	files := t.History.ListAllFiles()
	if len(files) == 0 {
		return tools.ToolResult{Output: "No files with history found"}
	}
	type fileInfo struct {
		path   string
		count  int
		latest FileSnapshot
	}
	var infos []fileInfo
	for _, fp := range files {
		snaps := t.History.ListSnapshots(fp)
		if len(snaps) == 0 {
			continue
		}
		latest := snaps[len(snaps)-1]
		if !since.IsZero() && latest.Timestamp.Before(since) {
			continue
		}
		infos = append(infos, fileInfo{path: fp, count: len(snaps), latest: latest})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].latest.Timestamp.After(infos[j].latest.Timestamp)
	})
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Files with history (%d total):\n", len(infos)))
	for _, info := range infos {
		desc := info.latest.Description
		deleted := ""
		if info.latest.Deleted {
			deleted = " [DELETED]"
		}
		if desc != "" {
			sb.WriteString(fmt.Sprintf("  %s -- %d versions, latest: %s%s (%s)\n", info.path, info.count, desc, deleted, info.latest.Timestamp.Format("15:04")))
		} else {
			sb.WriteString(fmt.Sprintf("  %s -- %d versions%s (%s)\n", info.path, info.count, deleted, info.latest.Timestamp.Format("15:04")))
		}
	}
	return tools.ToolResult{Output: sb.String()}
}

// ─── file_history_search ───

type FileHistorySearchTool struct {
	History *SnapshotHistory
}

func (t *FileHistorySearchTool) Name() string       { return "file_history_search" }
func (t *FileHistorySearchTool) Description() string  { return "Find versions where text was added, removed, or changed" }
func (t *FileHistorySearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":        map[string]any{"type": "string", "description": "File path"},
			"query":       map[string]any{"type": "string", "description": "Text to search for"},
			"mode":        map[string]any{"type": "string", "enum": []string{"added", "removed", "changed"}, "default": "changed", "description": "Search mode"},
			"ignore_case": map[string]any{"type": "boolean", "default": false, "description": "Case-insensitive"},
		},
		"required": []string{"path", "query"},
	}
}
func (t *FileHistorySearchTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistorySearchTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"query\"", IsError: true}
	}
	mode := SearchChanged
	if m, ok := params["mode"].(string); ok {
		switch m {
		case "added":
			mode = SearchAdded
		case "removed":
			mode = SearchRemoved
		}
	}
	ignoreCase := false
	if v, ok := params["ignore_case"].(bool); ok {
		ignoreCase = v
	}
	fullPath := expandPath(pathVal)
	results := t.History.Search(fullPath, query, mode, ignoreCase)
	if len(results) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("No %s results for %q in %s", params["mode"], query, fullPath)}
	}
	var sb strings.Builder
	modeStr := "changed"
	if m, ok := params["mode"].(string); ok {
		modeStr = m
	}
	sb.WriteString(fmt.Sprintf("Search results (%s: %q in %s):\n", modeStr, query, fullPath))
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("  v%d:\n", r.Version))
		for _, l := range r.Lines {
			if len(l) > 200 {
				l = l[:200]
			}
			sb.WriteString(fmt.Sprintf("    %s\n", l))
		}
	}
	return tools.ToolResult{Output: sb.String()}
}

// ─── file_history_timeline ───

type FileHistoryTimelineTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryTimelineTool) Name() string       { return "file_history_timeline" }
func (t *FileHistoryTimelineTool) Description() string  { return "Chronological cross-file change timeline" }
func (t *FileHistoryTimelineTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"since": map[string]any{"type": "string", "description": "Duration filter (e.g. '1h', '30m')"},
			"limit": map[string]any{"type": "integer", "minimum": 1, "default": 20, "description": "Max entries"},
		},
	}
}
func (t *FileHistoryTimelineTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryTimelineTool) Execute(params map[string]any) tools.ToolResult {
	var since time.Time
	if s, ok := params["since"].(string); ok && s != "" {
		since = parseDuration(s)
	}
	limit := 20
	if v, ok := params["limit"].(float64); ok {
		limit = int(v)
	}
	entries := t.History.GetTimeline(since)
	if len(entries) == 0 {
		return tools.ToolResult{Output: "No timeline entries found"}
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Timeline (%d entries):\n", len(entries)))
	for _, e := range entries {
		desc := e.Description
		if desc == "" {
			desc = "(no description)"
		}
		sb.WriteString(fmt.Sprintf("  %s  %s  v%d  %s\n",
			e.Timestamp.Format("15:04:05"),
			filepath.Base(e.FilePath),
			e.Version,
			desc,
		))
	}
	return tools.ToolResult{Output: sb.String()}
}

// ─── file_history_tag ───

type FileHistoryTagTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryTagTool) Name() string       { return "file_history_tag" }
func (t *FileHistoryTagTool) Description() string  { return "Manage tags on file versions. Actions: add, list, delete, search" }
func (t *FileHistoryTagTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path. Required for add/list/delete."},
			"tag":     map[string]any{"type": "string", "description": "Tag name."},
			"version": map[string]any{"type": "integer", "minimum": 1, "description": "Version number for delete (1-indexed)."},
			"action":  map[string]any{"type": "string", "enum": []string{"add", "list", "delete", "search"}, "description": "Action: add (default if tag given), list, delete, search."},
		},
		"required": []string{},
	}
}
func (t *FileHistoryTagTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryTagTool) Execute(params map[string]any) tools.ToolResult {
	actionVal, _ := params["action"].(string)
	tagVal, hasTag := params["tag"].(string)
	action := actionVal
	if action == "" {
		if hasTag && tagVal != "" {
			action = "add"
		} else {
			action = "list"
		}
	}
	switch action {
	case "search":
		if tagVal == "" {
			return tools.ToolResult{Output: "Error: tag is required for search", IsError: true}
		}
		results := t.History.SearchTagAll(tagVal)
		if len(results) == 0 {
			return tools.ToolResult{Output: fmt.Sprintf("No versions found with tag [%s]", tagVal)}
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Versions with tag [%s] (%d total):\n", tagVal, len(results)))
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("  %s v%d -- %s\n", r.FilePath, r.Version, r.Description))
		}
		return tools.ToolResult{Output: sb.String()}
	case "delete":
		pathVal, ok := params["path"].(string)
		if !ok || pathVal == "" {
			return tools.ToolResult{Output: "Error: path is required for delete", IsError: true}
		}
		if tagVal == "" {
			return tools.ToolResult{Output: "Error: tag is required for delete", IsError: true}
		}
		versionVal := 0
		if v, ok := params["version"].(float64); ok {
			versionVal = int(v)
		} else if v, ok := params["version"].(int); ok {
			versionVal = v
		}
		if versionVal == 0 {
			return tools.ToolResult{Output: "Error: version is required for delete", IsError: true}
		}
		fullPath := expandPath(pathVal)
		if t.History.RemoveTag(fullPath, versionVal, tagVal) {
			return tools.ToolResult{Output: fmt.Sprintf("Tag [%s] removed from %s v%d", tagVal, fullPath, versionVal)}
		}
		return tools.ToolResult{Output: fmt.Sprintf("Tag [%s] not found on %s v%d", tagVal, fullPath, versionVal), IsError: true}
	case "add":
		pathVal, ok := params["path"].(string)
		if !ok || pathVal == "" {
			return tools.ToolResult{Output: "Error: path is required for add", IsError: true}
		}
		if tagVal == "" {
			return tools.ToolResult{Output: "Error: tag is required for add", IsError: true}
		}
		fullPath := expandPath(pathVal)
		if t.History.AddTag(fullPath, tagVal) {
			return tools.ToolResult{Output: fmt.Sprintf("Tag [%s] added to %s", tagVal, fullPath)}
		}
		return tools.ToolResult{Output: fmt.Sprintf("Cannot add tag to %s (no snapshots)", fullPath), IsError: true}
	default:
		pathVal, ok := params["path"].(string)
		if !ok || pathVal == "" {
			return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
		}
		fullPath := expandPath(pathVal)
		snaps := t.History.ListSnapshots(fullPath)
		if len(snaps) == 0 {
			return tools.ToolResult{Output: fmt.Sprintf("No history for %s", fullPath), IsError: true}
		}
		tags := t.History.ListTags(fullPath)
		if len(tags) == 0 {
			return tools.ToolResult{Output: fmt.Sprintf("No tags for %s", fullPath)}
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Tags for %s:\n", fullPath))
		for _, tag := range tags {
			activeVer := 0
			for _, s := range snaps {
				if s.Deleted {
					continue
				}
				activeVer++
				if activeVer == tag.Version {
					desc := s.Description
					if desc == "" {
						desc = fmt.Sprintf("%d bytes", len(s.Content))
					} else {
						desc = fmt.Sprintf("%s (%d bytes)", desc, len(s.Content))
					}
					sb.WriteString(fmt.Sprintf("  v%d: [%s] %s\n", tag.Version, tag.Tag, desc))
					break
				}
			}
		}
		return tools.ToolResult{Output: sb.String()}
	}
}

// ─── file_history_annotate ───

type FileHistoryAnnotateTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryAnnotateTool) Name() string       { return "file_history_annotate" }
func (t *FileHistoryAnnotateTool) Description() string  { return "Add a user annotation/comment to a specific file version" }
func (t *FileHistoryAnnotateTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path"},
			"version": map[string]any{"type": "integer", "minimum": 1, "description": "Version number (1-indexed among active snapshots)"},
			"message": map[string]any{"type": "string", "description": "Annotation text to append to version description"},
		},
		"required": []string{"path", "version", "message"},
	}
}
func (t *FileHistoryAnnotateTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryAnnotateTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	message, ok := params["message"].(string)
	if !ok || message == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"message\"", IsError: true}
	}
	version := 0
	if v, ok := params["version"].(float64); ok {
		version = int(v)
	} else if v, ok := params["version"].(int); ok {
		version = v
	}
	if version < 1 {
		return tools.ToolResult{Output: "Error: version must be at least 1", IsError: true}
	}
	fullPath := expandPath(pathVal)
	if t.History.AnnotateSnapshot(fullPath, version, message) {
		return tools.ToolResult{Output: fmt.Sprintf("Annotation added to %s v%d: %q", fullPath, version, message)}
	}
	return tools.ToolResult{Output: fmt.Sprintf("Cannot annotate %s v%d (version not found)", fullPath, version), IsError: true}
}

// ─── file_history_checkout ───

type FileHistoryCheckoutTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryCheckoutTool) Name() string       { return "file_history_checkout" }
func (t *FileHistoryCheckoutTool) Description() string  { return "Restore a file to a specific version using flexible version specifiers" }
func (t *FileHistoryCheckoutTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path"},
			"version": map[string]any{"type": "string", "default": "last1", "description": "Version specifier (v3, current, last2, tag name)"},
		},
		"required": []string{"path"},
	}
}
func (t *FileHistoryCheckoutTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryCheckoutTool) Execute(params map[string]any) tools.ToolResult {
	pathVal, ok := params["path"].(string)
	if !ok || pathVal == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"path\"", IsError: true}
	}
	fullPath := expandPath(pathVal)
	versionSpec := "last1"
	if v, ok := params["version"].(string); ok && v != "" {
		versionSpec = v
	}
	targetVer, err := t.History.ResolveVersion(fullPath, versionSpec)
	if err != nil {
		return tools.ToolResult{Output: fmt.Sprintf("Cannot resolve version %q: %v", versionSpec, err), IsError: true}
	}
	content, err := t.History.Checkout(fullPath, targetVer)
	if err != nil {
		return tools.ToolResult{Output: err.Error(), IsError: true}
	}
	preview := content
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return tools.ToolResult{Output: fmt.Sprintf("Checked out %s to v%d:\n%s", fullPath, targetVer, preview)}
}

// ─── file_history_batch ───

type FileHistoryBatchTool struct {
	History *SnapshotHistory
}

func (t *FileHistoryBatchTool) Name() string       { return "file_history_batch" }
func (t *FileHistoryBatchTool) Description() string  { return "Batch operations on multiple files matching a glob pattern" }
func (t *FileHistoryBatchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern to match files (e.g. '**/*.go', 'src/*.ts')"},
			"action":  map[string]any{"type": "string", "enum": []string{"history", "diff", "restore", "count"}, "default": "history", "description": "Action to perform on matched files"},
		},
		"required": []string{"pattern"},
	}
}
func (t *FileHistoryBatchTool) CheckPermissions(params map[string]any) string { return "" }

func (t *FileHistoryBatchTool) Execute(params map[string]any) tools.ToolResult {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return tools.ToolResult{Output: "Error: missing required parameter: \"pattern\"", IsError: true}
	}
	action := "history"
	if a, ok := params["action"].(string); ok && a != "" {
		action = a
	}
	files := t.History.ListAllFiles()
	var matched []string
	for _, f := range files {
		matchedPath, _ := filepath.Match(pattern, filepath.Base(f))
		if !matchedPath {
			matchedPath, _ = filepath.Match(pattern, f)
		}
		if !matchedPath {
			matchedPath = globMatch(pattern, f)
		}
		if matchedPath {
			matched = append(matched, f)
		}
	}
	if len(matched) == 0 {
		return tools.ToolResult{Output: fmt.Sprintf("No files with history match pattern %q", pattern)}
	}
	switch action {
	case "count":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Files matching %q (%d total):\n", pattern, len(matched)))
		for _, f := range matched {
			count := t.History.SnapshotCount(f)
			sb.WriteString(fmt.Sprintf("  %s: %d versions\n", filepath.Base(f), count))
		}
		return tools.ToolResult{Output: sb.String()}
	case "restore":
		var sb strings.Builder
		restored := 0
		for _, f := range matched {
			_, err := t.History.RestoreLast(f)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %s: %v\n", filepath.Base(f), err))
			} else {
				sb.WriteString(fmt.Sprintf("  %s: restored\n", filepath.Base(f)))
				restored++
			}
		}
		sb.WriteString(fmt.Sprintf("Restored %d/%d files\n", restored, len(matched)))
		return tools.ToolResult{Output: sb.String()}
	case "diff":
		var sb strings.Builder
		for _, f := range matched {
			snaps := t.History.ListSnapshots(f)
			if len(snaps) < 2 {
				sb.WriteString(fmt.Sprintf("  %s: only %d version(s), skipping\n", filepath.Base(f), len(snaps)))
				continue
			}
			fromSnap := snaps[len(snaps)-2]
			toSnap := snaps[len(snaps)-1]
			if fromSnap.Checksum == toSnap.Checksum {
				sb.WriteString(fmt.Sprintf("  %s: no changes\n", filepath.Base(f)))
				continue
			}
			diff := difflib.UnifiedDiff{
				A:        difflib.SplitLines(fromSnap.Content),
				B:        difflib.SplitLines(toSnap.Content),
				FromFile: "previous",
				ToFile:   "current",
				Context:  3,
			}
			result, err := difflib.GetUnifiedDiffString(diff)
			if err != nil {
				sb.WriteString(fmt.Sprintf("  %s: diff error: %v\n", filepath.Base(f), err))
				continue
			}
			sb.WriteString(fmt.Sprintf("  %s:\n%s\n", filepath.Base(f), result))
		}
		return tools.ToolResult{Output: sb.String()}
	default:
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Files matching %q (%d total):\n", pattern, len(matched)))
		for _, f := range matched {
			snaps := t.History.ListSnapshots(f)
			if len(snaps) == 0 {
				continue
			}
			count := len(snaps)
			latest := snaps[len(snaps)-1].Description
			sb.WriteString(fmt.Sprintf("  %s: %d versions", filepath.Base(f), count))
			if latest != "" {
				sb.WriteString(fmt.Sprintf(", latest: %s", latest))
			}
			sb.WriteString("\n")
		}
		return tools.ToolResult{Output: sb.String()}
	}
}

func globMatch(pattern, path string) bool {
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			suffix = strings.TrimPrefix(suffix, "/")
			suffix = strings.TrimPrefix(suffix, string(filepath.Separator))
			if prefix != "" && !strings.HasPrefix(path, prefix) {
				return false
			}
			if suffix != "" {
				for i := 0; i < len(path); i++ {
					if matched, _ := filepath.Match(suffix, path[i:]); matched {
						return true
					}
				}
				return false
			}
			return true
		}
	}
	matched, _ := filepath.Match(pattern, path)
	return matched
}

// ─── Registration ───

// RegisterFileHistoryTools registers all file history tools.
func RegisterFileHistoryTools(registry *tools.Registry, history *SnapshotHistory) {
	registry.Register(&FileHistoryTool{History: history})
	registry.Register(&FileHistoryReadTool{History: history})
	registry.Register(&FileHistoryGrepTool{History: history})
	registry.Register(&FileRestoreTool{History: history})
	registry.Register(&FileRewindTool{History: history})
	registry.Register(&FileHistoryDiffTool{History: history})
	registry.Register(&FileHistorySummaryTool{History: history})
	registry.Register(&FileHistorySearchTool{History: history})
	registry.Register(&FileHistoryTimelineTool{History: history})
	registry.Register(&FileHistoryTagTool{History: history})
	registry.Register(&FileHistoryAnnotateTool{History: history})
	registry.Register(&FileHistoryCheckoutTool{History: history})
	registry.Register(&FileHistoryBatchTool{History: history})
}

// ─── Helpers ───

func parseDuration(s string) time.Time {
	d := time.Duration(0)
	if parsed, err := time.ParseDuration(s); err == nil {
		d = parsed
	} else {
		if strings.HasSuffix(s, "d") {
			if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil {
				d = time.Duration(n) * 24 * time.Hour
			}
		}
	}
	if d > 0 {
		return time.Now().Add(-d)
	}
	return time.Time{}
}

func getCwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func expandPath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(getCwd(), p)
}
