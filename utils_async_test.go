package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// ─── Sequential ───

func TestSequentialReturnsSameResult(t *testing.T) {
	result := Sequential([]int{5}, func(x int) int { return x * 2 })
	if len(result) != 1 || result[0] != 10 {
		t.Fatalf("expected [10], got %v", result)
	}
}

func TestSequentialEmptyInput(t *testing.T) {
	result := Sequential([]int{}, func(x int) int { return x * 2 })
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
}

func TestSequentialPreservesArguments(t *testing.T) {
	pairs := []struct{ a, b int }{{1, 2}, {3, 4}, {5, 6}}
	results := Sequential(pairs, func(p struct{ a, b int }) int {
		return p.a + p.b
	})
	expected := []int{3, 7, 11}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("results[%d] = %d, want %d", i, results[i], v)
		}
	}
}

func TestSequentialPreservesOrder(t *testing.T) {
	var callOrder []int
	input := []int{1, 2, 3, 4, 5}
	Sequential(input, func(n int) int {
		callOrder = append(callOrder, n)
		return n * 2
	})
	for i, v := range input {
		if callOrder[i] != v {
			t.Errorf("callOrder[%d] = %d, want %d", i, callOrder[i], v)
		}
	}
}

func TestSequentialDifferentTypes(t *testing.T) {
	input := []string{"hello", "world"}
	results := Sequential(input, func(s string) int { return len(s) })
	if len(results) != 2 || results[0] != 5 || results[1] != 5 {
		t.Fatalf("expected [5, 5], got %v", results)
	}
}

// ─── SequentialConcurrent ───

func TestSequentialConcurrentBasic(t *testing.T) {
	input := []int{1, 2, 3}
	results := SequentialConcurrent(input, 2, func(x int) int { return x * 2 })
	expected := []int{2, 4, 6}
	for i, v := range expected {
		if results[i] != v {
			t.Errorf("results[%d] = %d, want %d", i, results[i], v)
		}
	}
}

func TestSequentialConcurrentEmpty(t *testing.T) {
	results := SequentialConcurrent([]int{}, 2, func(x int) int { return x })
	if len(results) != 0 {
		t.Fatalf("expected empty result, got %v", results)
	}
}

func TestSequentialConcurrentSingleElement(t *testing.T) {
	results := SequentialConcurrent([]int{42}, 1, func(x int) int { return x * 2 })
	if len(results) != 1 || results[0] != 84 {
		t.Fatalf("expected [84], got %v", results)
	}
}

func TestSequentialConcurrentOrderPreserved(t *testing.T) {
	input := []int{10, 20, 30, 40, 50}
	results := SequentialConcurrent(input, 2, func(x int) int { return x })
	for i, v := range input {
		if results[i] != v {
			t.Errorf("results[%d] = %d, want %d", i, results[i], v)
		}
	}
}

func TestSequentialConcurrentHighConcurrency(t *testing.T) {
	input := make([]int, 100)
	for i := range input {
		input[i] = i
	}
	results := SequentialConcurrent(input, 10, func(x int) int { return x * 2 })
	for i, v := range input {
		if results[i] != v*2 {
			t.Errorf("results[%d] = %d, want %d", i, results[i], v*2)
		}
	}
}

// ─── Execution count invariant ───

func TestSequentialExecutionCount(t *testing.T) {
	var count int64
	input := []int{1, 2, 3, 4, 5}
	Sequential(input, func(x int) int {
		atomic.AddInt64(&count, 1)
		return x
	})
	if count != int64(len(input)) {
		t.Fatalf("expected %d executions, got %d", len(input), count)
	}
}

func TestSequentialConcurrentExecutionCount(t *testing.T) {
	var count int64
	input := []int{1, 2, 3, 4, 5}
	SequentialConcurrent(input, 3, func(x int) int {
		atomic.AddInt64(&count, 1)
		return x
	})
	if count != int64(len(input)) {
		t.Fatalf("expected %d executions, got %d", len(input), count)
	}
}

// ─── WithResolvers ───

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
			t.Log("channel received value")
		} else {
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

// ─── WithResolvers Roundtrip ───

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

// ─── Sleep ───

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
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("sleep returned too early")
	}
}

