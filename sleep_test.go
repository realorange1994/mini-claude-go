package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ─── Sleep ───────────────────────────────────────────────────────────────────

func TestSleepResolvesAfterTimeout(t *testing.T) {
	start := time.Now()
	err := Sleep(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Fatalf("sleep returned too early: %v", elapsed)
	}
}

func TestSleepReturnsEarlyOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before sleep starts
	start := time.Now()
	err := Sleep(10*time.Second, ctx)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("sleep did not return early on cancelled context: %v", elapsed)
	}
}

func TestSleepReturnsOnMidSleepCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	err := Sleep(10*time.Second, ctx)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("sleep did not return on cancel: %v", elapsed)
	}
}

func TestSleepWorksNoContext(t *testing.T) {
	start := time.Now()
	err := Sleep(10 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	// Just verify it returns
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("sleep returned too early")
	}
}

// ─── SleepWithAbort ──────────────────────────────────────────────────────────

func TestSleepWithAbortResolvesAfterTimeout(t *testing.T) {
	start := time.Now()
	err := SleepWithAbort(30*time.Millisecond, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 20*time.Millisecond {
		t.Fatalf("sleep returned too early: %v", elapsed)
	}
}

func TestSleepWithAbortThrowsOnAlreadyAborted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SleepWithAbort(10*time.Second, ctx, true)
	if err == nil {
		t.Fatal("expected error on already-aborted context with throwOnAbort=true")
	}
}

func TestSleepWithAbortCustomError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	customErr := errors.New("custom abort")
	err := SleepWithAbort(10*time.Second, ctx, true, func() error { return customErr })
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "custom abort" {
		t.Fatalf("expected custom error, got: %v", err)
	}
}

func TestSleepWithAbortNoThrowOnAbort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SleepWithAbort(10*time.Second, ctx, false)
	if err != nil {
		t.Fatalf("expected no error with throwOnAbort=false, got: %v", err)
	}
}

func TestSleepWithAbortMidSleepAbort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := SleepWithAbort(10*time.Second, ctx, true)
	if err == nil {
		t.Fatal("expected error on abort")
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("abort did not trigger promptly: %v", elapsed)
	}
}

// ─── WithTimeout ─────────────────────────────────────────────────────────────

func TestWithTimeoutResolvesWhenFast(t *testing.T) {
	result, err := WithTimeout(func() (int, error) {
		return 42, nil
	}, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result != 42 {
		t.Fatalf("expected 42, got %d", result)
	}
}

func TestWithTimeoutTimesOut(t *testing.T) {
	start := time.Now()
	_, err := WithTimeout(func() (int, error) {
		time.Sleep(5 * time.Second)
		return 42, nil
	}, 30*time.Millisecond, "operation timed out")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("withTimeout did not return promptly: %v", elapsed)
	}
}

func TestWithTimeoutPropagatesErrors(t *testing.T) {
	innerErr := fmt.Errorf("inner error")
	_, err := WithTimeout(func() (int, error) {
		return 0, innerErr
	}, 1*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "inner error" {
		t.Fatalf("expected inner error, got: %v", err)
	}
}

func TestWithTimeoutCustomMessage(t *testing.T) {
	_, err := WithTimeout(func() (int, error) {
		time.Sleep(5 * time.Second)
		return 42, nil
	}, 30*time.Millisecond, "my custom timeout")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err.Error() != "my custom timeout" {
		t.Fatalf("expected custom message, got: %v", err)
	}
}

func TestWithTimeoutDefaultMessage(t *testing.T) {
	_, err := WithTimeout(func() (int, error) {
		time.Sleep(5 * time.Second)
		return 42, nil
	}, 30*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err.Error() != "operation timed out" {
		t.Fatalf("expected default message, got: %v", err)
	}
}

// ─── Roundtrip: sleep followed by work ───────────────────────────────────────

func TestSleepThenWorkPattern(t *testing.T) {
	start := time.Now()

	err := Sleep(20 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate work
	result, err := WithTimeout(func() (string, error) {
		return "done", nil
	}, 100*time.Millisecond)
	if err != nil || result != "done" {
		t.Fatalf("work failed: %v, result: %v", err, result)
	}

	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Fatalf("pattern completed too early: %v", elapsed)
	}
}
