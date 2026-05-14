package main

import "sync"

// DeferredPromise represents a deferred resolution pattern (like Promise.withResolvers).
// Ported from upstream withResolvers.ts.
type DeferredPromise[T any] struct {
	Promise chan T
	Resolve func(T)
	Reject  func(error)
	done    bool
	mu      sync.Mutex
}

// WithResolvers creates a deferred promise that can be resolved or rejected externally.
// Ported from upstream withResolvers.ts.
func WithResolvers[T any]() *DeferredPromise[T] {
	dp := &DeferredPromise[T]{}
	dp.Promise = make(chan T, 1)
	dp.Resolve = func(val T) {
		dp.mu.Lock()
		defer dp.mu.Unlock()
		if !dp.done {
			dp.done = true
			dp.Promise <- val
		}
	}
	dp.Reject = func(err error) {
		dp.mu.Lock()
		defer dp.mu.Unlock()
		if !dp.done {
			dp.done = true
			// For rejected promises, we can't send the error through the typed channel.
			// The caller should check for errors via a separate mechanism.
			// In practice, we close the channel to signal rejection.
			close(dp.Promise)
		}
	}
	return dp
}

// IsResolved returns whether the promise has been resolved or rejected.
func (dp *DeferredPromise[T]) IsResolved() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.done
}