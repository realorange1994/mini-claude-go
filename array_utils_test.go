package main

import (
	"testing"
)

// ─── Intersperse ─────────────────────────────────────────────────────────────

func TestIntersperseInsertsSeparator(t *testing.T) {
	result := Intersperse([]int{1, 2, 3}, func(i int) int { return 0 })
	if len(result) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(result))
	}
	expected := []int{1, 0, 2, 0, 3}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %d, want %d", i, result[i], v)
		}
	}
}

func TestIntersperseEmpty(t *testing.T) {
	result := Intersperse([]int{}, func(i int) int { return 0 })
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(result))
	}
}

func TestIntersperseSingleElement(t *testing.T) {
	result := Intersperse([]int{1}, func(i int) int { return 0 })
	if len(result) != 1 || result[0] != 1 {
		t.Fatalf("expected [1], got %v", result)
	}
}

func TestInterspassesIndexToSeparator(t *testing.T) {
	result := Intersperse([]string{"a", "b", "c"}, func(i int) string { return "sep-" + string(rune('0'+i)) })
	if len(result) != 5 {
		t.Fatalf("expected 5 elements, got %d", len(result))
	}
	expected := []string{"a", "sep-1", "b", "sep-2", "c"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

// ─── Count ───────────────────────────────────────────────────────────────────

func TestCountMatchingElements(t *testing.T) {
	result := Count([]int{1, 2, 3, 4, 5}, func(x int) bool { return x > 3 })
	if result != 2 {
		t.Fatalf("expected 2, got %d", result)
	}
}

func TestCountEmptySlice(t *testing.T) {
	result := Count([]int{}, func(x int) bool { return true })
	if result != 0 {
		t.Fatalf("expected 0, got %d", result)
	}
}

func TestCountNothingMatches(t *testing.T) {
	result := Count([]int{1, 2, 3}, func(x int) bool { return x > 10 })
	if result != 0 {
		t.Fatalf("expected 0, got %d", result)
	}
}

func TestCountAllMatch(t *testing.T) {
	result := Count([]int{1, 2, 3}, func(x int) bool { return true })
	if result != 3 {
		t.Fatalf("expected 3, got %d", result)
	}
}

// ─── Uniq ────────────────────────────────────────────────────────────────────

func TestUniqRemovesDuplicates(t *testing.T) {
	result := Uniq([]int{1, 2, 2, 3, 3, 3})
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	expected := []int{1, 2, 3}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %d, want %d", i, result[i], v)
		}
	}
}

func TestUniqPreservesFirstOccurrence(t *testing.T) {
	result := Uniq([]int{3, 1, 2, 1, 3})
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	expected := []int{3, 1, 2}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %d, want %d", i, result[i], v)
		}
	}
}

func TestUniqEmptySlice(t *testing.T) {
	result := Uniq([]int{})
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d elements", len(result))
	}
}

func TestUniqStrings(t *testing.T) {
	result := Uniq([]string{"a", "b", "a"})
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" {
		t.Fatalf("expected [a, b], got %v", result)
	}
}
