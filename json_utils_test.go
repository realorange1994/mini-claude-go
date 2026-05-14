package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// ─── safeParseJSON ───────────────────────────────────────────────────────
// Ported from upstream json.test.ts

func TestSafeParseJSONValidObject(t *testing.T) {
	result := safeParseJSON(`{"a":1}`)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("expected a=1, got %v", m["a"])
	}
}

func TestSafeParseJSONValidArray(t *testing.T) {
	result := safeParseJSON(`[1,2,3]`)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected slice, got %T", result)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr))
	}
}

func TestSafeParseJSONStringValue(t *testing.T) {
	result := safeParseJSON(`"hello"`)
	if result != "hello" {
		t.Errorf("expected 'hello', got %v", result)
	}
}

func TestSafeParseJSONNumberValue(t *testing.T) {
	result := safeParseJSON(`42`)
	if result.(float64) != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestSafeParseJSONBooleanValue(t *testing.T) {
	result := safeParseJSON(`true`)
	if result != true {
		t.Errorf("expected true, got %v", result)
	}
}

func TestSafeParseJSONNullValue(t *testing.T) {
	result := safeParseJSON(`null`)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSafeParseJSONInvalidReturnsNil(t *testing.T) {
	result := safeParseJSON("{bad}")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestSafeParseJSONEmptyStringReturnsNil(t *testing.T) {
	result := safeParseJSON("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestSafeParseJSONWithBOM(t *testing.T) {
	result := safeParseJSON("\uFEFF" + `{"a":1}`)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map with BOM, got %T", result)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("expected a=1 with BOM, got %v", m["a"])
	}
}

func TestSafeParseJSONNestedObjects(t *testing.T) {
	input := `{"a":{"b":{"c":1}}}`
	result := safeParseJSON(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	inner, ok := m["a"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", m["a"])
	}
	inner2, ok := inner["b"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected deeply nested map, got %T", inner["b"])
	}
	if inner2["c"].(float64) != 1 {
		t.Errorf("expected c=1, got %v", inner2["c"])
	}
}

// ─── safeParseJSONC ──────────────────────────────────────────────────────
// Ported from upstream json.test.ts

func TestSafeParseJSONCStandardJSON(t *testing.T) {
	result := safeParseJSONC(`{"a":1}`)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("expected a=1, got %v", m["a"])
	}
}

func TestSafeParseJSONCSingleLineComments(t *testing.T) {
	input := "{\n// comment\n\"a\":1\n}"
	result := safeParseJSONC(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map with comments, got %T", result)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("expected a=1 with comments, got %v", m["a"])
	}
}

func TestSafeParseJSONCBlockComments(t *testing.T) {
	input := "{\n/* comment */\n\"a\":1\n}"
	result := safeParseJSONC(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map with block comments, got %T", result)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("expected a=1 with block comments, got %v", m["a"])
	}
}

func TestSafeParseJSONCTrailingCommas(t *testing.T) {
	result := safeParseJSONC(`{"a":1,}`)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map with trailing comma, got %T", result)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("expected a=1 with trailing comma, got %v", m["a"])
	}
}

func TestSafeParseJSONCEmptyStringReturnsNil(t *testing.T) {
	result := safeParseJSONC("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

// ─── parseJSONL ──────────────────────────────────────────────────────────
// Ported from upstream json.test.ts

func TestParseJSONLMultipleLines(t *testing.T) {
	result := parseJSONL(`{"a":1}` + "\n" + `{"b":2}`)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	m1, ok := result[0].(map[string]interface{})
	if !ok || m1["a"].(float64) != 1 {
		t.Errorf("first line: expected a=1, got %v", result[0])
	}
	m2, ok := result[1].(map[string]interface{})
	if !ok || m2["b"].(float64) != 2 {
		t.Errorf("second line: expected b=2, got %v", result[1])
	}
}

func TestParseJSONLEmptyString(t *testing.T) {
	result := parseJSONL("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestParseJSONLSingleLine(t *testing.T) {
	result := parseJSONL(`{"a":1}`)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, ok := result[0].(map[string]interface{})
	if !ok || m["a"].(float64) != 1 {
		t.Errorf("expected a=1, got %v", result[0])
	}
}

func TestParseJSONLSkipsMalformedLines(t *testing.T) {
	result := parseJSONL(`{"a":1}` + "\n" + "badline\n" + `{"b":2}`)
	if len(result) != 2 {
		t.Fatalf("expected 2 results (skipping malformed), got %d", len(result))
	}
}

// ─── addItemToJSONCArray ─────────────────────────────────────────────────
// Ported from upstream json.test.ts

func TestAddItemToJSONCArrayExistingArray(t *testing.T) {
	result := addItemToJSONCArray(`["a","b"]`, "c")
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 3 || parsed[0] != "a" || parsed[1] != "b" || parsed[2] != "c" {
		t.Errorf("expected [a,b,c], got %v", parsed)
	}
}

func TestAddItemToJSONCArrayEmptyArray(t *testing.T) {
	result := addItemToJSONCArray("[]", "item")
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 1 || parsed[0] != "item" {
		t.Errorf("expected [item], got %v", parsed)
	}
}

func TestAddItemToJSONCArrayEmptyContent(t *testing.T) {
	result := addItemToJSONCArray("", "first")
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 1 || parsed[0] != "first" {
		t.Errorf("expected [first], got %v", parsed)
	}
}

func TestAddItemToJSONCArrayObjectItem(t *testing.T) {
	result := addItemToJSONCArray("[]", map[string]interface{}{"key": "val"})
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 1 {
		t.Errorf("expected 1 element, got %d", len(parsed))
	}
	m, ok := parsed[0].(map[string]interface{})
	if !ok || m["key"] != "val" {
		t.Errorf("expected {key:val}, got %v", parsed[0])
	}
}

func TestAddItemToJSONCArrayNonArrayContent(t *testing.T) {
	result := addItemToJSONCArray(`{"a":1}`, "item")
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(parsed) != 1 || parsed[0] != "item" {
		t.Errorf("expected [item], got %v", parsed)
	}
}

// ─── Invariants & edge cases ─────────────────────────────────────────────

func TestSafeParseJSONIdempotent(t *testing.T) {
	// safeParseJSON(safeParseJSON(x)) should be nil for any string input
	// because the result is not a JSON string (it's a parsed value)
	input := `{"key":"value"}`
	result := safeParseJSON(input)
	// Re-serializing and re-parsing should roundtrip
	serialized, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to re-serialize: %v", err)
	}
	result2 := safeParseJSON(string(serialized))
	if !reflect.DeepEqual(result, result2) {
		t.Errorf("roundtrip failed: %v != %v", result, result2)
	}
}

func TestParseJSONLWithBOM(t *testing.T) {
	result := parseJSONL("\uFEFF" + `{"a":1}`)
	if len(result) != 1 {
		t.Fatalf("expected 1 result with BOM, got %d", len(result))
	}
}

func TestStripBOMNoBOM(t *testing.T) {
	result := stripBOM("hello")
	if result != "hello" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

func TestStripBOMWithBOM(t *testing.T) {
	result := stripBOM("\uFEFFhello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestStripBOMWithUTF8BOMBytes(t *testing.T) {
	result := stripBOM("\xEF\xBB\xBFhello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestSafeParseJSONCStringsWithComments(t *testing.T) {
	// Ensure strings containing // are not stripped
	input := `{"url":"http://example.com"}`
	result := safeParseJSONC(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["url"] != "http://example.com" {
		t.Errorf("expected url preserved, got %v", m["url"])
	}
}
