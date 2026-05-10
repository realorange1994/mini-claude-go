package tools

import (
	"testing"
)

// ─── MemoryAddTool ──────────────────────────────────────────────────────────

func TestMemoryAddToolName(t *testing.T) {
	tool := &MemoryAddTool{}
	if tool.Name() != "memory_add" {
		t.Errorf("expected 'memory_add', got %q", tool.Name())
	}
}

func TestMemoryAddToolSchema(t *testing.T) {
	tool := &MemoryAddTool{}
	schema := tool.InputSchema()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["category"]; !ok {
		t.Error("schema should have category property")
	}
	if _, ok := props["content"]; !ok {
		t.Error("schema should have content property")
	}
}

func TestMemoryAddToolExecuteValid(t *testing.T) {
	var capturedCategory, capturedContent string
	tool := &MemoryAddTool{
		OnAdd: func(category, content, source string) {
			capturedCategory = category
			capturedContent = content
		},
	}
	result := tool.Execute(map[string]any{
		"category": "preference",
		"content":  "user likes dark mode",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if capturedCategory != "preference" {
		t.Errorf("expected category 'preference', got %q", capturedCategory)
	}
	if capturedContent != "user likes dark mode" {
		t.Errorf("expected content 'user likes dark mode', got %q", capturedContent)
	}
}

func TestMemoryAddToolExecuteMissingCategory(t *testing.T) {
	tool := &MemoryAddTool{OnAdd: func(c, ct, s string) {}}
	result := tool.Execute(map[string]any{"content": "test"})
	if !result.IsError {
		t.Error("missing category should return error")
	}
}

func TestMemoryAddToolExecuteMissingContent(t *testing.T) {
	tool := &MemoryAddTool{OnAdd: func(c, ct, s string) {}}
	result := tool.Execute(map[string]any{"category": "state"})
	if !result.IsError {
		t.Error("missing content should return error")
	}
}

func TestMemoryAddToolExecuteNilCallback(t *testing.T) {
	tool := &MemoryAddTool{OnAdd: nil}
	result := tool.Execute(map[string]any{
		"category": "state",
		"content":  "test",
	})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestMemoryAddToolPermissions(t *testing.T) {
	tool := &MemoryAddTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

// ─── MemorySearchTool ───────────────────────────────────────────────────────

func TestMemorySearchToolName(t *testing.T) {
	tool := &MemorySearchTool{}
	if tool.Name() != "memory_search" {
		t.Errorf("expected 'memory_search', got %q", tool.Name())
	}
}

func TestMemorySearchToolSchema(t *testing.T) {
	tool := &MemorySearchTool{}
	schema := tool.InputSchema()
	required, _ := schema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Error("schema should require 'query'")
	}
}

func TestMemorySearchToolExecuteWithResults(t *testing.T) {
	tool := &MemorySearchTool{
		OnSearch: func(query string) []MemorySearchResult {
			return []MemorySearchResult{
				{Category: "state", Content: "working on feature"},
			}
		},
	}
	result := tool.Execute(map[string]any{
		"query": "feature",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestMemorySearchToolExecuteNoResults(t *testing.T) {
	tool := &MemorySearchTool{
		OnSearch: func(query string) []MemorySearchResult {
			return nil
		},
	}
	result := tool.Execute(map[string]any{
		"query": "nothing",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
}

func TestMemorySearchToolExecuteMissingQuery(t *testing.T) {
	tool := &MemorySearchTool{OnSearch: func(q string) []MemorySearchResult { return nil }}
	result := tool.Execute(map[string]any{})
	if !result.IsError {
		t.Error("missing query should return error")
	}
}

func TestMemorySearchToolExecuteNilCallback(t *testing.T) {
	tool := &MemorySearchTool{OnSearch: nil}
	result := tool.Execute(map[string]any{
		"query": "test",
	})
	if !result.IsError {
		t.Error("nil callback should return error")
	}
}

func TestMemorySearchToolPermissions(t *testing.T) {
	tool := &MemorySearchTool{}
	result := tool.CheckPermissions(nil)
	if result.Behavior != PermissionPassthrough {
		t.Error("should passthrough permissions")
	}
}

// ─── MemorySearchResult ─────────────────────────────────────────────────────

func TestMemorySearchResult(t *testing.T) {
	r := MemorySearchResult{Category: "decision", Content: "use Go"}
	if r.Category != "decision" {
		t.Errorf("expected category 'decision', got %q", r.Category)
	}
	if r.Content != "use Go" {
		t.Errorf("expected content 'use Go', got %q", r.Content)
	}
}
