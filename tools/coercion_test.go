package tools

import (
	"testing"
)

func TestCoerceArgumentsStringToInt(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	args := map[string]any{
		"count": "42",
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	if args["count"] != 42 {
		t.Errorf("expected count=42 (int), got %v (%T)", args["count"], args["count"])
	}
}

func TestCoerceArgumentsStringToFloat(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ratio": map[string]any{"type": "number"},
		},
	}
	args := map[string]any{
		"ratio": "3.14",
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	if args["ratio"] != 3.14 {
		t.Errorf("expected ratio=3.14 (float64), got %v (%T)", args["ratio"], args["ratio"])
	}
}

func TestCoerceArgumentsStringToBool(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"verbose": map[string]any{"type": "boolean"},
		},
	}

	tests := []struct {
		input  string
		expect bool
	}{
		{"true", true},
		{"false", false},
		{"True", true},
		{"False", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			args := map[string]any{"verbose": tt.input}
			warnings := CoerceArguments(schema, args)
			if len(warnings) == 0 {
				t.Errorf("expected coercion warning for %q", tt.input)
			}
			if args["verbose"] != tt.expect {
				t.Errorf("expected verbose=%v, got %v", tt.expect, args["verbose"])
			}
		})
	}
}

func TestCoerceArgumentsStringToArray(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{"type": "array"},
		},
	}
	args := map[string]any{
		"items": "[1, 2, 3]",
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	arr, ok := args["items"].([]any)
	if !ok {
		t.Fatalf("expected array, got %T", args["items"])
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 items, got %d", len(arr))
	}
}

func TestCoerceArgumentsIntToString(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
	args := map[string]any{
		"path": 42,
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	if args["path"] != "42" {
		t.Errorf("expected path='42', got %v", args["path"])
	}
}

func TestCoerceArgumentsBoolToString(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	args := map[string]any{
		"name": true,
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	if args["name"] != "true" {
		t.Errorf("expected name='true', got %v", args["name"])
	}
}

func TestCoerceArgumentsNoCoercionNeeded(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
			"name":  map[string]any{"type": "string"},
		},
	}
	args := map[string]any{
		"count": 42,
		"name":  "test",
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(warnings))
	}
	if args["count"] != 42 {
		t.Errorf("count should remain 42, got %v", args["count"])
	}
	if args["name"] != "test" {
		t.Errorf("name should remain 'test', got %v", args["name"])
	}
}

func TestCoerceArgumentsInvalidStringToInt(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	args := map[string]any{
		"count": "not_a_number",
	}

	// Should not coerce, leave as-is
	CoerceArguments(schema, args)
	if args["count"] != "not_a_number" {
		t.Errorf("invalid int string should remain unchanged, got %v", args["count"])
	}
}

func TestCoerceArgumentsInvalidStringToArray(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{"type": "array"},
		},
	}
	args := map[string]any{
		"items": "not json array",
	}

	// Should not coerce, leave as-is
	CoerceArguments(schema, args)
	if args["items"] != "not json array" {
		t.Errorf("invalid array string should remain unchanged, got %v", args["items"])
	}
}

func TestCoerceArgumentsExtraArgs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	args := map[string]any{
		"count":   42,
		"unknown": "value",
	}

	// Extra args should be left alone
	CoerceArguments(schema, args)
	if args["unknown"] != "value" {
		t.Errorf("extra args should be preserved, got %v", args["unknown"])
	}
}

func TestCoerceArgumentsNilSchema(t *testing.T) {
	args := map[string]any{
		"count": "42",
	}

	// Nil schema should not panic
	CoerceArguments(nil, args)
	if args["count"] != "42" {
		t.Errorf("nil schema should not modify args, got %v", args["count"])
	}
}

func TestCoerceArgumentsEmptySchema(t *testing.T) {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	args := map[string]any{
		"count": "42",
	}

	CoerceArguments(schema, args)
	if args["count"] != "42" {
		t.Errorf("empty properties should not modify args, got %v", args["count"])
	}
}