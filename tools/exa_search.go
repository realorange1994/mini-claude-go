package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const exaMCPURL = "https://mcp.exa.ai/mcp"

// ExaSearchTool searches the web using Exa AI via MCP protocol.
type ExaSearchTool struct{}

func (*ExaSearchTool) Name() string { return "web_search" }

func (*ExaSearchTool) Description() string {
	return "Search the web using Exa AI. Returns relevant content with titles, URLs, and text snippets."
}

func (*ExaSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query string",
			},
			"num_results": map[string]any{
				"type":        "integer",
				"description": "Number of results to return (default: 10)",
			},
			"livecrawl": map[string]any{
				"type":        "string",
				"description": "Crawl mode: 'fallback' or 'preferred'. Default: 'fallback'.",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "Search type: 'auto', 'fast', or 'deep'. Default: 'auto'.",
			},
		},
		"required": []string{"query"},
	}
}

func (*ExaSearchTool) CheckPermissions(params map[string]any) string {
	query, _ := params["query"].(string)
	if containsExaInternalURL(query) {
		return "Search blocked: internal URL detected in query"
	}
	return ""
}

func (*ExaSearchTool) Execute(params map[string]any) ToolResult {
	query, _ := params["query"].(string)
	if query == "" {
		return ToolResult{Output: "Error: query is required", IsError: true}
	}

	numResults := 8
	if n, ok := params["num_results"]; ok {
		switch v := n.(type) {
		case float64:
			numResults = int(v)
		case int:
			numResults = v
		}
	}
	if numResults < 1 {
		numResults = 1
	}
	if numResults > 10 {
		numResults = 10
	}

	liveCrawl := "fallback"
	if lc, ok := params["livecrawl"].(string); ok {
		liveCrawl = lc
	}

	searchType := "auto"
	if st, ok := params["type"].(string); ok {
		searchType = st
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := callExaMCP(ctx, "web_search_exa", map[string]any{
		"query":              query,
		"type":               searchType,
		"numResults":         numResults,
		"livecrawl":          liveCrawl,
		"contextMaxCharacters": 10000,
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Search error: %v", err), IsError: true}
	}

	if result == "" {
		return ToolResult{Output: fmt.Sprintf("No results found for: %s", query)}
	}

	return ToolResult{Output: result}
}

// ExaGetContentsTool fetches detailed content from specific URLs using Exa AI.
type ExaGetContentsTool struct{}

func (*ExaGetContentsTool) Name() string { return "web_fetch" }

func (*ExaGetContentsTool) Description() string {
	return "Fetch and extract content from specific URLs using Exa AI. Returns clean, readable text."
}

func (*ExaGetContentsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to fetch content from",
			},
			"extract_mode": map[string]any{
				"type":        "string",
				"description": "Extraction mode: 'markdown' or 'text'. Default: 'markdown'.",
			},
		},
		"required": []string{"url"},
	}
}

func (*ExaGetContentsTool) CheckPermissions(params map[string]any) string {
	urlStr, _ := params["url"].(string)
	if containsExaInternalURL(urlStr) {
		return "Fetch blocked: internal URL detected"
	}
	return ""
}

func (*ExaGetContentsTool) Execute(params map[string]any) ToolResult {
	urlStr, _ := params["url"].(string)
	if urlStr == "" {
		return ToolResult{Output: "Error: url is required", IsError: true}
	}

	extractMode := "markdown"
	if em, ok := params["extract_mode"].(string); ok {
		extractMode = em
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := callExaMCP(ctx, "get_contents", map[string]any{
		"urls":       []string{urlStr},
		"numResults": 1,
		"text":       true,
		"contents": map[string]any{
			"text": extractMode == "text",
		},
	})
	if err != nil {
		return ToolResult{Output: fmt.Sprintf("Fetch error: %v", err), IsError: true}
	}

	if result == "" {
		return ToolResult{Output: fmt.Sprintf("No content retrieved for: %s", urlStr)}
	}

	return ToolResult{Output: result}
}

// mcpRequest builds the JSON-RPC request for Exa MCP.
func mcpRequest(tool string, args map[string]any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": args,
		},
	}
}

// callExaMCP makes an HTTP POST to the Exa MCP server and parses the SSE response.
func callExaMCP(ctx context.Context, tool string, args map[string]any) (string, error) {
	body, err := json.Marshal(mcpRequest(tool, args))
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", exaMCPURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "miniClaudeCode-go/0.1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
			return "", fmt.Errorf("Exa request timed out after 30s")
		}
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 200000))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return parseExaResponse(string(respBody))
}

// parseExaResponse parses the SSE or JSON response from Exa MCP.
func parseExaResponse(raw string) (string, error) {
	// Try parsing as JSON first (sometimes MCP returns direct JSON)
	var direct map[string]any
	if err := json.Unmarshal([]byte(raw), &direct); err == nil {
		if content, ok := extractContent(direct); ok && content != "" {
			return content, nil
		}
		return "", nil
	}

	// Parse SSE format (data: {...}\ndata: {...})
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		var msg map[string]any
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			continue
		}
		if content, ok := extractContent(msg); ok && content != "" {
			return content, nil
		}
	}

	return "", nil
}

// extractContent extracts text content from an Exa MCP response.
func extractContent(msg map[string]any) (string, bool) {
	result, ok := msg["result"].(map[string]any)
	if !ok {
		return "", false
	}

	content, ok := result["content"].([]any)
	if !ok {
		return "", false
	}

	var parts []string
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := block["type"].(string); typ == "text" {
			if text, _ := block["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n"), true
	}

	return "", false
}

func containsExaInternalURL(query string) bool {
	lower := strings.ToLower(query)
	internalPatterns := []string{
		"localhost", "127.0.0.1", "192.168.", "10.0.",
		"internal.", "staging.", "dev.local",
	}
	for _, p := range internalPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
