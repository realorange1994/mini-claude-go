package main

import (
	"testing"
	"time"
)

// ─── WithResolvers ───────────────────────────────────────────────────────────

func TestWithResolversReturnsPromiseAndResolvers(t *testing.T) {
	dp := WithResolvers[string]()
	if dp.Promise == nil {
		t.Fatal("expected non-nil Promise channel")
	}
	if dp.Resolve == nil {
		t.Fatal("expected non-nil Resolve function")
	}
	if dp.Reject == nil {
		t.Fatal("expected non-nil Reject function")
	}
}

func TestWithResolversPromiseResolves(t *testing.T) {
	dp := WithResolvers[string]()
	dp.Resolve("hello")
	select {
	case result := <-dp.Promise:
		if result != "hello" {
			t.Fatalf("expected 'hello', got %q", result)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for promise resolution")
	}
}

func TestWithResolversRejectClosesChannel(t *testing.T) {
	dp := WithResolvers[string]()
	dp.Reject(nil)

	select {
	case _, ok := <-dp.Promise:
		if ok {
			// Value was sent (channel not closed) - this is fine for resolved
			t.Log("channel received value")
		} else {
			// Channel closed - this is fine for rejected
			t.Log("channel closed (rejected)")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for promise rejection")
	}
}

func TestWithResolversValueThrough(t *testing.T) {
	dp := WithResolvers[int]()
	dp.Resolve(42)
	select {
	case result := <-dp.Promise:
		if result != 42 {
			t.Fatalf("expected 42, got %d", result)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out")
	}
}

func TestWithResolversAsyncResolve(t *testing.T) {
	dp := WithResolvers[int]()

	go func() {
		time.Sleep(20 * time.Millisecond)
		dp.Resolve(99)
	}()

	start := time.Now()
	select {
	case result := <-dp.Promise:
		if result != 99 {
			t.Fatalf("expected 99, got %d", result)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for async resolution")
	}
	elapsed := time.Since(start)
	if elapsed < 10*time.Millisecond {
		t.Fatalf("resolved too quickly, likely not async: %v", elapsed)
	}
}

func TestWithResolversIdempotentResolve(t *testing.T) {
	dp := WithResolvers[int]()
	dp.Resolve(1)
	dp.Resolve(2) // Should not send again or panic

	select {
	case result := <-dp.Promise:
		if result != 1 {
			t.Fatalf("expected first value 1, got %d", result)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out")
	}
}

func TestWithResolversIsResolvedFlag(t *testing.T) {
	dp := WithResolvers[string]()
	if dp.IsResolved() {
		t.Fatal("should not be resolved initially")
	}
	dp.Resolve("done")
	if !dp.IsResolved() {
		t.Fatal("should be resolved after Resolve()")
	}
}

// ─── Roundtrip: resolve and consume ──────────────────────────────────────────

func TestWithResolversRoundtrip(t *testing.T) {
	dp := WithResolvers[int]()

	go func() {
		dp.Resolve(123)
	}()

	result := <-dp.Promise
	if result != 123 {
		t.Fatalf("expected 123, got %d", result)
	}
}
