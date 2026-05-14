package main

// Intersperse inserts a separator between elements of a slice.
// The separator function receives the index (1-based position between elements).
// Upstream: intersperse() in array.ts
func Intersperse[T any](items []T, separator func(int) T) []T {
	if len(items) == 0 {
		return []T{}
	}
	result := make([]T, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, separator(i))
		}
		result = append(result, item)
	}
	return result
}

// Count returns the number of elements in a slice that match the predicate.
// Upstream: count() in array.ts
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
// Upstream: uniq() in array.ts
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
