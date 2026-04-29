package tools

import (
	"context"
	"fmt"
	"strings"
)

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	Output   string
	IsError  bool
	Metadata ToolResultMetadata
}

// ToolResultMetadata holds structured metadata about a tool execution.
type ToolResultMetadata struct {
	ToolName    string
	ExitCode    int
	DurationMs  int64
	OutputLines int
	Truncated   bool
}

// ToCompactSummary returns a one-line summary of the tool result for display.
func (m ToolResultMetadata) ToCompactSummary(output string) string {
	status := "ok"
	if m.ExitCode != 0 {
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