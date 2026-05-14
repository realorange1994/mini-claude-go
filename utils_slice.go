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
// Upstream: intersperse(), count(), uniq() in array.ts

// Intersperse inserts a separator between elements of a slice.
// The separator function receives the index (1-based position between elements).
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

// ─── Set Utilities ────────────────────────────────────────────────────────────
// Upstream: difference(), intersects(), every(), union() in set.ts

// SetDifference returns elements in a that are not in b.
func SetDifference[T comparable](a, b map[T]bool) map[T]bool {
	result := make(map[T]bool)
	for item := range a {
		if !b[item] {
			result[item] = true
		}
	}
	return result
}

// SetIntersects returns true if a and b share any elements.
func SetIntersects[T comparable](a, b map[T]bool) bool {
	for item := range a {
		if b[item] {
			return true
		}
	}
	return false
}

// SetEvery returns true if every element of a is also in b (a is subset of b).
func SetEvery[T comparable](a, b map[T]bool) bool {
	for item := range a {
		if !b[item] {
			return false
		}
	}
	return true
}

// SetUnion returns the union of a and b.
func SetUnion[T comparable](a, b map[T]bool) map[T]bool {
	result := make(map[T]bool)
	for item := range a {
		result[item] = true
	}
	for item := range b {
		result[item] = true
	}
	return result
}
