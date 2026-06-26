package main

// ─── GroupBy ──────────────────────────────────────────────────────────────────
// Upstream: objectGroupBy() in objectGroupBy.ts

// GroupBy groups elements of a slice by a key function.
// Returns a map from key to slice of elements sharing that key.
func GroupBy[T any, K comparable](items []T, keyFn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, item := range items {
		key := keyFn(item)
		result[key] = append(result[key], item)
	}
	return result
}

// ─── Array Utilities ──────────────────────────────────────────────────────────
// Upstream: count(), uniq() in array.ts

// Count returns the number of elements in a slice that match the predicate.
func Count[T any](items []T, predicate func(T) bool) int {
	count := 0
	for _, item := range items {
		if predicate(item) {
			count++
		}
	}
	return count
}

// Uniq returns a deduplicated slice, preserving first-occurrence order.
func Uniq[T comparable](items []T) []T {
	seen := make(map[T]bool)
	var result []T
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

