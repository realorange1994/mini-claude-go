package main

import (
	"context"
	"fmt"
	"time"
)

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