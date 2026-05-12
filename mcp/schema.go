// Package mcp implements lightweight JSON Schema validation for MCP tool inputs
// alongside the MCP client. Validates arguments against tool input schemas with helpful error messages.
package mcp

import (
	"fmt"
	"strings"
)

// ValidateSchema validates args against a JSON Schema (InputSchema from MCP tool definition).
// Returns an error if validation fails, nil if valid.
func ValidateSchema(args map[string]any, schema map[string]any) error {
	if schema == nil || len(schema) == 0 {
		return nil // No schema = no validation
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "" && schemaType != "object" {
		return fmt.Errorf("expected schema type 'object', got %q", schemaType)
	}

	// Required fields check
	if required, ok := schema["required"].([]any); ok {
		for _, req := range required {
			field, _ := req.(string)
			if field == "" {
				continue
			}
			if _, found := args[field]; !found {
				return fmt.Errorf("missing required parameter: %q", field)
			}
		}
	}

	// Property type validation
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil // No properties defined
	}

	for name, propSchema := range properties {
		propDef, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}

		val, present := args[name]
		if !present {
			continue // Not provided — skip (required check handles missing required params)
		}

		if err := validateProperty(name, val, propDef); err != nil {
			return err
		}
	}

	return nil
}

func validateProperty(name string, val any, schema map[string]any) error {
	propType, _ := schema["type"].(string)
	if propType == "" {
		return nil // No type constraint
	}

	if propType == "null" {
		if val != nil {
			return fmt.Errorf("parameter %q must be null, got %T", name, val)
		}
		return nil
	}

	if val == nil {
		// null value is always invalid for non-null types
		return fmt.Errorf("parameter %q must be a %s, got null", name, propType)
	}

	switch propType {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("parameter %q must be a string, got %T", name, val)
		}
		// MinLength/MaxLength check
		if s, ok := val.(string); ok {
			if minLen, ok := schema["minLength"].(float64); ok && float64(len(s)) < minLen {
				return fmt.Errorf("parameter %q must be at least %.0f characters, got %d", name, minLen, len(s))
			}
			if maxLen, ok := schema["maxLength"].(float64); ok && float64(len(s)) > maxLen {
				return fmt.Errorf("parameter %q must be at most %.0f characters, got %d", name, maxLen, len(s))
			}
		}

	case "number", "integer":
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("parameter %q must be a number, got %T", name, val)
		}
		if f, ok := val.(float64); ok {
			if minVal, ok := schema["minimum"].(float64); ok && f < minVal {
				return fmt.Errorf("parameter %q must be >= %.0f, got %v", name, minVal, f)
			}
			if maxVal, ok := schema["maximum"].(float64); ok && f > maxVal {
				return fmt.Errorf("parameter %q must be <= %.0f, got %v", name, maxVal, f)
			}
		}

	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("parameter %q must be a boolean, got %T", name, val)
		}

	case "array":
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("parameter %q must be an array, got %T", name, val)
		}
		// Validate array items
		if items, ok := schema["items"].(map[string]any); ok {
			for i, item := range arr {
				if err := validateProperty(fmt.Sprintf("%s[%d]", name, i), item, items); err != nil {
					return err
				}
			}
		}
		// Min/Max items
		if minItems, ok := schema["minItems"].(float64); ok && float64(len(arr)) < minItems {
			return fmt.Errorf("parameter %q must have at least %.0f items, got %d", name, minItems, len(arr))
		}
		if maxItems, ok := schema["maxItems"].(float64); ok && float64(len(arr)) > maxItems {
			return fmt.Errorf("parameter %q must have at most %.0f items, got %d", name, maxItems, len(arr))
		}

	case "object":
		obj, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("parameter %q must be an object, got %T", name, val)
		}
		// Recursively validate nested objects
		if items, ok := schema["properties"].(map[string]any); ok {
			return ValidateSchema(obj, map[string]any{
				"type":       "object",
				"properties": items,
				"required":   schema["required"],
			})
		}

	default:
		// Unknown type — skip
	}

	// Enum check (works for all types)
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		if !containsEnum(val, enum) {
			var vals []string
			for _, e := range enum {
				vals = append(vals, formatValue(e))
			}
			return fmt.Errorf("parameter %q must be one of [%s], got %s", name, strings.Join(vals, ", "), formatValue(val))
		}
	}

	return nil
}

func containsEnum(val any, enum []any) bool {
	for _, e := range enum {
		if e == val {
			return true
		}
		// String comparison for numbers
		if f1, ok1 := val.(float64); ok1 {
			if f2, ok2 := e.(float64); ok2 && f1 == f2 {
				return true
			}
		}
		if s1, ok1 := val.(string); ok1 {
			if s2, ok2 := e.(string); ok2 && s1 == s2 {
				return true
			}
		}
	}
	return false
}

func formatValue(v any) string {
	switch t := v.(type) {
	case string:
		return fmt.Sprintf("%q", t)
	case float64:
		if t == float64(int(t)) {
			return fmt.Sprintf("%d", int(t))
		}
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%v", t)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}
