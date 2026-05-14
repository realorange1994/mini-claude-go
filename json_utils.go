package main

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// stripBOM removes a UTF-8 Byte Order Mark (U+FEFF) prefix from a string.
// PowerShell 5.x adds BOM to UTF-8 files, which breaks JSON.parse.
func stripBOM(s string) string {
	if len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF {
		return s[3:]
	}
	// Also handle the Unicode BOM character (U+FEFF) as a rune
	if r, _ := utf8.DecodeRuneInString(s); r == '\uFEFF' {
		return s[utf8.RuneLen('\uFEFF'):]
	}
	return s
}

// safeParseJSON parses a JSON string, returning nil for invalid or empty input.
// Ported from upstream json.ts safeParseJSON.
func safeParseJSON(jsonStr string) interface{} {
	if jsonStr == "" {
		return nil
	}
	cleaned := stripBOM(jsonStr)
	var result interface{}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil
	}
	return result
}

// safeParseJSONC parses JSON with comments (JSONC) by stripping comments
// before parsing. Returns nil for invalid or empty input.
// Ported from upstream json.ts safeParseJSONC.
func safeParseJSONC(jsonStr string) interface{} {
	if jsonStr == "" {
		return nil
	}
	cleaned := stripBOM(jsonStr)
	stripped := stripJSONComments(cleaned)
	stripped = stripTrailingCommas(stripped)
	var result interface{}
	if err := json.Unmarshal([]byte(stripped), &result); err != nil {
		return nil
	}
	return result
}

// parseJSONL parses JSON Lines format (one JSON object per line).
// Skips malformed lines. Ported from upstream json.ts parseJSONLString.
func parseJSONL(data string) []interface{} {
	if data == "" {
		return nil
	}
	stripped := stripBOM(data)
	var results []interface{}
	for _, line := range strings.Split(stripped, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			// Skip malformed lines
			continue
		}
		results = append(results, obj)
	}
	return results
}

// addItemToJSONCArray adds an item to a JSON array string, handling
// empty content and non-array content. Ported from upstream addItemToJSONCArray.
func addItemToJSONCArray(content string, newItem interface{}) string {
	// If content is empty or whitespace, create a new array
	if strings.TrimSpace(content) == "" {
		arr := []interface{}{newItem}
		encoded, err := json.MarshalIndent(arr, "", "    ")
		if err != nil {
			return ""
		}
		return string(encoded)
	}

	cleaned := stripBOM(content)

	// Try to parse as JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		// Parsing failed, create a new array with the item
		arr := []interface{}{newItem}
		encoded, _ := json.MarshalIndent(arr, "", "    ")
		return string(encoded)
	}

	// If it's an array, append the new item
	if arr, ok := parsed.([]interface{}); ok {
		arr = append(arr, newItem)
		encoded, err := json.MarshalIndent(arr, "", "    ")
		if err != nil {
			return ""
		}
		return string(encoded)
	}

	// If it's not an array, create a new array with the item
	arr := []interface{}{newItem}
	encoded, _ := json.MarshalIndent(arr, "", "    ")
	return string(encoded)
}

// stripJSONComments removes single-line (//) and block (/* */) comments
// from a JSON string, preserving strings that contain comment-like patterns.
func stripJSONComments(input string) string {
	var result strings.Builder
	result.Grow(len(input))
	inString := false
	i := 0

	for i < len(input) {
		ch := input[i]

		if inString {
			result.WriteByte(ch)
			if ch == '\\' && i+1 < len(input) {
				i++
				result.WriteByte(input[i])
			} else if ch == '"' {
				inString = false
			}
			i++
			continue
		}

		if ch == '"' {
			inString = true
			result.WriteByte(ch)
			i++
			continue
		}

		// Single-line comment
		if ch == '/' && i+1 < len(input) && input[i+1] == '/' {
			// Skip until newline
			for i < len(input) && input[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment
		if ch == '/' && i+1 < len(input) && input[i+1] == '*' {
			i += 2
			for i < len(input) {
				if input[i] == '*' && i+1 < len(input) && input[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// stripTrailingCommas removes trailing commas before ] or } in JSON.
func stripTrailingCommas(input string) string {
	var result strings.Builder
	result.Grow(len(input))
	inString := false
	i := 0

	for i < len(input) {
		ch := input[i]

		if inString {
			result.WriteByte(ch)
			if ch == '\\' && i+1 < len(input) {
				i++
				result.WriteByte(input[i])
			} else if ch == '"' {
				inString = false
			}
			i++
			continue
		}

		if ch == '"' {
			inString = true
			result.WriteByte(ch)
			i++
			continue
		}

		// Trailing comma before ] or }
		if ch == ',' {
			// Look ahead for ] or } (skipping whitespace)
			j := i + 1
			for j < len(input) && (input[j] == ' ' || input[j] == '\n' || input[j] == '\r' || input[j] == '\t') {
				j++
			}
			if j < len(input) && (input[j] == ']' || input[j] == '}') {
				// Skip the comma
				i++
				continue
			}
		}

		result.WriteByte(ch)
		i++
	}

	return result.String()
}