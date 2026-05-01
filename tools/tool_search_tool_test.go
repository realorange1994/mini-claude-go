package tools

import (
	"strings"
	"testing"
)

func TestToolSearchSelectForm(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&ExecTool{})
	registry.Register(&GlobTool{})
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, ok := registry.Get("tool_search")
	if !ok {
		t.Fatal("tool_search not found")
	}
	tst := tool.(*ToolSearchTool)

	result := tst.Execute(map[string]any{"query": "select:exec,glob"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "## exec") {
		t.Errorf("expected exec tool in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "## glob") {
		t.Errorf("expected glob tool in output, got: %s", result.Output)
	}
}

func TestToolSearchSelectNotFound(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&ExecTool{})
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, _ := registry.Get("tool_search")
	tst := tool.(*ToolSearchTool)

	result := tst.Execute(map[string]any{"query": "select:nonexistent_tool"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "not found") {
		t.Errorf("expected 'not found' message, got: %s", result.Output)
	}
}

func TestToolSearchKeywordForm(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&FileReadTool{})
	registry.Register(&FileWriteTool{})
	registry.Register(&GlobTool{})
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, _ := registry.Get("tool_search")
	tst := tool.(*ToolSearchTool)

	// Search for "file read" should match file_read
	result := tst.Execute(map[string]any{"query": "file read"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "## read_file") {
		t.Errorf("expected read_file tool in output, got: %s", result.Output)
	}
}

func TestToolSearchPrefixForm(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&FileReadTool{})
	registry.Register(&FileWriteTool{})
	registry.Register(&GlobTool{})
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, _ := registry.Get("tool_search")
	tst := tool.(*ToolSearchTool)

	// Search "+file" should only match tools with "file" in name
	result := tst.Execute(map[string]any{"query": "+file"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "## read_file") {
		t.Errorf("expected read_file in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "## write_file") {
		t.Errorf("expected write_file in output, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "## glob") {
		t.Errorf("did not expect glob in output, got: %s", result.Output)
	}
}

func TestToolSearchNoResults(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&ExecTool{})
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, _ := registry.Get("tool_search")
	tst := tool.(*ToolSearchTool)

	result := tst.Execute(map[string]any{"query": "xyznonexistent"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No tools matched") {
		t.Errorf("expected 'No tools matched' message, got: %s", result.Output)
	}
}

func TestToolSearchMaxResults(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&ExecTool{})
	registry.Register(&FileReadTool{})
	registry.Register(&FileWriteTool{})
	registry.Register(&GlobTool{})
	registry.Register(&GrepTool{})
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, _ := registry.Get("tool_search")
	tst := tool.(*ToolSearchTool)

	// Search for "file" should match file_read and file_write
	result := tst.Execute(map[string]any{
		"query":       "file",
		"max_results": 1,
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	// With max_results=1, should only get one tool
	count := strings.Count(result.Output, "## ")
	if count > 1 {
		t.Errorf("expected at most 1 tool, got %d: %s", count, result.Output)
	}
}

func TestToolSearchSkipsSelf(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&ToolSearchTool{Registry: registry})

	tool, _ := registry.Get("tool_search")
	tst := tool.(*ToolSearchTool)

	// Searching for "search" should not return tool_search itself
	result := tst.Execute(map[string]any{"query": "search"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if strings.Contains(result.Output, "## tool_search") {
		t.Errorf("tool_search should not appear in its own results, got: %s", result.Output)
	}
}
