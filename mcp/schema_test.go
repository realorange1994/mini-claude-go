package mcp

import (
	"strings"
	"testing"
)

func TestValidateRequired(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"count": map[string]any{"type": "integer"},
		},
		"required": []any{"name"},
	}

	// Missing required
	err := ValidateSchema(map[string]any{"count": float64(5)}, schema)
	if err == nil {
		t.Error("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "missing required") {
		t.Errorf("expected 'missing required' in error, got: %s", err)
	}

	// All required present
	err = ValidateSchema(map[string]any{"name": "test", "count": float64(5)}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}
}

func TestValidateTypes(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"s": map[string]any{"type": "string"},
			"n": map[string]any{"type": "number"},
			"i": map[string]any{"type": "integer"},
			"b": map[string]any{"type": "boolean"},
			"a": map[string]any{"type": "array"},
			"o": map[string]any{"type": "object"},
		},
	}

	// Wrong type
	err := ValidateSchema(map[string]any{"s": float64(42)}, schema)
	if err == nil {
		t.Error("expected error for wrong type")
	}

	err = ValidateSchema(map[string]any{"b": "true"}, schema)
	if err == nil {
		t.Error("expected error for wrong type")
	}

	// Correct types
	err = ValidateSchema(map[string]any{
		"s": "hello",
		"n": float64(3.14),
		"i": float64(42),
		"b": true,
		"a": []any{"x"},
		"o": map[string]any{},
	}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}
}

func TestValidateEnum(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type": "string",
				"enum": []any{"auto", "manual", "ask"},
			},
		},
	}

	err := ValidateSchema(map[string]any{"mode": "auto"}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}

	err = ValidateSchema(map[string]any{"mode": "invalid"}, schema)
	if err == nil {
		t.Error("expected error for invalid enum value")
	}
}

func TestValidateStringConstraints(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"minLength": float64(1),
				"maxLength": float64(100),
			},
		},
	}

	err := ValidateSchema(map[string]any{"name": ""}, schema)
	if err == nil {
		t.Error("expected error for minLength violation")
	}

	err = ValidateSchema(map[string]any{"name": strings.Repeat("x", 101)}, schema)
	if err == nil {
		t.Error("expected error for maxLength violation")
	}

	err = ValidateSchema(map[string]any{"name": "ok"}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}
}

func TestValidateNumberConstraints(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{
				"type":    "integer",
				"minimum": float64(0),
				"maximum": float64(100),
			},
		},
	}

	err := ValidateSchema(map[string]any{"count": float64(-1)}, schema)
	if err == nil {
		t.Error("expected error for minimum violation")
	}

	err = ValidateSchema(map[string]any{"count": float64(101)}, schema)
	if err == nil {
		t.Error("expected error for maximum violation")
	}

	err = ValidateSchema(map[string]any{"count": float64(50)}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}
}

func TestValidateArrayConstraints(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type":     "array",
				"minItems": float64(1),
				"maxItems": float64(10),
				"items":    map[string]any{"type": "string"},
			},
		},
	}

	err := ValidateSchema(map[string]any{"items": []any{}}, schema)
	if err == nil {
		t.Error("expected error for minItems violation")
	}

	err = ValidateSchema(map[string]any{"items": []any{"a", "b"}}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}
}

func TestValidateNullValue(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	err := ValidateSchema(map[string]any{"name": nil}, schema)
	if err == nil {
		t.Error("expected error for null value in non-null type")
	}
}

func TestValidateNoSchema(t *testing.T) {
	err := ValidateSchema(map[string]any{"foo": "bar"}, nil)
	if err != nil {
		t.Errorf("expected no validation with nil schema, got: %s", err)
	}

	err = ValidateSchema(map[string]any{"foo": "bar"}, map[string]any{})
	if err != nil {
		t.Errorf("expected no validation with empty schema, got: %s", err)
	}
}

func TestValidateNestedObject(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"debug": map[string]any{"type": "boolean"},
				},
				"required": []any{"debug"},
			},
		},
	}

	err := ValidateSchema(map[string]any{
		"config": map[string]any{},
	}, schema)
	if err == nil {
		t.Error("expected error for missing required in nested object")
	}

	err = ValidateSchema(map[string]any{
		"config": map[string]any{"debug": true},
	}, schema)
	if err != nil {
		t.Errorf("expected valid, got: %s", err)
	}
}
