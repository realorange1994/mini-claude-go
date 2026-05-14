package main

import "sync"

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
