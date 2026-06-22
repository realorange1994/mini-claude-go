package main

import (
	"context"
	"time"
)

// ─── Abort Utilities (MiMo-Code 4) ─────────────────────────────────────────
//
// GC-friendly timeout composition for context cancellation.
//
// MiMo-Code source: util/abort.ts (35 lines)

// AbortAfter creates a context with timeout.
// Uses direct function reference to avoid capturing surrounding scope.
func AbortAfter(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// AbortAfterAny creates a context that cancels on timeout or parent cancellation.
func AbortAfterAny(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// AbortWithSignals creates a context that cancels when any input context is cancelled.
func AbortWithSignals(contexts ...context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	for _, c := range contexts {
		go func(ctx context.Context) {
			<-ctx.Done()
			cancel()
		}(c)
	}

	return ctx, cancel
}

// FormatAbortStatus formats abort status for display.
func FormatAbortStatus(ctx context.Context) string {
	if ctx == nil {
		return "No context."
	}
	select {
	case <-ctx.Done():
		return "Cancelled: " + ctx.Err().Error()
	default:
		return "Active"
	}
}
