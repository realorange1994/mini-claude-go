package tools

import (
	"fmt"
	"sort"
	"strings"
)

// ToolSearchTool allows the model to search for available tools by name or keyword.
type ToolSearchTool struct {
	// Registry is set by the agent loop to provide access to all tools.
	// NOTE: Set via agent_loop after registry is fully populated, since
	// DefaultRegistry() registers this tool before the registry instance exists.
	Registry *Registry
}

func (t *ToolSearchTool) Name() string { return "tool_search" }
func (t *ToolSearchTool) Description() string {
	return `Search for available tools by name or keyword. Use this when you need to find a tool but don't know its exact name. Supports three query forms:
- "select:tool1,tool2" — fetch specific tools by exact name
- "keyword1 keyword2" — keyword search, returns best matches
- "+prefix keyword" — require "prefix" in tool name, rank by remaining terms

Returns tool names, descriptions, and input schemas for matched tools.`
}

func (t *ToolSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query. Forms: 'select:name1,name2' for exact names, 'keyword1 keyword2' for search, '+prefix keyword' for name-prefix required search.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default 10, max 20).",
			},
		},
	}
}

func (t *ToolSearchTool) CheckPermissions(params map[string]any) string { return "" }

func (t *ToolSearchTool) Execute(params map[string]any) ToolResult {
	if t.Registry == nil {
		return ToolResult{Output: "Error: tool registry not available", IsError: true}
	}

	query, _ := params["query"].(string)
	if query == "" {
		return ToolResult{Output: "Error: query is required", IsError: true}
	}

	maxResults := 10
	if mr, ok := params["max_results"]; ok {
		switch v := mr.(type) {
		case float64:
			maxResults = int(v)
		case int:
			maxResults = v
		}
	}
	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 20 {
		maxResults = 20
	}

	// Handle "select:" form — exact name lookup
	if strings.HasPrefix(query, "select:") {
		names := strings.Split(strings.TrimPrefix(query, "select:"), ",")
		return t.selectTools(names)
	}

	// Handle "+" prefix form — require prefix in name
	requirePrefix := ""
	searchTerms := query
	if strings.HasPrefix(query, "+") {
		parts := strings.Fields(query[1:])
		if len(parts) > 0 {
			requirePrefix = strings.ToLower(parts[0])
			searchTerms = strings.Join(parts[1:], " ")
		}
	}

	return t.searchTools(searchTerms, requirePrefix, maxResults)
}

// selectTools returns full definitions for specific tool names
func (t *ToolSearchTool) selectTools(names []string) ToolResult {
	var results []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tool, found := t.Registry.Get(name)
		if !found {
			results = append(results, fmt.Sprintf("Tool '%s' not found", name))
			continue
		}
		results = append(results, formatToolDefinition(tool))
	}
	if len(results) == 0 {
		return ToolResult{Output: "No tools found for the given names"}
	}
	return ToolResult{Output: strings.Join(results, "\n\n")}
}

// searchTools performs keyword search across tool names and descriptions
func (t *ToolSearchTool) searchTools(query, requirePrefix string, maxResults int) ToolResult {
	terms := strings.Fields(strings.ToLower(query))
	allTools := t.Registry.AllTools()

	type scoredTool struct {
		tool  Tool
		score float64
	}

	var candidates []scoredTool
	for _, tool := range allTools {
		name := strings.ToLower(tool.Name())
		desc := strings.ToLower(tool.Description())

		// Skip self
		if tool.Name() == "tool_search" {
			continue
		}

		// Check prefix requirement
		if requirePrefix != "" && !strings.Contains(name, requirePrefix) {
			continue
		}

		// Calculate relevance score
		var score float64
		if len(terms) == 0 {
			// No search terms: include all prefix-matching tools with score 1
			score = 1.0
		} else {
			for _, term := range terms {
				if strings.Contains(name, term) {
					score += 3.0 // Name match is worth more
				}
				if strings.Contains(desc, term) {
					score += 1.0 // Description match
				}
			}
		}

		if score > 0 {
			candidates = append(candidates, scoredTool{tool: tool, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Limit results
	if len(candidates) > maxResults {
		candidates = candidates[:maxResults]
	}

	if len(candidates) == 0 {
		return ToolResult{Output: "No tools matched your search. Try broader terms or use 'select:name' for exact lookup."}
	}

	var results []string
	for _, c := range candidates {
		results = append(results, formatToolDefinition(c.tool))
	}
	return ToolResult{Output: strings.Join(results, "\n\n")}
}

// formatToolDefinition returns a formatted tool definition string
func formatToolDefinition(tool Tool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s\n", tool.Name()))
	sb.WriteString(tool.Description())
	sb.WriteString("\n")

	schema := tool.InputSchema()
	if schema != nil {
		sb.WriteString("Parameters: ")
		if props, ok := schema["properties"].(map[string]any); ok {
			var names []string
			for k := range props {
				names = append(names, k)
			}
			sort.Strings(names)
			sb.WriteString(strings.Join(names, ", "))
		}
	}

	return sb.String()
}
