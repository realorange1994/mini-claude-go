package main

import (
	"sync"
)

// ─── Bounded Async Queue (MiMo-Code 5) ─────────────────────────────────────
//
// Memory-safe bounded queue with drop-oldest backpressure.
//
// MiMo-Code source: util/queue.ts (60 lines)

// BoundedQueue implements a bounded queue with drop-oldest semantics.
type BoundedQueue struct {
	mu       sync.Mutex
	items    []any
	capacity int
	dropped  int
	waitCh   chan struct{}
}

// NewBoundedQueue creates a new bounded queue.
func NewBoundedQueue(capacity int) *BoundedQueue {
	if capacity <= 0 {
		capacity = 1000
	}
	return &BoundedQueue{
		items:    make([]any, 0, capacity),
		capacity: capacity,
		waitCh:   make(chan struct{}, 1),
	}
}

// Push adds an item to the queue. If full, drops the oldest item.
func (q *BoundedQueue) Push(item any) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) >= q.capacity {
		// Drop oldest
		q.items = q.items[1:]
		q.dropped++
	}

	q.items = append(q.items, item)

	// Signal waiting consumers
	select {
	case q.waitCh <- struct{}{}:
	default:
	}
}

// Pop removes and returns the oldest item. Blocks if empty.
func (q *BoundedQueue) Pop() (any, bool) {
	q.mu.Lock()

	for len(q.items) == 0 {
		q.mu.Unlock()
		<-q.waitCh
		q.mu.Lock()
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.mu.Unlock()

	return item, true
}

// TryPop removes and returns the oldest item. Returns false if empty.
func (q *BoundedQueue) TryPop() (any, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil, false
	}

	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

// Len returns the number of items in the queue.
func (q *BoundedQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Dropped returns the number of dropped items.
func (q *BoundedQueue) Dropped() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dropped
}

// Clear removes all items from the queue.
func (q *BoundedQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = make([]any, 0, q.capacity)
}
