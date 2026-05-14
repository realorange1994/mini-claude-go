package main

import (
	"testing"
)

// ─── FromArray ───────────────────────────────────────────────────────────────

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

// ─── ToArray ─────────────────────────────────────────────────────────────────

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

// ─── LastX ───────────────────────────────────────────────────────────────────

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

// ─── All ─────────────────────────────────────────────────────────────────────

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

// ─── Roundtrip: FromArray → ToArray identity ─────────────────────────────────

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
