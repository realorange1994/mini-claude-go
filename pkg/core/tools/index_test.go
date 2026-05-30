package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTools(t *testing.T) {
	reg := DefaultTools()

	expectedTools := []string{"Read", "Write", "Edit", "Bash", "Grep", "Find", "Glob", "Ls"}
	for _, name := range expectedTools {
		tool := reg.Get(name)
		if tool == nil {
			t.Errorf("tool %s not registered", name)
			continue
		}
		if tool.Definition.Name != name {
			t.Errorf("tool %s has wrong name in definition: %s", name, tool.Definition.Name)
		}
		if tool.Definition.Description == "" {
			t.Errorf("tool %s has empty description", name)
		}
		if tool.Definition.InputSchema == nil {
			t.Errorf("tool %s has empty input_schema", name)
		}
		if tool.Handler == nil {
			t.Errorf("tool %s has nil handler", name)
		}
	}
}

func TestGetDefinitions(t *testing.T) {
	reg := DefaultTools()
	defs := reg.GetDefinitions()

	expectedCount := 8
	if len(defs) != expectedCount {
		t.Errorf("GetDefinitions returned %d, want %d", len(defs), expectedCount)
	}
}

func TestExecute(t *testing.T) {
	reg := DefaultTools()

	// Test Read tool execution
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world\n"), 0644)

	result, err := reg.Execute(context.Background(), "Read", map[string]interface{}{
		"path": testFile,
	})
	if err != nil {
		t.Fatalf("Read tool error: %v", err)
	}
	if result != "hello world\n" {
		t.Errorf("Read result = %q, want %q", result, "hello world\n")
	}
}

func TestExecute_WriteAndRead(t *testing.T) {
	reg := DefaultTools()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test2.txt")

	// Write
	writeResult, err := reg.Execute(context.Background(), "Write", map[string]interface{}{
		"path":    testFile,
		"content": "test content for write and read",
	})
	if err != nil {
		t.Fatalf("Write tool error: %v", err)
	}
	if writeResult == "" {
		t.Error("Write tool returned empty result")
	}

	// Read back — write without newline, read adds one
	readResult, err := reg.Execute(context.Background(), "Read", map[string]interface{}{
		"path": testFile,
	})
	if err != nil {
		t.Fatalf("Read tool error: %v", err)
	}
	expected := "test content for write and read\n"
	if readResult != expected {
		t.Errorf("Read result = %q, want %q", readResult, expected)
	}
}

func TestExecute_Ls(t *testing.T) {
	reg := DefaultTools()

	tmpDir := t.TempDir()
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)

	result, err := reg.Execute(context.Background(), "Ls", map[string]interface{}{
		"path": tmpDir,
	})
	if err != nil {
		t.Fatalf("Ls tool error: %v", err)
	}
	if result == "" {
		t.Error("Ls tool returned empty result")
	}
}

func TestExecute_Grep(t *testing.T) {
	reg := DefaultTools()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "search_test.txt")
	os.WriteFile(testFile, []byte("hello world\nfoo bar\nhello test"), 0644)

	result, err := reg.Execute(context.Background(), "Grep", map[string]interface{}{
		"pattern": "hello",
		"paths":   []interface{}{testFile},
	})
	if err != nil {
		t.Fatalf("Grep tool error: %v", err)
	}
	if result == "" {
		t.Error("Grep tool returned empty result")
	}
}

func TestExecute_Glob(t *testing.T) {
	reg := DefaultTools()

	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	result, err := reg.Execute(context.Background(), "Glob", map[string]interface{}{
		"pattern": "*.go",
		"cwd":     tmpDir,
	})
	if err != nil {
		t.Fatalf("Glob tool error: %v", err)
	}
	if result == "" {
		t.Error("Glob tool returned empty result")
	}
}

func TestExecute_NonExistentTool(t *testing.T) {
	reg := DefaultTools()

	_, err := reg.Execute(context.Background(), "NonExistent", map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for non-existent tool, got nil")
	}
}

func TestParseToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantLen  int
	}{
		{
			name:     "JSON array of tool calls",
			response: `[{"id":"tc_1","name":"Read","input":{"path":"test.txt"}},{"id":"tc_2","name":"Write","input":{"path":"out.txt","content":"hello"}}]`,
			wantLen:  2,
		},
		{
			name:     "No tool calls",
			response: `I'll help you with that task. Let me think about it.`,
			wantLen:  0,
		},
		{
			name:     "Simple array format",
			response: `[{"name":"Read","input":{"path":"file.txt"}}]`,
			wantLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, _ := ParseToolCalls(tt.response)
			if len(calls) != tt.wantLen {
				t.Errorf("ParseToolCalls returned %d calls, want %d", len(calls), tt.wantLen)
			}
		})
	}
}
