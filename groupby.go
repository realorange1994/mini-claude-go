package main

// GroupBy groups elements of a slice by a key function.
// Returns a map from key to slice of elements sharing that key.
// Upstream: objectGroupBy() in objectGroupBy.ts
func GroupBy[T any, K comparable](items []T, keyFn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, item := range items {
		key := keyFn(item)
		result[key] = append(result[key], item)
	}
	return result
}
