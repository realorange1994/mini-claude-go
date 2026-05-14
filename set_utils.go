package main

// SetDifference returns elements in a that are not in b.
// Upstream: difference() in set.ts
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
// Upstream: intersects() in set.ts
func SetIntersects[T comparable](a, b map[T]bool) bool {
	for item := range a {
		if b[item] {
			return true
		}
	}
	return false
}

// SetEvery returns true if every element of a is also in b (a is subset of b).
// Upstream: every() in set.ts
func SetEvery[T comparable](a, b map[T]bool) bool {
	for item := range a {
		if !b[item] {
			return false
		}
	}
	return true
}

// SetUnion returns the union of a and b.
// Upstream: union() in set.ts
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
