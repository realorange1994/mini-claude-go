package main

import (
	"unicode/utf8"
)

// deepSanitizeUTF8 recursively replaces invalid UTF-8 bytes with U+FFFD.
// Fast path: if all strings are valid, returns the original object unchanged.
// This matches openclacky's deep_sanitize_utf8 in message_history.rb.
func deepSanitizeUTF8(obj any) any {
	switch v := obj.(type) {
	case string:
		if utf8.ValidString(v) {
			return v
		}
		return sanitizeStringUTF8(v)
	case []any:
		if !containsDirtyUTF8Slice(v) {
			return v
		}
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = deepSanitizeUTF8(item)
		}
		return result
	case map[string]any:
		if !containsDirtyUTF8Map(v) {
			return v
		}
		result := make(map[string]any, len(v))
		for k, item := range v {
			result[k] = deepSanitizeUTF8(item)
		}
		return result
	default:
		return obj
	}
}

// sanitizeStringUTF8 replaces invalid UTF-8 bytes in a string with U+FFFD.
func sanitizeStringUTF8(s string) string {
	// Fast path: build a new string by iterating runes and replacing invalid ones
	var result []byte
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid byte — replace with U+FFFD
			result = append(result, "\xEF\xBF\xBD"...)
		} else {
			result = append(result, s[i:i+size]...)
		}
		i += size
	}
	return string(result)
}

// containsDirtyUTF8Slice returns true if any element in the slice has invalid UTF-8.
func containsDirtyUTF8Slice(items []any) bool {
	for _, item := range items {
		if isDirtyUTF8(item) {
			return true
		}
	}
	return false
}

// containsDirtyUTF8Map returns true if any value in the map has invalid UTF-8.
func containsDirtyUTF8Map(m map[string]any) bool {
	for _, v := range m {
		if isDirtyUTF8(v) {
			return true
		}
	}
	return false
}

// isDirtyUTF8 checks if a value contains invalid UTF-8 bytes.
func isDirtyUTF8(v any) bool {
	switch s := v.(type) {
	case string:
		return !utf8.ValidString(s)
	case []any:
		return containsDirtyUTF8Slice(s)
	case map[string]any:
		return containsDirtyUTF8Map(s)
	}
	return false
}
