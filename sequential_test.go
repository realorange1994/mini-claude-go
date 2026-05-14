package main

import (
	"sync/atomic"
	"testing"
)

// ─── Sequential ──────────────────────────────────────────────────────────────

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

// ─── SequentialConcurrent ────────────────────────────────────────────────────

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

// ─── Execution count invariant ───────────────────────────────────────────────

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
