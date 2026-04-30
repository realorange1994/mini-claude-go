package tools

import (
	"testing"
)

// --- Existing tests (updated where behavior changed) ---

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

	// Should wrap in single-element array as fallback
	CoerceArguments(schema, args)
	arr, ok := args["items"].([]any)
	if !ok {
		t.Fatalf("expected array (wrapped), got %T", args["items"])
	}
	if len(arr) != 1 || arr[0] != "not json array" {
		t.Errorf("expected single-element array wrapping the string, got %v", arr)
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

// --- New tests for missing features ---

func TestCoerceArgumentsBoolExtendedYesNo(t *testing.T) {
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
		{"yes", true},
		{"no", false},
		{"YES", true},
		{"NO", false},
		{"on", true},
		{"off", false},
		{"ON", true},
		{"OFF", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			args := map[string]any{"verbose": tt.input}
			warnings := CoerceArguments(schema, args)
			if len(warnings) == 0 {
				t.Errorf("expected coercion warning for %q", tt.input)
			}
			if args["verbose"] != tt.expect {
				t.Errorf("expected verbose=%v for input %q, got %v", tt.expect, tt.input, args["verbose"])
			}
		})
	}
}

func TestCoerceArgumentsNumberToBoolean(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"flag": map[string]any{"type": "boolean"},
		},
	}

	tests := []struct {
		name   string
		input  any
		expect bool
	}{
		{"int_zero", 0, false},
		{"int_nonzero", 42, true},
		{"int_negative", -1, true},
		{"float_zero", 0.0, false},
		{"float_nonzero", 3.14, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"flag": tt.input}
			warnings := CoerceArguments(schema, args)
			if len(warnings) == 0 {
				t.Errorf("expected coercion warning for %v", tt.input)
			}
			if args["flag"] != tt.expect {
				t.Errorf("expected flag=%v for input %v, got %v", tt.expect, tt.input, args["flag"])
			}
		})
	}
}

func TestCoerceArgumentsBoolToNumber(t *testing.T) {
	// bool -> integer
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"flag": map[string]any{"type": "integer"},
		},
	}

	t.Run("bool_true_to_int", func(t *testing.T) {
		args := map[string]any{"flag": true}
		warnings := CoerceArguments(schema, args)
		if len(warnings) == 0 {
			t.Error("expected coercion warning")
		}
		if args["flag"] != 1 {
			t.Errorf("expected flag=1, got %v", args["flag"])
		}
	})

	t.Run("bool_false_to_int", func(t *testing.T) {
		args := map[string]any{"flag": false}
		warnings := CoerceArguments(schema, args)
		if len(warnings) == 0 {
			t.Error("expected coercion warning")
		}
		if args["flag"] != 0 {
			t.Errorf("expected flag=0, got %v", args["flag"])
		}
	})

	// bool -> number (float)
	floatSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"flag": map[string]any{"type": "number"},
		},
	}

	t.Run("bool_true_to_float", func(t *testing.T) {
		args := map[string]any{"flag": true}
		warnings := CoerceArguments(floatSchema, args)
		if len(warnings) == 0 {
			t.Error("expected coercion warning")
		}
		if args["flag"] != 1.0 {
			t.Errorf("expected flag=1.0, got %v", args["flag"])
		}
	})

	t.Run("bool_false_to_float", func(t *testing.T) {
		args := map[string]any{"flag": false}
		warnings := CoerceArguments(floatSchema, args)
		if len(warnings) == 0 {
			t.Error("expected coercion warning")
		}
		if args["flag"] != 0.0 {
			t.Errorf("expected flag=0.0, got %v", args["flag"])
		}
	})
}

func TestCoerceArgumentsStringToObject(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{"type": "object"},
		},
	}

	t.Run("valid_json_object", func(t *testing.T) {
		args := map[string]any{
			"config": `{"key": "value", "num": 42}`,
		}
		warnings := CoerceArguments(schema, args)
		if len(warnings) == 0 {
			t.Error("expected coercion warning")
		}
		obj, ok := args["config"].(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", args["config"])
		}
		if obj["key"] != "value" {
			t.Errorf("expected key='value', got %v", obj["key"])
		}
		if obj["num"] != 42.0 {
			t.Errorf("expected num=42, got %v", obj["num"])
		}
	})

	t.Run("string_not_object", func(t *testing.T) {
		args := map[string]any{
			"config": "just a string",
		}
		CoerceArguments(schema, args)
		// Should not coerce -- doesn't start with {
		if args["config"] != "just a string" {
			t.Errorf("non-object string should remain unchanged, got %v", args["config"])
		}
	})

	t.Run("invalid_json_object", func(t *testing.T) {
		args := map[string]any{
			"config": "{invalid json}",
		}
		CoerceArguments(schema, args)
		// Should not coerce -- invalid JSON
		if args["config"] != "{invalid json}" {
			t.Errorf("invalid JSON object string should remain unchanged, got %v", args["config"])
		}
	})
}