// ─── SleepWithAbort ───

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

// ─── WithTimeout ───

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

// ─── Sleep Roundtrip ───

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

// ─── FromArray ───

func TestFromArrayYieldsAllElements(t *testing.T) {
	gen := FromArray([]int{10, 20, 30})
	result := ToArray(gen)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	expected := []int{10, 20, 30}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %d, want %d", i, result[i], v)
		}
	}
}

func TestFromArrayEmpty(t *testing.T) {
	gen := FromArray([]int{})
	result := ToArray(gen)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d elements", len(result))
	}
}

// ─── ToArray ───

func TestToArrayCollectsAll(t *testing.T) {
	gen := FromArray([]int{0, 1, 2, 3})
	result := ToArray(gen)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
}

func TestToArrayPreservesOrder(t *testing.T) {
	gen := FromArray([]string{"c", "b", "a"})
	result := ToArray(gen)
	if len(result) != 3 || result[0] != "c" || result[1] != "b" || result[2] != "a" {
		t.Fatalf("expected [c, b, a], got %v", result)
	}
}

func TestToArrayEmpty(t *testing.T) {
	gen := FromArray([]string{})
	result := ToArray(gen)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d elements", len(result))
	}
}

// ─── LastX ───

func TestLastXReturnsLastValue(t *testing.T) {
	gen := FromArray([]int{0, 1, 2, 3, 4})
	last, err := LastX(gen)
	if err != nil {
		t.Fatal(err)
	}
	if last != 4 {
		t.Fatalf("expected 4, got %d", last)
	}
}

func TestLastXSingleValue(t *testing.T) {
	gen := FromArray([]int{0})
	last, err := LastX(gen)
	if err != nil {
		t.Fatal(err)
	}
	if last != 0 {
		t.Fatalf("expected 0, got %d", last)
	}
}

func TestLastXEmpty(t *testing.T) {
	gen := FromArray([]int{})
	_, err := LastX(gen)
	if err == nil {
		t.Fatal("expected error for empty generator")
	}
}

// ─── All ───

func TestAllMergesMultipleGenerators(t *testing.T) {
	gen1 := FromArray([]int{1, 2})
	gen2 := FromArray([]int{3, 4})
	merged := All([]Generator[int]{gen1, gen2})
	result := ToArray(merged)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
}

func TestAllEmptyGeneratorArray(t *testing.T) {
	merged := All([]Generator[int]{})
	result := ToArray(merged)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d elements", len(result))
	}
}

func TestAllSingleGenerator(t *testing.T) {
	gen := FromArray([]int{42})
	merged := All([]Generator[int]{gen})
	result := ToArray(merged)
	if len(result) != 1 || result[0] != 42 {
		t.Fatalf("expected [42], got %v", result)
	}
}

func TestAllDifferentLengthGenerators(t *testing.T) {
	gen1 := FromArray([]int{1, 2, 3})
	gen2 := FromArray([]int{10})
	merged := All([]Generator[int]{gen1, gen2})
	result := ToArray(merged)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
}

func TestAllPreservesAllValues(t *testing.T) {
	gen1 := FromArray([]int{1})
	gen2 := FromArray([]int{2})
	gen3 := FromArray([]int{3})
	merged := All([]Generator[int]{gen1, gen2, gen3})
	result := ToArray(merged)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	// All values should be present (order may vary due to concurrency)
	seen := make(map[int]bool)
	for _, v := range result {
		seen[v] = true
	}
	for _, v := range []int{1, 2, 3} {
		if !seen[v] {
			t.Errorf("missing value %d", v)
		}
	}
}

// ─── Generators Roundtrip ───

func TestFromArrayToArrayIdentity(t *testing.T) {
	original := []int{100, 200, 300, 400, 500}
	result := ToArray(FromArray(original))
	if len(result) != len(original) {
		t.Fatalf("length mismatch: expected %d, got %d", len(original), len(result))
	}
	for i := range original {
		if result[i] != original[i] {
			t.Errorf("result[%d] = %d, want %d", i, result[i], original[i])
		}
	}
}
