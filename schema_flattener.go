package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ─── Schema Flattener (MiMo-Code 5) ────────────────────────────────────────
//
// Flattens anyOf/oneOf discriminated union schemas into flat type:"object" schemas.
// Makes complex tool schemas compatible with strict providers.
//
// MiMo-Code source: provider/transform.ts (1130-1322 lines)

// SchemaFlattener flattens discriminated union schemas.
type SchemaFlattener struct{}

// NewSchemaFlattener creates a new schema flattener.
func NewSchemaFlattener() *SchemaFlattener {
	return &SchemaFlattener{}
}

// FlattenSchema flattens a schema that uses anyOf/oneOf.
func (f *SchemaFlattener) FlattenSchema(schema map[string]any) map[string]any {
	// Check for anyOf/oneOf at root level
	if anyOf, ok := schema["anyOf"].([]any); ok {
		return f.flattenUnion(schema, anyOf, "anyOf")
	}
	if oneOf, ok := schema["oneOf"].([]any); ok {
		return f.flattenUnion(schema, oneOf, "oneOf")
	}

	// Check nested schemas
	result := make(map[string]any)
	for k, v := range schema {
		if k == "properties" {
			if props, ok := v.(map[string]any); ok {
				flattenedProps := make(map[string]any)
				for pk, pv := range props {
					if propMap, ok := pv.(map[string]any); ok {
						flattenedProps[pk] = f.FlattenSchema(propMap)
					} else {
						flattenedProps[pk] = pv
					}
				}
				result[k] = flattenedProps
			} else {
				result[k] = v
			}
		} else {
			result[k] = v
		}
	}

	return result
}

// flattenUnion flattens an anyOf/oneOf union into a single object.
func (f *SchemaFlattener) flattenUnion(root map[string]any, variants []any, unionType string) map[string]any {
	result := map[string]any{
		"type": "object",
	}

	mergedProps := make(map[string]any)
	var required []string
	discriminator := ""
	discriminatorValues := make([]string, 0)

	for _, variant := range variants {
		if variantMap, ok := variant.(map[string]any); ok {
			// Extract properties
			if props, ok := variantMap["properties"].(map[string]any); ok {
				for k, v := range props {
					mergedProps[k] = v
				}
			}

			// Extract required
			if req, ok := variantMap["required"].([]any); ok {
				for _, r := range req {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}

			// Look for discriminator
			if props, ok := variantMap["properties"].(map[string]any); ok {
				for propName, propDef := range props {
					if propMap, ok := propDef.(map[string]any); ok {
						if enum, ok := propMap["enum"].([]any); ok && len(enum) == 1 {
							discriminator = propName
							if val, ok := enum[0].(string); ok {
								discriminatorValues = append(discriminatorValues, val)
							}
						}
					}
				}
			}
		}
	}

	// Set merged properties
	if len(mergedProps) > 0 {
		result["properties"] = mergedProps
	}

	// Set required (unique)
	if len(required) > 0 {
		result["required"] = uniqueStringsSchema(required)
	}

	// Add discriminator info to description
	if discriminator != "" && len(discriminatorValues) > 0 {
		desc := fmt.Sprintf("Discriminator: %s. Valid values: %s", discriminator, strings.Join(discriminatorValues, ", "))
		if existing, ok := root["description"].(string); ok && existing != "" {
			desc = existing + " " + desc
		}
		result["description"] = desc
	}

	return result
}

// SanitizeForGemini sanitizes a schema for Gemini compatibility.
func (f *SchemaFlattener) SanitizeForGemini(schema map[string]any) map[string]any {
	result := make(map[string]any)

	for k, v := range schema {
		switch k {
		case "enum":
			// Convert integer enums to strings
			if enum, ok := v.([]any); ok {
				strEnum := make([]any, len(enum))
				for i, e := range enum {
					strEnum[i] = fmt.Sprintf("%v", e)
				}
				result[k] = strEnum
			} else {
				result[k] = v
			}

		case "required":
			// Filter required to only include fields in properties
			if req, ok := v.([]any); ok {
				props, _ := schema["properties"].(map[string]any)
				if props != nil {
					var filtered []string
					for _, r := range req {
						if s, ok := r.(string); ok {
							if _, exists := props[s]; exists {
								filtered = append(filtered, s)
							}
						}
					}
					result[k] = filtered
				} else {
					result[k] = v
				}
			} else {
				result[k] = v
			}

		case "properties":
			// Recursively sanitize nested properties
			if props, ok := v.(map[string]any); ok {
				sanitizedProps := make(map[string]any)
				for pk, pv := range props {
					if propMap, ok := pv.(map[string]any); ok {
						sanitizedProps[pk] = f.SanitizeForGemini(propMap)
					} else {
						sanitizedProps[pk] = pv
					}
				}
				result[k] = sanitizedProps
			} else {
				result[k] = v
			}

		case "items":
			// Ensure arrays have items.type
			if items, ok := v.(map[string]any); ok {
				if _, hasType := items["type"]; !hasType {
					items["type"] = "string"
				}
				result[k] = f.SanitizeForGemini(items)
			} else {
				result[k] = v
			}

		default:
			result[k] = v
		}
	}

	return result
}

// uniqueStringsSchema returns a new slice with unique strings.
func uniqueStringsSchema(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// FlattenToolInputSchema flattens a tool's input schema for strict providers.
func FlattenToolInputSchema(schema map[string]any) map[string]any {
	flattener := NewSchemaFlattener()
	return flattener.FlattenSchema(schema)
}

// SanitizeToolSchemaForGemini sanitizes a tool schema for Gemini.
func SanitizeToolSchemaForGemini(schema map[string]any) map[string]any {
	flattener := NewSchemaFlattener()
	return flattener.SanitizeForGemini(schema)
}

// SchemaToJSON converts a schema to JSON string.
func SchemaToJSON(schema map[string]any) string {
	data, _ := json.MarshalIndent(schema, "", "  ")
	return string(data)
}
