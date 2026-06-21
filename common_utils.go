package main

import (
	"bytes"
	"strings"
	"sync"
)

// ─── Common Utilities ──────────────────────────────────────────────────────
//
// Shared utility functions to avoid code duplication.

// BufferPool is a pool of bytes.Buffer for reducing memory allocations.
var BufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// GetBuffer gets a buffer from the pool.
func GetBuffer() *bytes.Buffer {
	return BufferPool.Get().(*bytes.Buffer)
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	BufferPool.Put(buf)
}

// StringPool is a pool of strings.Builder for reducing memory allocations.
var StringPool = sync.Pool{
	New: func() any {
		return new(strings.Builder)
	},
}

// GetStringBuilder gets a strings.Builder from the pool.
func GetStringBuilder() *strings.Builder {
	return StringPool.Get().(*strings.Builder)
}

// PutStringBuilder returns a strings.Builder to the pool.
func PutStringBuilder(sb *strings.Builder) {
	sb.Reset()
	StringPool.Put(sb)
}

// ContainsStr checks if a string slice contains a string.
func ContainsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ContainsString checks if a string contains a substring.
func ContainsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// SearchString searches for a substring in a string.
func SearchString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Min returns the minimum of two integers.
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Clamp clamps an integer between min and max.
func Clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// SliceContains checks if a slice contains an element.
func SliceContains[T comparable](slice []T, element T) bool {
	for _, item := range slice {
		if item == element {
			return true
		}
	}
	return false
}

// SliceUnique returns a new slice with unique elements.
func SliceUnique[T comparable](slice []T) []T {
	seen := make(map[T]bool)
	var result []T
	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// MapKeys returns the keys of a map.
func MapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// MapValues returns the values of a map.
func MapValues[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}