func TestCoerceArgumentsArrayWrappingFallback(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{"type": "array"},
		},
	}

	// Non-JSON string should be wrapped in a single-element array
	args := map[string]any{
		"items": "hello",
	}
	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	arr, ok := args["items"].([]any)
	if !ok {
		t.Fatalf("expected array, got %T", args["items"])
	}
	if len(arr) != 1 || arr[0] != "hello" {
		t.Errorf("expected single-element array ['hello'], got %v", arr)
	}
}

func TestCoerceArgumentsTrimBeforeParsing(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count":   map[string]any{"type": "integer"},
			"ratio":   map[string]any{"type": "number"},
			"flag":    map[string]any{"type": "boolean"},
			"items":   map[string]any{"type": "array"},
			"config":  map[string]any{"type": "object"},
		},
	}

	t.Run("trimmed_int", func(t *testing.T) {
		args := map[string]any{"count": "  42  "}
		CoerceArguments(schema, args)
		if args["count"] != 42 {
			t.Errorf("expected count=42 after trim, got %v", args["count"])
		}
	})

	t.Run("trimmed_float", func(t *testing.T) {
		args := map[string]any{"ratio": "  3.14  "}
		CoerceArguments(schema, args)
		if args["ratio"] != 3.14 {
			t.Errorf("expected ratio=3.14 after trim, got %v", args["ratio"])
		}
	})

	t.Run("trimmed_bool_yes", func(t *testing.T) {
		args := map[string]any{"flag": "  yes  "}
		CoerceArguments(schema, args)
		if args["flag"] != true {
			t.Errorf("expected flag=true after trim, got %v", args["flag"])
		}
	})

	t.Run("trimmed_array", func(t *testing.T) {
		args := map[string]any{"items": "  [1, 2, 3]  "}
		CoerceArguments(schema, args)
		arr, ok := args["items"].([]any)
		if !ok {
			t.Fatalf("expected array, got %T", args["items"])
		}
		if len(arr) != 3 {
			t.Errorf("expected 3 items after trim, got %d", len(arr))
		}
	})

	t.Run("trimmed_object", func(t *testing.T) {
		args := map[string]any{"config": `  {"key": "val"}  `}
		CoerceArguments(schema, args)
		obj, ok := args["config"].(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", args["config"])
		}
		if obj["key"] != "val" {
			t.Errorf("expected key='val' after trim, got %v", obj["key"])
		}
	})
}

func TestCoerceArgumentsFloat64ToInteger(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	args := map[string]any{
		"count": 42.7,
	}

	warnings := CoerceArguments(schema, args)
	if len(warnings) == 0 {
		t.Error("expected coercion warning")
	}
	if args["count"] != 42 {
		t.Errorf("expected count=42 (truncated), got %v", args["count"])
	}
}

func TestCoercionWarningFields(t *testing.T) {
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
		t.Fatal("expected coercion warning")
	}
	w := warnings[0]
	if w.Key != "count" {
		t.Errorf("expected Key='count', got %q", w.Key)
	}
	if w.From != "string" {
		t.Errorf("expected From='string', got %q", w.From)
	}
	if w.To != "integer" {
		t.Errorf("expected To='integer', got %q", w.To)
	}
	if w.FromValue != "42" {
		t.Errorf("expected FromValue='42', got %q", w.FromValue)
	}
	if w.ToValue != "42" {
		t.Errorf("expected ToValue='42', got %q", w.ToValue)
	}
}

func TestCoercionWarningTruncation(t *testing.T) {
	// Direct unit test of truncateDisplay
	result := truncateDisplay("hello world this is a long string for testing", 10)
	if result != "hello worl..." {
		t.Errorf("expected truncated string, got %q", result)
	}
	short := truncateDisplay("hi", 10)
	if short != "hi" {
		t.Errorf("expected 'hi', got %q", short)
	}
}

func TestCoerceArgumentsStringToIntegerWithFloat(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	// "42.0" should parse as float then truncate to int
	args := map[string]any{
		"count": "42.0",
	}
	CoerceArguments(schema, args)
	if args["count"] != 42 {
		t.Errorf("expected count=42, got %v", args["count"])
	}
}
