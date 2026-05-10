package tools

import (
	"testing"
)

// ─── ExaSearchTool interface ─────────────────────────────────────────────────

func TestExaSearchToolName(t *testing.T) {
	tool := &ExaSearchTool{}
	if tool.Name() != "web_search" {
		t.Errorf("expected 'web_search', got %q", tool.Name())
	}
}

func TestExaSearchToolSchema(t *testing.T) {
	tool := &ExaSearchTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("expected required=[query], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["query"]; !ok {
		t.Error("schema should have query property")
	}
	if _, ok := props["num_results"]; !ok {
		t.Error("schema should have num_results property")
	}
	if _, ok := props["livecrawl"]; !ok {
		t.Error("schema should have livecrawl property")
	}
	if _, ok := props["type"]; !ok {
		t.Error("schema should have type property")
	}
}

func TestExaSearchToolPermissionsPublic(t *testing.T) {
	tool := &ExaSearchTool{}
	result := tool.CheckPermissions(map[string]any{"query": "golang testing"})
	if result.Behavior != PermissionPassthrough {
		t.Error("public query should passthrough")
	}
}

func TestExaSearchToolPermissionsInternalURL(t *testing.T) {
	tool := &ExaSearchTool{}
	result := tool.CheckPermissions(map[string]any{"query": "http://localhost:8080"})
	if result.Behavior != PermissionAsk {
		t.Error("internal URL in query should ask")
	}
}

func TestExaSearchToolPermissionsStaging(t *testing.T) {
	tool := &ExaSearchTool{}
	result := tool.CheckPermissions(map[string]any{"query": "https://staging.example.com"})
	if result.Behavior != PermissionAsk {
		t.Error("staging URL should ask")
	}
}

func TestExaSearchToolExecuteNoQuery(t *testing.T) {
	tool := &ExaSearchTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing query should return error")
	}
}

func TestExaSearchToolExecuteEmptyQuery(t *testing.T) {
	tool := &ExaSearchTool{}
	result := tool.Execute(map[string]any{"query": ""})
	if !result.IsError {
		t.Error("empty query should return error")
	}
}

// ─── ExaGetContentsTool interface ────────────────────────────────────────────

func TestExaGetContentsToolName(t *testing.T) {
	tool := &ExaGetContentsTool{}
	if tool.Name() != "web_fetch" {
		t.Errorf("expected 'web_fetch', got %q", tool.Name())
	}
}

func TestExaGetContentsToolSchema(t *testing.T) {
	tool := &ExaGetContentsTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "url" {
		t.Errorf("expected required=[url], got %v", required)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["url"]; !ok {
		t.Error("schema should have url property")
	}
	if _, ok := props["extract_mode"]; !ok {
		t.Error("schema should have extract_mode property")
	}
}

func TestExaGetContentsToolPermissionsPublic(t *testing.T) {
	tool := &ExaGetContentsTool{}
	result := tool.CheckPermissions(map[string]any{"url": "https://example.com"})
	if result.Behavior != PermissionPassthrough {
		t.Error("public URL should passthrough")
	}
}

func TestExaGetContentsToolPermissionsInternalURL(t *testing.T) {
	tool := &ExaGetContentsTool{}
	result := tool.CheckPermissions(map[string]any{"url": "http://127.0.0.1:3000"})
	if result.Behavior != PermissionAsk {
		t.Error("internal URL should ask")
	}
}

func TestExaGetContentsToolExecuteNoURL(t *testing.T) {
	tool := &ExaGetContentsTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing url should return error")
	}
}

func TestExaGetContentsToolExecuteEmptyURL(t *testing.T) {
	tool := &ExaGetContentsTool{}
	result := tool.Execute(map[string]any{"url": ""})
	if !result.IsError {
		t.Error("empty url should return error")
	}
}

// ─── containsExaInternalURL ─────────────────────────────────────────────────

func TestContainsExaInternalURLLocalhost(t *testing.T) {
	if !containsExaInternalURL("http://localhost:8080") {
		t.Error("localhost should be detected")
	}
}

func TestContainsExaInternalURL127(t *testing.T) {
	if !containsExaInternalURL("http://127.0.0.1") {
		t.Error("127.0.0.1 should be detected")
	}
}

