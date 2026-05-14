package main

import (
	"testing"
)

// ─── GroupBy ─────────────────────────────────────────────────────────────────

func TestGroupByGroupItemsByKey(t *testing.T) {
	result := GroupBy([]int{1, 2, 3, 4}, func(n int) string {
		if n%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if len(result["even"]) != 2 || result["even"][0] != 2 || result["even"][1] != 4 {
		t.Fatalf("expected even=[2,4], got %v", result["even"])
	}
	if len(result["odd"]) != 2 || result["odd"][0] != 1 || result["odd"][1] != 3 {
		t.Fatalf("expected odd=[1,3], got %v", result["odd"])
	}
}

func TestGroupByEmptyInput(t *testing.T) {
	result := GroupBy([]int{}, func(n int) string { return "key" })
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}
}

func TestGroupBySingleGroup(t *testing.T) {
	result := GroupBy([]string{"a", "b", "c"}, func(s string) string { return "all" })
	if len(result["all"]) != 3 {
		t.Fatalf("expected 3 elements in 'all', got %d", len(result["all"]))
	}
}

func TestGroupByWithIndex(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	result := GroupBy(items, func(s string) string {
		// Find index by scanning items
		for i, item := range items {
			if item == s {
				if i < 2 {
					return "first"
				}
				return "second"
			}
		}
		return "other"
	})
	if len(result["first"]) != 2 {
		t.Fatalf("expected 2 elements in 'first', got %d", len(result["first"]))
	}
	if len(result["second"]) != 2 {
		t.Fatalf("expected 2 elements in 'second', got %d", len(result["second"]))
	}
}

func TestGroupByWithStructs(t *testing.T) {
	type Item struct {
		Name string
		Role string
	}
	items := []Item{
		{"Alice", "admin"},
		{"Bob", "user"},
		{"Charlie", "admin"},
	}
	result := GroupBy(items, func(item Item) string { return item.Role })
	if len(result["admin"]) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(result["admin"]))
	}
	if len(result["user"]) != 1 {
		t.Fatalf("expected 1 user, got %d", len(result["user"]))
	}
}

func TestGroupByPreservesOrder(t *testing.T) {
	// Within each group, elements should maintain their original order
	result := GroupBy([]int{1, 2, 3, 4, 5, 6}, func(n int) string {
		if n%3 == 0 {
			return "div3"
		}
		return "other"
	})
	other := result["other"]
	if len(other) != 4 || other[0] != 1 || other[1] != 2 || other[2] != 4 || other[3] != 5 {
		t.Fatalf("expected other=[1,2,4,5], got %v", other)
	}
}

// ─── Invariant: sum of group sizes equals input size ─────────────────────────

func TestGroupBySumEqualsInputSize(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := GroupBy(input, func(n int) string {
		return "group-" + string(rune('a'+n%3))
	})
	total := 0
	for _, group := range result {
		total += len(group)
	}
	if total != len(input) {
		t.Fatalf("sum of group sizes (%d) != input size (%d)", total, len(input))
	}
}

// ─── Boundary: zero-value key ────────────────────────────────────────────────

func TestGroupByZeroKey(t *testing.T) {
	result := GroupBy([]int{0, 1, 0, 2, 0}, func(n int) int { return n % 2 })
	if len(result[0]) != 4 { // 0,0,2,0 all give remainder 0
		t.Fatalf("expected 4 elements with key 0, got %d", len(result[0]))
	}
	if len(result[1]) != 1 { // only 1 gives remainder 1
		t.Fatalf("expected 1 element with key 1, got %d", len(result[1]))
	}
}
