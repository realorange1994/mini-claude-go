package main

import (
	"strings"
	"testing"
	"time"
)

// ─── SSE Chunk Timeout Tests ────────────────────────────────────────────────

func TestSSEChunkReader_Timeout(t *testing.T) {
	// Create a reader that never returns
	r := NewSSEChunkReader(&slowReader{delay: 2 * time.Second}, 100*time.Millisecond)

	buf := make([]byte, 10)
	_, err := r.Read(buf)
	if err == nil {
		t.Error("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' error, got: %v", err)
	}
}

func TestSSEChunkReader_Success(t *testing.T) {
	r := NewSSEChunkReader(strings.NewReader("hello"), 1*time.Second)

	buf := make([]byte, 10)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes, got %d", n)
	}
}

func TestSSEChunkReader_Close(t *testing.T) {
	r := NewSSEChunkReader(strings.NewReader("hello"), 1*time.Second)

	err := r.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWrapReader_Disabled(t *testing.T) {
	original := strings.NewReader("hello")
	config := &SSEChunkTimeoutConfig{Enabled: false}

	wrapped := WrapReader(original, config)
	if wrapped != original {
		t.Error("expected original reader when disabled")
	}
}

func TestWrapReader_NilConfig(t *testing.T) {
	original := strings.NewReader("hello")

	wrapped := WrapReader(original, nil)
	if wrapped != original {
		t.Error("expected original reader when config is nil")
	}
}

func TestNewSSEChunkTimeoutConfig(t *testing.T) {
	config := NewSSEChunkTimeoutConfig()

	if !config.Enabled {
		t.Error("expected enabled by default")
	}
	if config.Timeout != DefaultChunkTimeout {
		t.Errorf("expected %v, got %v", DefaultChunkTimeout, config.Timeout)
	}
}

type slowReader struct {
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	copy(p, "data")
	return 4, nil
}

// ─── Schema Flattener Tests ────────────────────────────────────────────────

func TestSchemaFlattener_FlattenUnion(t *testing.T) {
	f := NewSchemaFlattener()

	schema := map[string]any{
		"anyOf": []any{
			map[string]any{
				"properties": map[string]any{
					"type": map[string]any{"enum": []any{"file"}},
					"path": map[string]any{"type": "string"},
				},
				"required": []any{"type", "path"},
			},
			map[string]any{
				"properties": map[string]any{
					"type":  map[string]any{"enum": []any{"dir"}},
					"path":  map[string]any{"type": "string"},
					"depth": map[string]any{"type": "number"},
				},
				"required": []any{"type", "path"},
			},
		},
	}

	result := f.FlattenSchema(schema)

	if result["type"] != "object" {
		t.Errorf("expected 'object', got %v", result["type"])
	}

	props, ok := result["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	if _, exists := props["path"]; !exists {
		t.Error("expected 'path' property")
	}
	if _, exists := props["depth"]; !exists {
		t.Error("expected 'depth' property")
	}
}

func TestSchemaFlattener_NoUnion(t *testing.T) {
	f := NewSchemaFlattener()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	result := f.FlattenSchema(schema)

	if result["type"] != "object" {
		t.Errorf("expected 'object', got %v", result["type"])
	}
}

func TestSchemaFlattener_SanitizeForGemini(t *testing.T) {
	f := NewSchemaFlattener()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"enum": []any{1, 2, 3},
			},
		},
		"required": []any{"mode", "nonexistent"},
	}

	result := f.SanitizeForGemini(schema)

	// Integer enums should be converted to strings
	props := result["properties"].(map[string]any)
	mode := props["mode"].(map[string]any)
	enum := mode["enum"].([]any)
	if enum[0] != "1" {
		t.Errorf("expected '1', got %v", enum[0])
	}

	// Required should be filtered
	req := result["required"].([]string)
	if len(req) != 1 || req[0] != "mode" {
		t.Errorf("expected ['mode'], got %v", req)
	}
}

func TestFlattenToolInputSchema(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{
			map[string]any{
				"properties": map[string]any{
					"action": map[string]any{"enum": []any{"read"}},
				},
			},
		},
	}

	result := FlattenToolInputSchema(schema)
	if result["type"] != "object" {
		t.Errorf("expected 'object', got %v", result["type"])
	}
}

func TestSanitizeToolSchemaForGemini(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "number"},
		},
	}

	result := SanitizeToolSchemaForGemini(schema)
	if result["type"] != "object" {
		t.Errorf("expected 'object', got %v", result["type"])
	}
}

func TestSchemaToJSON(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	result := SchemaToJSON(schema)
	if result == "" {
		t.Error("expected non-empty JSON")
	}
}

func TestUniqueStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	result := uniqueStrings(input)
	if len(result) != 3 {
		t.Errorf("expected 3 unique strings, got %d", len(result))
	}
}
