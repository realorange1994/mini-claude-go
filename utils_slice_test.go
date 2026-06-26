package main

import (
	"testing"
)

// ─── GroupBy Tests ────────────────────────────────────────────────────────────

func TestGroupByEmpty(t *testing.T) {
	result := GroupBy([]int{}, func(x int) string {
		if x%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestGroupByStrings(t *testing.T) {
	items := []string{"apple", "avocado", "banana", "blueberry", "cherry"}
	result := GroupBy(items, func(s string) byte { return s[0] })

	if len(result['a']) != 2 {
		t.Errorf("expected 2 items starting with 'a', got %d", len(result['a']))
	}
	if len(result['b']) != 2 {
		t.Errorf("expected 2 items starting with 'b', got %d", len(result['b']))
	}
	if len(result['c']) != 1 {
		t.Errorf("expected 1 item starting with 'c', got %d", len(result['c']))
	}
}

func TestGroupByInts(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6}
	result := GroupBy(items, func(x int) string {
		if x%2 == 0 {
			return "even"
		}
		return "odd"
	})

	if len(result["even"]) != 3 {
		t.Errorf("expected 3 even items, got %d", len(result["even"]))
	}
	if len(result["odd"]) != 3 {
		t.Errorf("expected 3 odd items, got %d", len(result["odd"]))
	}
}

func TestGroupBySumOfSizesInvariant(t *testing.T) {
	// Invariant: sum of group sizes == original slice length
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := GroupBy(items, func(x int) string {
		if x%3 == 0 {
			return "mod3"
		} else if x%3 == 1 {
			return "mod1"
		}
		return "mod2"
	})

	total := 0
	for _, group := range result {
		total += len(group)
	}
	if total != len(items) {
		t.Errorf("sum of group sizes (%d) != original length (%d)", total, len(items))
	}
}

func TestGroupByAllSameKey(t *testing.T) {
	items := []int{1, 2, 3}
	result := GroupBy(items, func(x int) string { return "same" })

	if len(result) != 1 {
		t.Errorf("expected 1 group, got %d", len(result))
	}
	if len(result["same"]) != 3 {
		t.Errorf("expected 3 items in 'same' group, got %d", len(result["same"]))
	}
}

func TestGroupByAllUniqueKeys(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := GroupBy(items, func(s string) string { return s })

	if len(result) != 3 {
		t.Errorf("expected 3 groups, got %d", len(result))
	}
	for _, s := range items {
		if len(result[s]) != 1 {
			t.Errorf("expected 1 item in group %q, got %d", s, len(result[s]))
		}
	}
}

// ─── Array Utilities Tests ────────────────────────────────────────────────────

func TestCountEmpty(t *testing.T) {
	result := Count([]int{}, func(x int) bool { return x > 0 })
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

func TestCountSome(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	result := Count(items, func(x int) bool { return x > 3 })
	if result != 2 {
		t.Errorf("expected 2, got %d", result)
	}
}

func TestCountAll(t *testing.T) {
	items := []int{2, 4, 6}
	result := Count(items, func(x int) bool { return x%2 == 0 })
	if result != 3 {
		t.Errorf("expected 3, got %d", result)
	}
}

func TestUniqEmpty(t *testing.T) {
	result := Uniq([]int{})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestUniqNoDuplicates(t *testing.T) {
	result := Uniq([]int{1, 2, 3})
	if len(result) != 3 {
		t.Errorf("expected 3 elements, got %d", len(result))
	}
}

func TestUniqWithDuplicates(t *testing.T) {
	result := Uniq([]int{1, 2, 2, 3, 1, 4})
	expected := []int{1, 2, 3, 4}
	if len(result) != len(expected) {
		t.Fatalf("expected %d elements, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("at index %d: expected %d, got %d", i, v, result[i])
		}
	}
}

func TestUniqPreservesOrder(t *testing.T) {
	result := Uniq([]string{"c", "a", "b", "a", "c"})
	expected := []string{"c", "a", "b"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d elements, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("at index %d: expected %q, got %q", i, v, result[i])
		}
	}
}
