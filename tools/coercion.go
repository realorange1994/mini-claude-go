package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// CoercionWarning records a type coercion that was applied.
type CoercionWarning struct {
	Key       string
	From      string
	To        string
	FromValue string
	ToValue   string
}

// truncateDisplay truncates a string to maxLen characters, appending "..." if truncated.
func truncateDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// formatValue converts an arbitrary value to a display string for warnings.
func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
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
			coerceToNumber(key, expectedType, argVal, args, &warnings)
		case "string":
			coerceToString(key, argVal, args, &warnings)
		case "boolean":
			coerceToBoolean(key, argVal, args, &warnings)
		case "array":
			coerceToArray(key, argVal, args, &warnings)
		case "object":
			coerceToObject(key, argVal, args, &warnings)
		}
	}
	return warnings
}

// coerceToNumber handles coercion to integer or number types.
func coerceToNumber(key, expectedType string, argVal any, args map[string]any, warnings *[]CoercionWarning) {
	switch v := argVal.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if expectedType == "integer" {
			if n, err := strconv.Atoi(trimmed); err == nil {
				args[key] = n
				*warnings = append(*warnings, CoercionWarning{
					Key: key, From: "string", To: expectedType,
					FromValue: truncateDisplay(v, 50),
					ToValue:   truncateDisplay(strconv.Itoa(n), 50),
				})
			} else if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
				args[key] = int(f)
				*warnings = append(*warnings, CoercionWarning{
					Key: key, From: "string", To: expectedType,
					FromValue: truncateDisplay(v, 50),
					ToValue:   truncateDisplay(strconv.Itoa(int(f)), 50),
				})
			}
		} else {
			if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
				args[key] = f
				*warnings = append(*warnings, CoercionWarning{
					Key: key, From: "string", To: expectedType,
					FromValue: truncateDisplay(v, 50),
					ToValue:   truncateDisplay(strconv.FormatFloat(f, 'f', -1, 64), 50),
				})
			}
		}
	case bool:
		// Bool -> Number: true -> 1, false -> 0
		if expectedType == "integer" {
			if v {
				args[key] = 1
			} else {
				args[key] = 0
			}
		} else {
			if v {
				args[key] = 1.0
			} else {
				args[key] = 0.0
			}
		}
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "boolean", To: expectedType,
			FromValue: strconv.FormatBool(v),
			ToValue:   formatValue(args[key]),
		})
	case float64:
		// float64 -> integer: truncate
		if expectedType == "integer" {
			args[key] = int(v)
			*warnings = append(*warnings, CoercionWarning{
				Key: key, From: "number", To: expectedType,
				FromValue: strconv.FormatFloat(v, 'f', -1, 64),
				ToValue:   strconv.Itoa(int(v)),
			})
		}
	}
}

// coerceToString handles coercion to string type.
func coerceToString(key string, argVal any, args map[string]any, warnings *[]CoercionWarning) {
	switch v := argVal.(type) {
	case int:
		s := strconv.Itoa(v)
		args[key] = s
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "integer", To: "string",
			FromValue: s, ToValue: s,
		})
	case float64:
		s := strconv.FormatFloat(v, 'f', -1, 64)
		args[key] = s
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "number", To: "string",
			FromValue: s, ToValue: s,
		})
	case bool:
		s := strconv.FormatBool(v)
		args[key] = s
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "boolean", To: "string",
			FromValue: s, ToValue: s,
		})
	}
}

// coerceToBoolean handles coercion to boolean type.
func coerceToBoolean(key string, argVal any, args map[string]any, warnings *[]CoercionWarning) {
	switch v := argVal.(type) {
	case string:
		lower := strings.ToLower(strings.TrimSpace(v))
		switch lower {
		case "true", "1", "yes", "on", "t":
			args[key] = true
			*warnings = append(*warnings, CoercionWarning{
				Key: key, From: "string", To: "boolean",
				FromValue: truncateDisplay(v, 50), ToValue: "true",
			})
		case "false", "0", "no", "off", "f":
			args[key] = false
			*warnings = append(*warnings, CoercionWarning{
				Key: key, From: "string", To: "boolean",
				FromValue: truncateDisplay(v, 50), ToValue: "false",
			})
		default:
			// Fall back to strconv.ParseBool for remaining cases (e.g., "T", "F")
			if b, err := strconv.ParseBool(v); err == nil {
				args[key] = b
				*warnings = append(*warnings, CoercionWarning{
					Key: key, From: "string", To: "boolean",
					FromValue: truncateDisplay(v, 50), ToValue: strconv.FormatBool(b),
				})
			}
		}
	case int:
		args[key] = v != 0
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "integer", To: "boolean",
			FromValue: strconv.Itoa(v), ToValue: strconv.FormatBool(v != 0),
		})
	case float64:
		args[key] = v != 0
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "number", To: "boolean",
			FromValue: strconv.FormatFloat(v, 'f', -1, 64), ToValue: strconv.FormatBool(v != 0),
		})
	}
}

// coerceToArray handles coercion to array type.
func coerceToArray(key string, argVal any, args map[string]any, warnings *[]CoercionWarning) {
	s, ok := argVal.(string)
	if !ok {
		return
	}
	trimmed := strings.TrimSpace(s)

	// Try to parse as JSON array
	var arr []any
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		args[key] = arr
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "string", To: "array",
			FromValue: truncateDisplay(s, 50),
			ToValue:   truncateDisplay(formatValue(arr), 50),
		})
		return
	}

	// Fallback: wrap in a single-element array
	args[key] = []any{s}
	*warnings = append(*warnings, CoercionWarning{
		Key: key, From: "string", To: "array",
		FromValue: truncateDisplay(s, 50),
		ToValue:   truncateDisplay(s, 50),
	})
}

// RemapFilePath copies file_path to path for file tools, so internal code
// that reads input["path"] works with the new official schema.
// This is a no-op if file_path is absent (backward compat).
func RemapFilePath(params map[string]any) {
	if fp, ok := params["file_path"].(string); ok && fp != "" {
		params["path"] = fp
	} else if _, exists := params["file_path"]; exists && params["file_path"] == nil {
		// file_path is explicitly null — set path to empty so downstream validation fires
		params["path"] = ""
	}
}

// RemapDirParam copies directory to dir for list_dir, so internal code
// that reads input["dir"] works with the official schema.
func RemapDirParam(params map[string]any) {
	if dir, ok := params["directory"].(string); ok && dir != "" {
		params["dir"] = dir
	}
}
func coerceToObject(key string, argVal any, args map[string]any, warnings *[]CoercionWarning) {
	s, ok := argVal.(string)
	if !ok {
		return
	}
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "{") {
		return
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
		args[key] = obj
		*warnings = append(*warnings, CoercionWarning{
			Key: key, From: "string", To: "object",
			FromValue: truncateDisplay(s, 50),
			ToValue:   truncateDisplay(formatValue(obj), 50),
		})
	}
}
