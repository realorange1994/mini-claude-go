package tools

import (
	"fmt"
	"strings"
)

// MemoryAddCallback is the function signature for adding a memory note.
type MemoryAddCallback func(category, content, source string)

// MemorySearchCallback is the function signature for searching memory notes.
type MemorySearchCallback func(query string) []MemorySearchResult

// MemorySearchResult represents a single search result from session memory.
type MemorySearchResult struct {
	Category string
	Content  string
}

// MemoryAddTool saves a note to session memory.
type MemoryAddTool struct {
	OnAdd MemoryAddCallback
}

func (t *MemoryAddTool) Name() string        { return "memory_add" }
func (t *MemoryAddTool) Description() string  { return "Save a note to session memory for later reference. Use categories: 'preference' (user preferences), 'decision' (key decisions), 'state' (project state), 'reference' (useful references)." }

func (t *MemoryAddTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"category", "content"},
		"properties": map[string]any{
			"category": map[string]any{
				"type":        "string",
				"enum":        []string{"preference", "decision", "state", "reference"},
				"description": "Category of the memory note",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The note content to remember",
			},
		},
	}
}

func (t *MemoryAddTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *MemoryAddTool) Execute(params map[string]any) ToolResult {
	category, _ := params["category"].(string)
	content, _ := params["content"].(string)
	if category == "" || content == "" {
		return ToolResultError("category and content are required")
	}
	if t.OnAdd == nil {
		return ToolResultError("memory system not initialized")
	}
	t.OnAdd(category, content, "assistant")
	return ToolResultOK(fmt.Sprintf("Saved to memory [%s]: %s", category, content))
}

// MemorySearchTool searches session memory for relevant notes.
type MemorySearchTool struct {
	OnSearch MemorySearchCallback
}

func (t *MemorySearchTool) Name() string        { return "memory_search" }
func (t *MemorySearchTool) Description() string  { return "Search session memory for notes matching a query. Returns relevant memory entries." }

func (t *MemorySearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query to find relevant memory notes",
			},
		},
	}
}

func (t *MemorySearchTool) CheckPermissions(params map[string]any) PermissionResult { return PermissionResultPassthrough() }

func (t *MemorySearchTool) Execute(params map[string]any) ToolResult {
	query, _ := params["query"].(string)
	if query == "" {
		return ToolResultError("query is required")
	}
	if t.OnSearch == nil {
		return ToolResultError("memory system not initialized")
	}
	results := t.OnSearch(query)
	if len(results) == 0 {
		return ToolResultOK("No matching memory notes found.")
	}
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("[%s] %s\n", r.Category, r.Content))
	}
	return ToolResultOK(sb.String())
}
