package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"miniclaudecode-go/pkg/core/extensions"
	"miniclaudecode-go/pkg/core/tools/bashtool"
	"miniclaudecode-go/pkg/core/tools/edittool"
	"miniclaudecode-go/pkg/core/tools/filemutationqueue"
	"miniclaudecode-go/pkg/core/tools/findtool"
	"miniclaudecode-go/pkg/core/tools/globtool"
	"miniclaudecode-go/pkg/core/tools/greptool"
	"miniclaudecode-go/pkg/core/tools/lstool"
	"miniclaudecode-go/pkg/core/tools/readtool"
	"miniclaudecode-go/pkg/core/tools/writetool"
)

// ToolHandler is a function that executes a tool with context support.
type ToolHandler func(ctx context.Context, input map[string]interface{}) (string, error)

// ProcessLogger is a callback for logging tool execution events.
type ProcessLogger func(stage string, info map[string]string)

// Registry manages available tools
type Registry struct {
	tools         map[string]*Tool
	logger        ProcessLogger
	mutationQueue *filemutationqueue.FileMutationQueue // serializes concurrent file writes per path
}

// Tool represents a registered tool
type Tool struct {
	Definition  extensions.ToolDefinition
	Handler     ToolHandler
	Operations  map[string]interface{} // pluggable operations
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools:         make(map[string]*Tool),
		mutationQueue: filemutationqueue.NewFileMutationQueue(),
	}
}

// SetLogger sets the process logger for tool execution logging.
func (r *Registry) SetLogger(logger ProcessLogger) {
	r.logger = logger
}

// Register registers a tool with the registry
func (r *Registry) Register(name string, def extensions.ToolDefinition, handler ToolHandler) {
	r.tools[name] = &Tool{
		Definition: def,
		Handler:    handler,
		Operations: make(map[string]interface{}),
	}
}

// Unregister removes a tool
func (r *Registry) Unregister(name string) {
	delete(r.tools, name)
}

// Get returns a tool by name
func (r *Registry) Get(name string) *Tool {
	return r.tools[name]
}

// List returns all registered tools
func (r *Registry) List() []*Tool {
	tools := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetDefinitions returns tool definitions for API
func (r *Registry) GetDefinitions() []extensions.ToolDefinition {
	defs := make([]extensions.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition)
	}
	return defs
}

