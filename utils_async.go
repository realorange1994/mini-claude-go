package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ─── Sequential ───

// Sequential runs functions sequentially, returning results in order.
// This is the Go equivalent of TypeScript's sequential() that chains
// async operations one after another, collecting results.
// Upstream: sequential() in sequential.ts
func Sequential[T any, R any](items []T, fn func(T) R) []R {
	results := make([]R, len(items))
	for i, item := range items {
		results[i] = fn(item)
	}
	return results
}

// SequentialConcurrent runs functions sequentially with a concurrency limit.
// Results are returned in the same order as the input.
// Upstream: sequential with concurrency control
func SequentialConcurrent[T any, R any](items []T, limit int, fn func(T) R) []R {
	results := make([]R, len(items))
	if len(items) == 0 {
		return results
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, item := range items {
		wg.Add(1)
		go func(idx int, val T) {
			defer wg.Done()
			sem <- struct{}{}
			result := fn(val)
			mu.Lock()
			results[idx] = result
			mu.Unlock()
			<-sem
		}(i, item)
	}
	wg.Wait()
	return results
}

// ─── WithResolvers ───

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

// ─── Sleep ───

// Sleep pauses for the specified duration. If a context is provided and
// is cancelled before the duration elapses, Sleep returns early without error.
// Upstream: sleep() in sleep.ts
func Sleep(duration time.Duration, ctx ...context.Context) error {
	if len(ctx) > 0 && ctx[0] != nil {
		select {
		case <-time.After(duration):
			return nil
		case <-ctx[0].Done():
			return nil
		}
	}
	<-time.After(duration)
	return nil
}

// SleepWithAbort pauses for the specified duration with abort behavior.
// If throwOnAbort is true, returns an error when the context is cancelled.
// Upstream: sleep() in sleep.ts with throwOnAbort option
func SleepWithAbort(duration time.Duration, ctx context.Context, throwOnAbort bool, abortError ...func() error) error {
	if ctx == nil {
		<-time.After(duration)
		return nil
	}

	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		if throwOnAbort {
			if len(abortError) > 0 {
				return abortError[0]()
			}
			return fmt.Errorf("aborted")
		}
		return nil
	}
}

// WithTimeout wraps a function call with a timeout.
// Returns the function's result, or an error if the timeout expires.
// Upstream: withTimeout() in sleep.ts
func WithTimeout[R any](fn func() (R, error), timeout time.Duration, timeoutMsg ...string) (R, error) {
	var zero R
	resultCh := make(chan R, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := fn()
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return zero, err
	case <-time.After(timeout):
		msg := "operation timed out"
		if len(timeoutMsg) > 0 {
			msg = timeoutMsg[0]
		}
		return zero, fmt.Errorf(msg)
	}
}

// ─── Generators ───

// Generator represents a channel-based iterator.
// Yielded values are sent on the channel; closing signals completion.
type Generator[T any] <-chan T

// FromArray creates a generator that yields all elements of a slice.
// Upstream: fromArray() in generators.ts
func FromArray[T any](items []T) Generator[T] {
	ch := make(chan T)
	go func() {
		defer close(ch)
		for _, item := range items {
			ch <- item
		}
	}()
	return ch
}

// ToArray collects all values from a generator into a slice.
// Upstream: toArray() in generators.ts
func ToArray[T any](gen Generator[T]) []T {
	var result []T
	for item := range gen {
		result = append(result, item)
	}
	return result
}

// LastX returns the last value yielded by a generator.
// Returns an error if the generator yields nothing.
// Upstream: lastX() in generators.ts
func LastX[T any](gen Generator[T]) (T, error) {
	var last T
	found := false
	for item := range gen {
		last = item
		found = true
	}
	if !found {
		return last, fmt.Errorf("no items in generator")
	}
	return last, nil
}

// All merges multiple generators into a single generator, yielding values
// from all generators concurrently (order depends on scheduling).
// Upstream: all() in generators.ts
func All[T any](generators []Generator[T]) Generator[T] {
	ch := make(chan T)
	go func() {
		defer close(ch)
		done := make(chan struct{})
		remaining := len(generators)
		for _, gen := range generators {
			go func(g Generator[T]) {
				for item := range g {
					ch <- item
				}
				done <- struct{}{}
			}(gen)
		}
		for remaining > 0 {
			<-done
			remaining--
		}
	}()
	return ch
}
