package main

// CircularBuffer is a fixed-size buffer that evicts the oldest entries when full.
// Ported from upstream TypeScript CircularBuffer.ts.
//
// Usage:
//
//	buf := NewCircularBuffer[int](5)
//	buf.Add(1)
//	buf.Add(2)
//	items := buf.ToArray() // [1, 2]
type CircularBuffer[T any] struct {
	data     []T
	capacity int
}

// NewCircularBuffer creates a new circular buffer with the given capacity.
func NewCircularBuffer[T any](capacity int) *CircularBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &CircularBuffer[T]{
		data:     make([]T, 0, capacity),
		capacity: capacity,
	}
}

// Add appends an item to the buffer, evicting the oldest if at capacity.
func (b *CircularBuffer[T]) Add(item T) {
	if len(b.data) >= b.capacity {
		// Evict the oldest item (shift left)
		copy(b.data, b.data[1:])
		b.data[len(b.data)-1] = item
	} else {
		b.data = append(b.data, item)
	}
}

// AddAll adds multiple items to the buffer.
func (b *CircularBuffer[T]) AddAll(items []T) {
	for _, item := range items {
		b.Add(item)
	}
}

// Length returns the number of items currently in the buffer.
func (b *CircularBuffer[T]) Length() int {
	return len(b.data)
}

// ToArray returns a copy of all items in the buffer in insertion order.
func (b *CircularBuffer[T]) ToArray() []T {
	result := make([]T, len(b.data))
	copy(result, b.data)
	return result
}

// GetRecent returns the last N items from the buffer.
// If N is greater than the number of items, returns all items.
func (b *CircularBuffer[T]) GetRecent(n int) []T {
	if n >= len(b.data) {
		return b.ToArray()
	}
	start := len(b.data) - n
	result := make([]T, n)
	copy(result, b.data[start:])
	return result
}

// Clear removes all items from the buffer.
func (b *CircularBuffer[T]) Clear() {
	b.data = b.data[:0]
}