// Execute runs a tool by name with the given input.
// The ctx parameter is used for cancellation (Ctrl+C) and per-turn timeout.
func (r *Registry) Execute(ctx context.Context, name string, input map[string]interface{}) (string, error) {
	tool := r.Get(name)
	if tool == nil {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	if tool.Handler == nil {
		return "", fmt.Errorf("tool handler not implemented: %s", name)
	}

	// Log tool execution start
	if r.logger != nil {
		info := map[string]string{"tool": name}
		switch name {
		case "bash":
			if cmd, ok := input["command"].(string); ok {
				info["command"] = truncateForLog(cmd, 200)
			}
			if cwd, ok := input["cwd"].(string); ok && cwd != "" {
				info["cwd"] = cwd
			}
		case "read":
			if p, ok := input["path"].(string); ok {
				info["path"] = p
			}
			if offset := intOf(input["offset"]); offset > 0 {
				info["offset"] = fmt.Sprintf("%d", offset)
			}
			if limit := intOf(input["limit"]); limit > 0 {
				info["limit"] = fmt.Sprintf("%d", limit)
			}
		case "write":
			if p, ok := input["path"].(string); ok {
				info["path"] = p
			}
			if c, ok := input["content"].(string); ok {
				info["contentLen"] = fmt.Sprintf("%d", len(c))
			}
		case "edit":
			if p, ok := input["path"].(string); ok {
				info["path"] = p
			}
			if old, ok := input["old_string"].(string); ok && old != "" {
				info["oldLen"] = fmt.Sprintf("%d", len(old))
			}
			if nw, ok := input["new_string"].(string); ok && nw != "" {
				info["newLen"] = fmt.Sprintf("%d", len(nw))
			}
		case "grep":
			if p, ok := input["pattern"].(string); ok {
				info["pattern"] = truncateForLog(p, 100)
			}
			if g, ok := input["glob"].(string); ok && g != "" {
				info["glob"] = g
			}
		case "glob":
			if p, ok := input["pattern"].(string); ok {
				info["pattern"] = p
			}
		case "find":
			if p, ok := input["pattern"].(string); ok {
				info["pattern"] = p
			}
			if d, ok := input["dir"].(string); ok && d != "" {
				info["dir"] = d
			}
		case "ls":
			if p, ok := input["path"].(string); ok && p != "" {
				info["path"] = p
			}
		}
		r.logger("start", info)
	}

	result, err := tool.Handler(ctx, input)

	// Log tool execution end (only on error)
	if r.logger != nil && err != nil {
		info := map[string]string{
			"tool":  name,
			"error": truncateForLog(err.Error(), 200),
		}
		r.logger("error", info)
	}

	return result, err
}

// truncateForLog truncates a string for log display.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ToInfo converts a tool definition to structured info
func ToInfo(def extensions.ToolDefinition) map[string]interface{} {
	return map[string]interface{}{
		"name":         def.Name,
		"description":  def.Description,
		"input_schema": def.InputSchema,
	}
}

// RegisterOperation registers a pluggable operation for a tool
func (t *Tool) RegisterOperation(name string, op interface{}) {
	t.Operations[name] = op
}

// GetOperation returns a registered operation
func (t *Tool) GetOperation(name string) interface{} {
	return t.Operations[name]
}

// getCwd returns the current working directory.
func getCwd() string {
	cwd, _ := os.Getwd()
	return cwd
}

// DefaultTools returns the default built-in tools with handlers wired.
// Aligned to pi's tools/index.ts — each tool uses its dedicated package.
func DefaultTools() *Registry {
	reg := NewRegistry()

	reg.Register("read", extensions.ToolDefinition{
		Name:        "read",
		Description: "Read the contents of a file. Use this to view files before editing or to read configuration files, source code, or documentation.",
		PromptSnippet: "Read file contents",
		PromptGuidelines: []string{"Use read to examine files instead of cat or sed."},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read (relative or absolute)",
				},
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read (alias for path)",
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Line number to start reading from (1-indexed)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of lines to read",
				},
			},
			"required": []string{},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		path := stringOf(input["path"])
		if path == "" {
			path = stringOf(input["file_path"])
		}
		if path == "" {
			return "", fmt.Errorf("missing or invalid 'path' parameter")
		}
		ri := readtool.ReadInput{
			Path:   path,
			Offset: intOf(input["offset"]),
			Limit:  intOf(input["limit"]),
		}
		result, err := readtool.Execute(ri, getCwd(), readtool.LocalReadOperations{}, true, true)
		if err != nil {
			return "", err
		}
		return readtool.FormatReadOutput(result), nil
	})

	reg.Register("write", extensions.ToolDefinition{
		Name:        "write",
		Description: "Write content to a file. This will create the file if it doesn't exist or overwrite if it does. Use for creating new files or making significant changes.",
		PromptSnippet: "Create or overwrite files",
		PromptGuidelines: []string{"Use write only for new files or complete rewrites."},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		path, ok := input["path"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'path' parameter")
		}
		content, ok := input["content"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'content' parameter")
		}
		var result *writetool.WriteResult
		var err error
		err = reg.mutationQueue.WithFileMutationQueue(path, func() error {
			result, err = writetool.Execute(writetool.WriteInput{
				Path:    path,
				Content: content,
			}, getCwd(), writetool.LocalWriteOperations{})
			return err
		})
		if err != nil {
			return "", err
		}
		if result.Created {
			return fmt.Sprintf("Created new file %s with %d bytes", result.Path, result.BytesWritten), nil
		}
		if result.Diff == "" {
			return fmt.Sprintf("No changes to %s (content unchanged)", result.Path), nil
		}
		return fmt.Sprintf("Updated %s (%d bytes):\n%s", result.Path, result.BytesWritten, result.Diff), nil
	})

	reg.Register("edit", extensions.ToolDefinition{
		Name:        "edit",
		Description: "Make precise file edits with exact text replacement. Every edits[].oldText must match a unique, non-overlapping region of the original file. If two changes affect the same block or nearby lines, merge them into one edit instead of emitting overlapping edits.",
		PromptSnippet: "Make precise file edits with exact text replacement, including multiple disjoint edits in one call",
		PromptGuidelines: []string{
			"Use edit for precise changes (edits[].oldText must match exactly)",
			"When changing multiple separate locations in one file, use one edit call with multiple entries in edits[] instead of multiple edit calls",
			"Each edits[].oldText is matched against the original file, not after earlier edits are applied. Do not emit overlapping or nested edits. Merge nearby changes into one edit.",
			"Keep edits[].oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions.",
		},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to edit (relative or absolute)",
				},
				"edits": map[string]interface{}{
					"type":        "array",
					"description": "One or more targeted replacements. Each edit is matched against the original file, not incrementally. Do not include overlapping or nested edits.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"oldText": map[string]interface{}{
								"type":        "string",
								"description": "Exact text for one targeted replacement. Must be unique in the original file and not overlap with other edits[].oldText.",
							},
							"newText": map[string]interface{}{
								"type":        "string",
								"description": "Replacement text for this targeted edit.",
							},
						},
						"required": []string{"oldText", "newText"},
					},
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Edit mode: replace, insert, or delete (legacy)",
				},
				"old_string": map[string]interface{}{
					"type":        "string",
					"description": "Text to find and replace (legacy, for replace/delete mode)",
				},
				"new_string": map[string]interface{}{
					"type":        "string",
					"description": "Text to insert (legacy, for replace/insert mode)",
				},
				"insert_line": map[string]interface{}{
					"type":        "integer",
					"description": "Line number to insert at (legacy)",
				},
				"start_line": map[string]interface{}{
					"type":        "integer",
					"description": "Start line for range edit (legacy)",
				},
				"end_line": map[string]interface{}{
					"type":        "integer",
					"description": "End line for range edit (legacy)",
				},
			},
			"required": []string{"path"},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		// Parse edits: support edits[] array and legacy oldText/newText top-level fields
		var editEntries []edittool.EditEntry
		if editsRaw, ok := input["edits"]; ok {
			if editsArr, ok := editsRaw.([]interface{}); ok {
				for _, v := range editsArr {
					if m, ok := v.(map[string]interface{}); ok {
						oldT := stringOf(m["oldText"])
						if oldT == "" {
							oldT = stringOf(m["old_string"])
						}
						editEntries = append(editEntries, edittool.EditEntry{
							OldText: oldT,
							NewText: stringOf(m["newText"]),
						})
					}
				}
			}
		}
		// Legacy compat: if LLM sends oldText/newText as top-level fields (TS prepareEditArguments)
		if len(editEntries) == 0 {
			oldT := stringOf(input["oldText"])
			newT := stringOf(input["newText"])
			if oldT != "" || newT != "" {
				editEntries = append(editEntries, edittool.EditEntry{
					OldText: oldT,
					NewText: newT,
				})
			}
		}

		editType := edittool.EditReplace
		if t, ok := input["type"].(string); ok {
			editType = edittool.EditType(t)
		}

		filePath := stringOf(input["path"])
		var result *edittool.EditResult
		var err error
		err = reg.mutationQueue.WithFileMutationQueue(filePath, func() error {
			result, err = edittool.Execute(edittool.EditInput{
				Edits:      editEntries,
				Type:       editType,
				Path:       filePath,
				OldString:  stringOf(input["old_string"]),
				NewString:  stringOf(input["new_string"]),
				InsertLine: intOf(input["insert_line"]),
				StartLine:  intOf(input["start_line"]),
				EndLine:    intOf(input["end_line"]),
			}, getCwd(), edittool.LocalEditOperations{})
			return err
		})
		if err != nil {
			return "", err
		}
		return edittool.FormatEditOutput(result), nil
	})

	reg.Register("bash", extensions.ToolDefinition{
		Name:        "bash",
		Description: "Execute a shell command. Use for running git commands, npm scripts, running tests, or any command-line operations.",
		PromptSnippet: "Execute bash commands (ls, grep, find, etc.)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
				"cwd": map[string]interface{}{
					"type":        "string",
					"description": "Optional working directory for the command",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in seconds (default: 120)",
				},
			},
			"required": []string{"command"},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		cmd, ok := input["command"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'command' parameter")
		}
		ri := bashtool.BashInput{
			Command: cmd,
			CWD:     stringOf(input["cwd"]),
			Timeout: intOf(input["timeout"]),
		}
		if ri.CWD == "" {
			ri.CWD = getCwd()
		}
		result, err := bashtool.Execute(ctx, ri, bashtool.LocalBashOperations{})
		if err != nil {
			return "", err
		}
		return bashtool.FormatBashOutput(result), nil
	})

	reg.Register("grep", extensions.ToolDefinition{
		Name:        "grep",
		Description: "Search for a pattern in files using ripgrep. Use to find function definitions, TODO comments, or any text pattern across your codebase.",
		PromptSnippet: "Search file contents for patterns (respects .gitignore)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regular expression pattern to search for",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File path or directory to search in (alias for paths[0])",
				},
				"paths": map[string]interface{}{
					"type":        "array",
					"description": "File paths or directories to search in",
					"items":       map[string]interface{}{"type": "string"},
				},
				"ignoreCase": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the search should be case insensitive (alias for !case_sensitive)",
				},
				"case_sensitive": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the search should be case sensitive",
				},
				"literal": map[string]interface{}{
					"type":        "boolean",
					"description": "Treat pattern as a literal string instead of regex",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of matches to return (alias for max_results)",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of matches to return",
				},
				"context": map[string]interface{}{
					"type":        "integer",
					"description": "Number of context lines before and after matches",
				},
				"glob": map[string]interface{}{
					"type":        "string",
					"description": "Glob filter for file names (e.g., *.go)",
				},
			},
			"required": []string{"pattern"},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		pattern, ok := input["pattern"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'pattern' parameter")
		}
		var paths []string
		// TS alias: "path" (string) → paths[0]
		if p, ok := input["path"].(string); ok && p != "" {
			paths = append(paths, p)
		}
		if p, ok := input["paths"].([]interface{}); ok {
			for _, v := range p {
				if s, ok := v.(string); ok {
					paths = append(paths, s)
				}
			}
		}
		// TS alias: "ignoreCase" → !CaseSensitive
		caseSensitive := boolOf(input["case_sensitive"])
		if ignoreCase, ok := input["ignoreCase"].(bool); ok {
			caseSensitive = !ignoreCase
		}
		// TS alias: "limit" → MaxResults
		maxResults := intOf(input["max_results"])
		if limit := intOf(input["limit"]); limit > 0 {
			maxResults = limit
		}
		ri := greptool.GrepInput{
			Pattern:       pattern,
			Paths:         paths,
			MaxResults:    maxResults,
			CaseSensitive: caseSensitive,
			Literal:       boolOf(input["literal"]),
			ContextLines:  intOf(input["context"]),
			Glob:          stringOf(input["glob"]),
		}
		gctx, cancel := context.WithTimeout(ctx, greptool.GrepTimeout)
		defer cancel()
		result, err := greptool.ExecuteWithFallback(gctx, ri, getCwd())
		if err != nil {
			return "", err
		}
		return greptool.FormatGrepOutput(result), nil
	})

	reg.Register("find", extensions.ToolDefinition{
		Name:        "find",
		Description: "Find files matching a pattern using fd (or Go fallback). Use to locate files by name pattern.",
		PromptSnippet: "Find files by glob pattern (respects .gitignore)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Directory to search in (alias for dir)",
				},
				"dir": map[string]interface{}{
					"type":        "string",
					"description": "Directory to search in",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regex or glob pattern for file names",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of results to return (alias for max_results, default: 1000)",
				},
				"max_depth": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum directory depth to search",
				},
			},
			"required": []string{"pattern"},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		dir := stringOf(input["dir"])
		if dir == "" {
			dir = stringOf(input["path"])
		}
		if dir == "" {
			dir = getCwd()
		}
		// TS default: limit=1000
		limit := intOf(input["limit"])
		if limit == 0 {
			limit = 1000
		}
		ri := findtool.FindInput{
			Dir:      dir,
			Pattern:  stringOf(input["pattern"]),
			MaxDepth: intOf(input["max_depth"]),
		}
		fctx, cancel := context.WithTimeout(ctx, findtool.FindTimeout)
		defer cancel()
		result, err := findtool.Execute(fctx, ri, dir, findtool.LocalFindOperations{})
		if err != nil {
			return "", err
		}
		// Apply limit to results (TS alignment: default limit=1000)
		if len(result.Paths) > limit {
			result.Paths = result.Paths[:limit]
			result.TotalCount = limit
			result.Truncated = true
		}
		return findtool.FormatFindOutput(result), nil
	})

	reg.Register("glob", extensions.ToolDefinition{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Use for quick file searches using wildcards.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Glob pattern (e.g., *.go, **/*.ts)",
				},
				"cwd": map[string]interface{}{
					"type":        "string",
					"description": "Working directory for the search",
				},
			},
			"required": []string{"pattern"},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		pattern, ok := input["pattern"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'pattern' parameter")
		}
		cwd := stringOf(input["cwd"])
		if cwd == "" {
			cwd = getCwd()
		}
		result, err := globtool.Execute(globtool.GlobInput{
			Pattern: pattern,
			Cwd:     cwd,
		}, cwd, globtool.LocalGlobOperations{})
		if err != nil {
			return "", err
		}
		if len(result.Matches) == 0 {
			return "No files found", nil
		}
		return globtool.FormatGlobOutput(result), nil
	})

	reg.Register("ls", extensions.ToolDefinition{
		Name:        "ls",
		Description: "List directory contents. Returns entries sorted alphabetically, with '/' suffix for directories. Includes dotfiles.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Directory path to list (default: current directory)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of entries to return (default: 500)",
				},
			},
			"required": []string{},
		},
	}, func(ctx context.Context, input map[string]interface{}) (string, error) {
		path := stringOf(input["path"])
		if path == "" {
			path = "."
		}
		result, err := lstool.Execute(lstool.LsInput{
			Path:  path,
			Limit: intOf(input["limit"]),
		}, getCwd(), lstool.LocalLsOperations{})
		if err != nil {
			return "", err
		}
		return result.Output, nil
	})

	return reg
}

// Helper functions
func stringOf(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intOf(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func boolOf(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// ParseToolCalls parses tool use blocks from an LLM response (JSON format)
func ParseToolCalls(response string) ([]map[string]interface{}, error) {
	var toolCalls []map[string]interface{}

	start := -1
	depth := 0
	inString := false
	escape := false

	for i, c := range response {
		if escape {
			escape = false
			continue
		}
		switch c {
		case '\\':
			escape = true
		case '"':
			inString = !inString
		case '{', '[':
			if !inString {
				if depth == 0 {
					start = i
				}
				depth++
			}
		case '}', ']':
			if !inString {
				depth--
				if depth == 0 && start >= 0 {
					jsonStr := response[start : i+1]
					if err := json.Unmarshal([]byte(jsonStr), &toolCalls); err == nil {
						return toolCalls, nil
					}
					start = -1
				}
			}
		}
	}

	if err := json.Unmarshal([]byte(response), &toolCalls); err == nil {
		return toolCalls, nil
	}

	return nil, fmt.Errorf("no tool calls found in response")
}

// BuildToolResult creates a tool result message
func BuildToolResult(toolCallId, content string, isError bool) map[string]interface{} {
	return map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": toolCallId,
		"content":     content,
		"is_error":    isError,
	}
}
