package tools

import (
	"strings"
	"testing"
)

// ─── FileEncodingTool ───────────────────────────────────────────────────────

func TestFileEncodingName(t *testing.T) {
	tool := &FileEncodingTool{}
	if tool.Name() != "file_encoding" {
		t.Errorf("Name() = %q, want file_encoding", tool.Name())
	}
}

func TestFileEncodingDescription(t *testing.T) {
	tool := &FileEncodingTool{}
	desc := tool.Description()
	if !strings.Contains(desc, "encoding") {
		t.Error("description should mention encoding")
	}
}

func TestFileEncodingInputSchema(t *testing.T) {
	tool := &FileEncodingTool{}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in schema")
	}
	for _, key := range []string{"path", "encoding", "operation"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema should have %q property", key)
		}
	}
}

func TestFileEncodingCheckPermissions(t *testing.T) {
	tool := &FileEncodingTool{}
	result := tool.CheckPermissions(map[string]any{})
	if result.Behavior != PermissionPassthrough {
		t.Errorf("FileEncodingTool should passthrough permissions, got: %v", result)
	}
}

func TestFileEncodingDetectOperation(t *testing.T) {
	tool := &FileEncodingTool{}
	// Detect encoding of a simple ASCII file (the test file itself)
	result := tool.Execute(map[string]any{
		"path":      "file_encoding_test.go",
		"operation": "detect",
	})
	if result.IsError {
		t.Fatalf("detect error: %s", result.Output)
	}
	if !strings.Contains(strings.ToUpper(result.Output), "UTF-8") && !strings.Contains(strings.ToUpper(result.Output), "ASCII") {
		t.Errorf("expected UTF-8 or ASCII encoding, got: %s", result.Output)
	}
}

func TestFileEncodingDetectNoPath(t *testing.T) {
	tool := &FileEncodingTool{}
	result := tool.Execute(map[string]any{
		"operation": "detect",
	})
	if !result.IsError {
		t.Error("expected error when path is missing")
	}
}

func TestFileEncodingDetectNonexistent(t *testing.T) {
	tool := &FileEncodingTool{}
	result := tool.Execute(map[string]any{
		"path":      "nonexistent_file_xyz_12345.go",
		"operation": "detect",
	})
	if !result.IsError {
		t.Error("expected error for nonexistent file")
	}
}

func TestFileEncodingUnknownOperation(t *testing.T) {
	tool := &FileEncodingTool{}
	result := tool.Execute(map[string]any{
		"path":      "file_encoding_test.go",
		"operation": "convert",
	})
	// "convert" is not a valid operation — should error
	if !result.IsError {
		t.Error("expected error for invalid operation")
	}
	if !strings.Contains(result.Output, "unknown operation") {
		t.Errorf("expected 'unknown operation' error, got: %s", result.Output)
	}
}

func TestFileEncodingReadOperation(t *testing.T) {
	tool := &FileEncodingTool{}
	// Read with explicit encoding
	result := tool.Execute(map[string]any{
		"path":      "file_encoding_test.go",
		"operation": "read",
		"encoding":  "utf-8",
	})
	if result.IsError {
		t.Fatalf("read error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "package tools") {
		t.Errorf("expected file content, got: %s", result.Output[:min(200, len(result.Output))])
	}
}

func TestFileEncodingReadUnsupportedEncoding(t *testing.T) {
	tool := &FileEncodingTool{}
	result := tool.Execute(map[string]any{
		"path":      "file_encoding_test.go",
		"operation": "read",
		"encoding":  "bogus-encoding-xyz",
	})
	if !result.IsError {
		t.Error("expected error for unsupported encoding")
	}
}

func TestFileEncodingDetectWithEncodingHint(t *testing.T) {
	tool := &FileEncodingTool{}
	result := tool.Execute(map[string]any{
		"path":      "file_encoding_test.go",
		"operation": "detect",
		"encoding":  "utf-8",
	})
	if result.IsError {
		t.Fatalf("detect with hint error: %s", result.Output)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
