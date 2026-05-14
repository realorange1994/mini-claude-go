package main

import "fmt"

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
