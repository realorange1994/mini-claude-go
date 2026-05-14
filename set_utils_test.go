package main

import (
	"testing"
)

// ─── SetDifference ───────────────────────────────────────────────────────────

func TestSetDifferenceElementsInANotInB(t *testing.T) {
	a := map[int]bool{1: true, 2: true, 3: true}
	b := map[int]bool{2: true, 3: true, 4: true}
	result := SetDifference(a, b)
	if len(result) != 1 || !result[1] {
		t.Fatalf("expected {1}, got %v", result)
	}
}

func TestSetDifferenceASubsetOfB(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{1: true, 2: true, 3: true}
	result := SetDifference(a, b)
	if len(result) != 0 {
		t.Fatalf("expected empty set, got %v", result)
	}
}

func TestSetDifferenceBEmpty(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{}
	result := SetDifference(a, b)
	if len(result) != 2 || !result[1] || !result[2] {
		t.Fatalf("expected {1,2}, got %v", result)
	}
}

func TestSetDifferenceBothEmpty(t *testing.T) {
	a := map[int]bool{}
	b := map[int]bool{}
	result := SetDifference(a, b)
	if len(result) != 0 {
		t.Fatalf("expected empty set, got %v", result)
	}
}

// ─── SetIntersects ───────────────────────────────────────────────────────────

func TestSetIntersectsTrueWhenShared(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{2: true, 3: true}
	if !SetIntersects(a, b) {
		t.Fatal("expected true, got false")
	}
}

func TestSetIntersectsFalseWhenDisjoint(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{3: true, 4: true}
	if SetIntersects(a, b) {
		t.Fatal("expected false, got true")
	}
}

func TestSetIntersectsFalseForEmptySets(t *testing.T) {
	a := map[int]bool{}
	b := map[int]bool{1: true}
	if SetIntersects(a, b) {
		t.Fatal("expected false when a is empty")
	}
	c := map[int]bool{1: true}
	d := map[int]bool{}
	if SetIntersects(c, d) {
		t.Fatal("expected false when b is empty")
	}
}

// ─── SetEvery ────────────────────────────────────────────────────────────────

func TestSetEveryTrueWhenSubset(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{1: true, 2: true, 3: true}
	if !SetEvery(a, b) {
		t.Fatal("expected true, got false")
	}
}

func TestSetEveryFalseWhenNotSubset(t *testing.T) {
	a := map[int]bool{1: true, 4: true}
	b := map[int]bool{1: true, 2: true, 3: true}
	if SetEvery(a, b) {
		t.Fatal("expected false, got true")
	}
}

func TestSetEveryTrueForEmptyA(t *testing.T) {
	a := map[int]bool{}
	b := map[int]bool{1: true, 2: true}
	if !SetEvery(a, b) {
		t.Fatal("expected true for empty a (vacuously true)")
	}
}

// ─── SetUnion ────────────────────────────────────────────────────────────────

func TestSetUnionCombinesBoth(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{3: true, 4: true}
	result := SetUnion(a, b)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
	for _, v := range []int{1, 2, 3, 4} {
		if !result[v] {
			t.Errorf("expected %d in union", v)
		}
	}
}

func TestSetUnionDeduplicates(t *testing.T) {
	a := map[int]bool{1: true, 2: true}
	b := map[int]bool{2: true, 3: true}
	result := SetUnion(a, b)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
}

func TestSetUnionEmptySets(t *testing.T) {
	a := map[int]bool{}
	b := map[int]bool{1: true}
	result := SetUnion(a, b)
	if len(result) != 1 || !result[1] {
		t.Fatalf("expected {1}, got %v", result)
	}

	c := map[int]bool{1: true}
	d := map[int]bool{}
	result2 := SetUnion(c, d)
	if len(result2) != 1 || !result2[1] {
		t.Fatalf("expected {1}, got %v", result2)
	}
}

// ─── Roundtrip / Invariant tests ─────────────────────────────────────────────

func TestSetDifferenceUnionIdentity(t *testing.T) {
	// a = difference(a,b) union (a intersect b)
	a := map[int]bool{1: true, 2: true, 3: true}
	b := map[int]bool{2: true, 3: true, 4: true}
	diff := SetDifference(a, b)
	intersect := map[int]bool{}
	for k := range a {
		if b[k] {
			intersect[k] = true
		}
	}
	reconstructed := SetUnion(diff, intersect)
	if len(reconstructed) != len(a) {
		t.Fatalf("roundtrip failed: expected %d elements, got %d", len(a), len(reconstructed))
	}
	for k := range a {
		if !reconstructed[k] {
			t.Errorf("element %d missing in reconstructed set", k)
		}
	}
}

func TestSetDifferenceDisjointFromIntersect(t *testing.T) {
	// difference(a,b) and intersect(a,b) are always disjoint
	a := map[string]bool{"x": true, "y": true, "z": true}
	b := map[string]bool{"y": true, "z": true, "w": true}
	diff := SetDifference(a, b)
	intersect := map[string]bool{}
	for k := range a {
		if b[k] {
			intersect[k] = true
		}
	}
	if SetIntersects(diff, intersect) {
		t.Fatal("difference and intersection should be disjoint")
	}
}
