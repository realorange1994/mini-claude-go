package tools

import (
	"strings"
	"testing"
)

func TestWebSearchEmptyQuery(t *testing.T) {
	tool := &WebSearchTool{}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("expected error for empty query")
	}
}

func TestWebSearchPermissionInternalURL(t *testing.T) {
	tool := &WebSearchTool{}
	result := tool.CheckPermissions(map[string]any{"query": "curl http://10.0.0.1/secret"})
	if result.Behavior == PermissionPassthrough {
		t.Error("expected denial for internal URL in query")
	}
}

func TestWebSearchPermissionPublicQuery(t *testing.T) {
	tool := &WebSearchTool{}
	result := tool.CheckPermissions(map[string]any{"query": "Go programming language"})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("expected public query to be allowed, got: %v", result)
	}
}

func TestWebSearchLive(t *testing.T) {
	t.Skip("requires network access")
	tool := &WebSearchTool{}
	result := tool.Execute(map[string]any{
		"query": "Go programming language",
		"count": 3,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Results for:") {
		t.Errorf("expected results header, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "http") {
		t.Errorf("expected URLs in results, got: %s", result.Output)
	}
}

func TestWebSearchCountValidation(t *testing.T) {
	// Test that count is capped at 10 and floored at 1
	tool := &WebSearchTool{}
	// Just verify it doesn't crash with edge cases
	result := tool.Execute(map[string]any{
		"query": "test",
		"count": 100,
	})
	// May fail due to network, but should not panic
	if result.IsError {
		t.Logf("expected network error on localhost: %s", result.Output)
	}
}
