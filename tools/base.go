package tools

import (
	"context"
	"fmt"
)

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	Output  string
	IsError bool
}

// Tool is the interface all tools must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	CheckPermissions(params map[string]any) string
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

// ValidateParams checks that required parameters are present.
func ValidateParams(tool Tool, params map[string]any) error {
	schema := tool.InputSchema()
	required, ok := schema["required"].([]string)
	if !ok {
		return nil
	}
	for _, key := range required {
		if _, exists := params[key]; !exists {
			return fmt.Errorf("missing required parameter: %q", key)
		}
	}
	return nil
}

// Registry collects tool instances and provides lookup + API schema generation.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns the tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// AllTools returns all registered tools.
func (r *Registry) AllTools() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
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