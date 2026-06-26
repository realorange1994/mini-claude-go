package main

// ─── Common Utilities ──────────────────────────────────────────────────────
//
// Shared utility functions to avoid code duplication.

// ContainsStr checks if a string slice contains a string.
func ContainsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Max returns the maximum of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MapKeys returns the keys of a map.
func MapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
