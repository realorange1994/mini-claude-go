package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"miniclaudecode-go/pkg/core/extensions"
	"miniclaudecode-go/pkg/core/tools/bashtool"
	"miniclaudecode-go/pkg/core/tools/builtin"
	"miniclaudecode-go/pkg/core/tools/findtool"
	"miniclaudecode-go/pkg/core/tools/greptool"
	"miniclaudecode-go/pkg/core/tools/readtool"
	"miniclaudecode-go/pkg/core/tools/writetool"
)

// ToolHandler is a function that executes a tool
type ToolHandler func(input map[string]interface{}) (string, error)

// Registry manages available tools
type Registry struct {
	tools map[string]*Tool
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
		tools: make(map[string]*Tool),
	}
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

// Execute runs a tool by name with the given input
func (r *Registry) Execute(name string, input map[string]interface{}) (string, error) {
	tool := r.Get(name)
	if tool == nil {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	if tool.Handler == nil {
		return "", fmt.Errorf("tool handler not implemented: %s", name)
	}
	return tool.Handler(input)
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
// Uses the new aligned implementations for Read, Write, Grep, Find, and Bash.
func DefaultTools() *Registry {
	reg := NewRegistry()

	// Read tool handler — uses new readtool package
	reg.Register("Read", extensions.ToolDefinition{
		Name:        "Read",
		Description: "Read the contents of a file. Use this to view files before editing or to read configuration files, source code, or documentation.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read (relative or absolute)",
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
			"required": []string{"path"},
		},
	}, func(input map[string]interface{}) (string, error) {
		path, ok := input["path"].(string)
		if !ok {
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

	// Write tool handler — uses new writetool package
	reg.Register("Write", extensions.ToolDefinition{
		Name:        "Write",
		Description: "Write content to a file. This will create the file if it doesn't exist or overwrite if it does. Use for creating new files or making significant changes.",
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
	}, func(input map[string]interface{}) (string, error) {
		path, ok := input["path"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'path' parameter")
		}
		content, ok := input["content"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'content' parameter")
		}

		result, err := writetool.Execute(writetool.WriteInput{
			Path:    path,
			Content: content,
		}, getCwd(), writetool.LocalWriteOperations{})
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

	// Edit tool handler — uses builtin + editdiff
	reg.Register("Edit", extensions.ToolDefinition{
		Name:        "Edit",
		Description: "Make targeted edits to a file. Supports three modes: replace (replace old_string with new_string), insert (insert new_string at line), and delete (remove content).",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to edit",
				},
				"old_string": map[string]interface{}{
					"type":        "string",
					"description": "Text to find and replace (for replace/delete mode)",
				},
				"new_string": map[string]interface{}{
					"type":        "string",
					"description": "Text to insert (for replace/insert mode)",
				},
				"insert_line": map[string]interface{}{
					"type":        "integer",
					"description": "Line number to insert at (for insert mode)",
				},
				"start_line": map[string]interface{}{
					"type":        "integer",
					"description": "Start line for range edit",
				},
				"end_line": map[string]interface{}{
					"type":        "integer",
					"description": "End line for range edit",
				},
			},
			"required": []string{"path"},
		},
	}, func(input map[string]interface{}) (string, error) {
		path, ok := input["path"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'path' parameter")
		}
		editType := "replace"
		if t, ok := input["type"].(string); ok {
			editType = t
		}

		spec := builtin.EditSpec{
			Type:       editType,
			Path:       path,
			OldString:  stringOf(input["old_string"]),
			NewString:  stringOf(input["new_string"]),
			InsertLine: intOf(input["insert_line"]),
			StartLine:  intOf(input["start_line"]),
			EndLine:    intOf(input["end_line"]),
		}
		return builtin.Edit(spec)
	})

	// Bash tool handler — uses new bashtool package
	reg.Register("Bash", extensions.ToolDefinition{
		Name:        "Bash",
		Description: "Execute a shell command. Use for running git commands, npm scripts, running tests, or any command-line operations.",
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
	}, func(input map[string]interface{}) (string, error) {
		cmd, ok := input["command"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'command' parameter")
		}

		ctx := context.Background()
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

	// Grep tool handler — uses new greptool package with ripgrep
	reg.Register("Grep", extensions.ToolDefinition{
		Name:        "Grep",
		Description: "Search for a pattern in files using ripgrep. Use to find function definitions, TODO comments, or any text pattern across your codebase.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regular expression pattern to search for",
				},
				"paths": map[string]interface{}{
					"type":        "array",
					"description": "File paths or directories to search in",
					"items":       map[string]interface{}{"type": "string"},
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of matches to return",
				},
				"case_sensitive": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the search should be case sensitive",
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
	}, func(input map[string]interface{}) (string, error) {
		pattern, ok := input["pattern"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'pattern' parameter")
		}

		var paths []string
		if p, ok := input["paths"].([]interface{}); ok {
			for _, v := range p {
				if s, ok := v.(string); ok {
					paths = append(paths, s)
				}
			}
		}

		ri := greptool.GrepInput{
			Pattern:       pattern,
			Paths:         paths,
			MaxResults:    intOf(input["max_results"]),
			CaseSensitive: boolOf(input["case_sensitive"]),
			ContextLines:  intOf(input["context"]),
			Glob:          stringOf(input["glob"]),
		}

		ctx, cancel := context.WithTimeout(context.Background(), greptool.GrepTimeout)
		defer cancel()

		result, err := greptool.ExecuteWithFallback(ctx, ri, getCwd())
		if err != nil {
			return "", err
		}

		return greptool.FormatGrepOutput(result), nil
	})

	// Find tool handler — uses new findtool package
	reg.Register("Find", extensions.ToolDefinition{
		Name:        "Find",
		Description: "Find files matching a pattern using fd (or Go fallback). Use to locate files by name pattern.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"dir": map[string]interface{}{
					"type":        "string",
					"description": "Directory to search in",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regex or glob pattern for file names",
				},
				"max_depth": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum directory depth to search",
				},
			},
			"required": []string{"pattern"},
		},
	}, func(input map[string]interface{}) (string, error) {
		dir := stringOf(input["dir"])
		if dir == "" {
			dir = getCwd()
		}

		ri := findtool.FindInput{
			Dir:      dir,
			Pattern:  stringOf(input["pattern"]),
			MaxDepth: intOf(input["max_depth"]),
		}

		ctx, cancel := context.WithTimeout(context.Background(), findtool.FindTimeout)
		defer cancel()

		result, err := findtool.Execute(ctx, ri, dir, findtool.LocalFindOperations{})
		if err != nil {
			return "", err
		}

		return findtool.FormatFindOutput(result), nil
	})

	// Glob tool handler
	reg.Register("Glob", extensions.ToolDefinition{
		Name:        "Glob",
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
	}, func(input map[string]interface{}) (string, error) {
		pattern, ok := input["pattern"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'pattern' parameter")
		}
		cwd := stringOf(input["cwd"])
		if cwd == "" {
			cwd = getCwd()
		}

		results, err := builtin.Glob(cwd, pattern)
		if err != nil {
			return "", err
		}

		if len(results) == 0 {
			return "No files found", nil
		}
		return fmt.Sprintf("Found %d files:\n%s", len(results), joinLines(results)), nil
	})

	// Ls tool handler
	reg.Register("Ls", extensions.ToolDefinition{
		Name:        "Ls",
		Description: "List files and directories in a path. Use to explore directory structure.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Directory path to list",
				},
			},
			"required": []string{"path"},
		},
	}, func(input map[string]interface{}) (string, error) {
		path := stringOf(input["path"])
		if path == "" {
			path = "."
		}

		files, err := builtin.Ls(path)
		if err != nil {
			return "", err
		}

		if len(files) == 0 {
			return "Empty directory", nil
		}

		result := fmt.Sprintf("Total %d items:\n", len(files))
		for _, f := range files {
			icon := "[file]"
			if f.IsDir {
				icon = "[dir]"
			}
			result += fmt.Sprintf("%s %s\n", icon, f.Name)
		}
		return result, nil
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

func joinLines(items []string) string {
	result := ""
	for i, item := range items {
		if i > 0 {
			result += "\n"
		}
		result += item
	}
	return result
}

// ParseToolCalls parses tool use blocks from an LLM response (JSON format)
// Returns list of tool calls: []map[string]interface{}{"id": "...", "name": "...", "input": {...}}
func ParseToolCalls(response string) ([]map[string]interface{}, error) {
	var toolCalls []map[string]interface{}

	// Simple JSON detection - look for array structure
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

	// Fallback: try to parse the whole response if it looks like JSON
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
