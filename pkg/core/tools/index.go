package tools

import (
	"encoding/json"
	"fmt"

	"miniclaudecode-go/pkg/core/extensions"
	"miniclaudecode-go/pkg/core/tools/builtin"
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

// DefaultTools returns the default built-in tools with handlers wired
func DefaultTools() *Registry {
	reg := NewRegistry()

	// Read tool handler
	reg.Register("Read", extensions.ToolDefinition{
		Name:        "Read",
		Description: "Read the contents of a file. Use this to view files before editing or to read configuration files, source code, or documentation.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The path to the file to read",
				},
				"lineRange": map[string]interface{}{
					"type":        "array",
					"description": "Optional line range to read [start, end] (0-indexed)",
					"items":       map[string]interface{}{"type": "integer"},
				},
			},
			"required": []string{"path"},
		},
	}, func(input map[string]interface{}) (string, error) {
		path, ok := input["path"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'path' parameter")
		}
		var lineRange *[2]int
		if lr, ok := input["lineRange"].([]interface{}); ok && len(lr) == 2 {
			start := intOf(lr[0])
			end := intOf(lr[1])
			lineRange = &[2]int{start, end}
		}
		result, err := builtin.Read(path, lineRange)
		if err != nil {
			return "", err
		}
		if result.Truncated() {
			return result.Success() + "\n[truncated]", nil
		}
		return result.Success(), nil
	})

	// Write tool handler
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
		if err := builtin.Write(path, content); err != nil {
			return "", err
		}
		return fmt.Sprintf("Wrote %d bytes to %s", len(content), path), nil
	})

	// Edit tool handler
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

	// Bash tool handler
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
			},
			"required": []string{"command"},
		},
	}, func(input map[string]interface{}) (string, error) {
		cmd, ok := input["command"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'command' parameter")
		}
		cwd := stringOf(input["cwd"])
		timeout := 300 // default 5 minutes
		if t, ok := input["timeout"].(float64); ok {
			timeout = int(t)
		}
		return builtin.Bash(cmd, cwd, timeout)
	})

	// Grep tool handler
	reg.Register("Grep", extensions.ToolDefinition{
		Name:        "Grep",
		Description: "Search for a pattern in files. Use to find function definitions, TODO comments, or any text pattern across your codebase.",
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
		if len(paths) == 0 {
			paths = []string{"."}
		}

		maxResults := 100
		if m, ok := input["max_results"].(float64); ok {
			maxResults = int(m)
		}

		caseSensitive := false
		if c, ok := input["case_sensitive"].(bool); ok {
			caseSensitive = c
		}

		opts := builtin.GrepOptions{
			MaxResults:    maxResults,
			CaseSensitive: caseSensitive,
		}

		matches, err := builtin.Grep(pattern, paths, opts)
		if err != nil {
			return "", err
		}

		if len(matches) == 0 {
			return "No matches found", nil
		}

		result := fmt.Sprintf("Found %d matches:\n", len(matches))
		for _, m := range matches {
			result += fmt.Sprintf("%s:%d: %s\n", m.Path(), m.LineNum(), m.Content())
		}
		return result, nil
	})

	// Find tool handler
	reg.Register("Find", extensions.ToolDefinition{
		Name:        "Find",
		Description: "Find files matching a pattern. Use to locate files by name pattern.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"dir": map[string]interface{}{
					"type":        "string",
					"description": "Directory to search in",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regular expression pattern for file names",
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
			dir = "."
		}
		pattern, ok := input["pattern"].(string)
		if !ok {
			return "", fmt.Errorf("missing or invalid 'pattern' parameter")
		}
		maxDepth := 10
		if d, ok := input["max_depth"].(float64); ok {
			maxDepth = int(d)
		}

		results, err := builtin.Find(dir, pattern, maxDepth)
		if err != nil {
			return "", err
		}

		if len(results) == 0 {
			return "No files found", nil
		}
		return fmt.Sprintf("Found %d files:\n%s", len(results), joinLines(results)), nil
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
			cwd = "."
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
			icon := "📄"
			if f.IsDir {
				icon = "📁"
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
	// Try to extract JSON array of tool calls from response
	// This is a simple implementation - real impl would use proper JSON parsing

	// Look for tool_use blocks in the response
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
		"type": "tool_result",
		"tool_use_id": toolCallId,
		"content":     content,
		"is_error":    isError,
	}
}