func TestContainsExaInternalURL192168(t *testing.T) {
	if !containsExaInternalURL("http://192.168.1.1") {
		t.Error("192.168. should be detected")
	}
}

func TestContainsExaInternalURL10(t *testing.T) {
	if !containsExaInternalURL("http://10.0.0.1") {
		t.Error("10.0. should be detected")
	}
}

func TestContainsExaInternalURLInternal(t *testing.T) {
	if !containsExaInternalURL("https://internal.company.com") {
		t.Error("internal. should be detected")
	}
}

func TestContainsExaInternalURLStaging(t *testing.T) {
	if !containsExaInternalURL("https://staging.example.com") {
		t.Error("staging. should be detected")
	}
}

func TestContainsExaInternalURLDevLocal(t *testing.T) {
	if !containsExaInternalURL("http://app.dev.local") {
		t.Error("dev.local should be detected")
	}
}

func TestContainsExaInternalURLPublic(t *testing.T) {
	if containsExaInternalURL("https://example.com") {
		t.Error("public URL should not be detected as internal")
	}
}

func TestContainsExaInternalURLEmpty(t *testing.T) {
	if containsExaInternalURL("") {
		t.Error("empty string should not be detected as internal")
	}
}

// ─── parseExaResponse ───────────────────────────────────────────────────────

func TestParseExaResponseJSON(t *testing.T) {
	raw := `{"result":{"content":[{"type":"text","text":"Hello world"}]}}`
	result, err := parseExaResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", result)
	}
}

func TestParseExaResponseSSE(t *testing.T) {
	raw := "data: {\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"SSE content\"}]}}\n\n"
	result, err := parseExaResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "SSE content" {
		t.Errorf("expected 'SSE content', got %q", result)
	}
}

func TestParseExaResponseEmpty(t *testing.T) {
	raw := `{"result":{"content":[]}}`
	result, err := parseExaResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestParseExaResponseInvalidJSON(t *testing.T) {
	raw := "not json at all"
	result, err := parseExaResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("invalid JSON should return empty, got %q", result)
	}
}

// ─── extractContent ─────────────────────────────────────────────────────────

func TestExtractContentWithText(t *testing.T) {
	msg := map[string]any{
		"result": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "hello"},
			},
		},
	}
	content, ok := extractContent(msg)
	if !ok {
		t.Error("should find content")
	}
	if content != "hello" {
		t.Errorf("expected 'hello', got %q", content)
	}
}

func TestExtractContentMultipleBlocks(t *testing.T) {
	msg := map[string]any{
		"result": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "part1"},
				map[string]any{"type": "text", "text": "part2"},
			},
		},
	}
	content, ok := extractContent(msg)
	if !ok {
		t.Error("should find content")
	}
	if content != "part1\npart2" {
		t.Errorf("expected 'part1\\npart2', got %q", content)
	}
}

func TestExtractContentNoResult(t *testing.T) {
	msg := map[string]any{"error": "something"}
	content, ok := extractContent(msg)
	if ok {
		t.Error("should not find content without result key")
	}
	if content != "" {
		t.Errorf("expected empty, got %q", content)
	}
}

func TestExtractContentNonTextBlock(t *testing.T) {
	msg := map[string]any{
		"result": map[string]any{
			"content": []any{
				map[string]any{"type": "image", "data": "base64..."},
			},
		},
	}
	content, ok := extractContent(msg)
	if ok {
		t.Error("image block should not yield text content")
	}
	if content != "" {
		t.Errorf("expected empty, got %q", content)
	}
}

// ─── mcpRequest ─────────────────────────────────────────────────────────────

func TestMcpRequestStructure(t *testing.T) {
	req := mcpRequest("web_search_exa", map[string]any{"query": "test"})
	if req["jsonrpc"] != "2.0" {
		t.Error("jsonrpc should be 2.0")
	}
	if req["method"] != "tools/call" {
		t.Error("method should be tools/call")
	}
	params, ok := req["params"].(map[string]any)
	if !ok {
		t.Fatal("params should be a map")
	}
	if params["name"] != "web_search_exa" {
		t.Error("tool name should be web_search_exa")
	}
}
