package tools

import (
	"encoding/json"
	"strconv"
)

// CoercionWarning records a type coercion that was applied.
type CoercionWarning struct {
	Key  string
	From string
	To   string
}

// CoerceArguments coerces argument types to match the tool's input schema.
// LLMs often pass wrong types (e.g., string "123" for an integer field).
func CoerceArguments(schema map[string]any, args map[string]any) []CoercionWarning {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	var warnings []CoercionWarning
	for key, propVal := range props {
		prop, ok := propVal.(map[string]any)
		if !ok {
			continue
		}
		expectedType, _ := prop["type"].(string)
		argVal, exists := args[key]
		if !exists {
			continue
		}

		switch expectedType {
		case "integer", "number":
			if s, ok := argVal.(string); ok {
				if v, err := strconv.Atoi(s); err == nil {
					args[key] = v
					warnings = append(warnings, CoercionWarning{Key: key, From: "string", To: expectedType})
				} else if v, err := strconv.ParseFloat(s, 64); err == nil {
					args[key] = v
					warnings = append(warnings, CoercionWarning{Key: key, From: "string", To: expectedType})
				}
			}
		case "string":
			switch v := argVal.(type) {
			case int:
				args[key] = strconv.Itoa(v)
				warnings = append(warnings, CoercionWarning{Key: key, From: "int", To: "string"})
			case float64:
				args[key] = strconv.FormatFloat(v, 'f', -1, 64)
				warnings = append(warnings, CoercionWarning{Key: key, From: "float64", To: "string"})
			case bool:
				args[key] = strconv.FormatBool(v)
				warnings = append(warnings, CoercionWarning{Key: key, From: "bool", To: "string"})
			}
		case "boolean":
			if s, ok := argVal.(string); ok {
				if v, err := strconv.ParseBool(s); err == nil {
					args[key] = v
					warnings = append(warnings, CoercionWarning{Key: key, From: "string", To: "boolean"})
				}
			}
		case "array":
			if s, ok := argVal.(string); ok {
				var arr []any
				if err := json.Unmarshal([]byte(s), &arr); err == nil {
					args[key] = arr
					warnings = append(warnings, CoercionWarning{Key: key, From: "string", To: "array"})
				}
			}
		}
	}
	return warnings
}
