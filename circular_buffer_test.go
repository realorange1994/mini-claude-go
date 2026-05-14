package main

import (
	"testing"
)

// ---------- CircularBuffer tests ----------
// Ported from upstream CircularBuffer.test.ts

func TestCircularBuffer_BasicAddAndLength(t *testing.T) {
	buf := NewCircularBuffer[int](3)
	if buf.Length() != 0 {
		t.Fatalf("expected length 0, got %d", buf.Length())
	}

	buf.Add(1)
	if buf.Length() != 1 {
		t.Fatalf("expected length 1, got %d", buf.Length())
	}

	buf.Add(2)
	buf.Add(3)
	if buf.Length() != 3 {
		t.Fatalf("expected length 3, got %d", buf.Length())
	}
}

func TestCircularBuffer_Eviction(t *testing.T) {
	buf := NewCircularBuffer[int](3)
	buf.Add(1)
	buf.Add(2)
	buf.Add(3)
	buf.Add(4) // should evict 1

	if buf.Length() != 3 {
		t.Fatalf("expected length 3, got %d", buf.Length())
	}

	arr := buf.ToArray()
	if len(arr) != 3 {
		t.Fatalf("expected 3 items, got %d", len(arr))
	}

	// Items should be [2, 3, 4]
	expected := []int{2, 3, 4}
	for i, v := range expected {
		if arr[i] != v {
			t.Fatalf("index %d: expected %d, got %d", i, v, arr[i])
		}
	}
}

func TestCircularBuffer_MultipleEvictions(t *testing.T) {
	buf := NewCircularBuffer[int](3)
	for i := 1; i <= 10; i++ {
		buf.Add(i)
	}

	arr := buf.ToArray()
	expected := []int{8, 9, 10}
	for i, v := range expected {
		if arr[i] != v {
			t.Fatalf("index %d: expected %d, got %d", i, v, arr[i])
		}
	}
}

func TestCircularBuffer_ToArrayIsACopy(t *testing.T) {
	buf := NewCircularBuffer[int](3)
	buf.Add(1)
	buf.Add(2)

	arr := buf.ToArray()
	arr[0] = 999

	original := buf.ToArray()
	if original[0] == 999 {
		t.Fatal("ToArray should return a copy, not a reference")
	}
}

func TestCircularBuffer_GetRecent(t *testing.T) {
	buf := NewCircularBuffer[int](5)
	buf.Add(1)
	buf.Add(2)
	buf.Add(3)
	buf.Add(4)
	buf.Add(5)

	// GetRecent(2) should return [4, 5]
	recent := buf.GetRecent(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 items, got %d", len(recent))
	}
	if recent[0] != 4 || recent[1] != 5 {
		t.Fatalf("expected [4,5], got %v", recent)
	}

	// GetRecent(10) should return all items
	all := buf.GetRecent(10)
	if len(all) != 5 {
		t.Fatalf("expected 5 items, got %d", len(all))
	}
}

func TestCircularBuffer_GetRecentEmpty(t *testing.T) {
	buf := NewCircularBuffer[int](5)
	recent := buf.GetRecent(3)
	if len(recent) != 0 {
		t.Fatalf("expected 0 items from empty buffer, got %d", len(recent))
	}
}

func TestCircularBuffer_Clear(t *testing.T) {
	buf := NewCircularBuffer[int](3)
	buf.Add(1)
	buf.Add(2)
	buf.Clear()

	if buf.Length() != 0 {
		t.Fatalf("expected length 0 after clear, got %d", buf.Length())
	}

	arr := buf.ToArray()
	if len(arr) != 0 {
		t.Fatalf("expected empty array after clear, got %v", arr)
	}

	// Should still be usable after clear
	buf.Add(10)
	if buf.Length() != 1 {
		t.Fatalf("expected length 1 after clear+add, got %d", buf.Length())
	}
	if buf.ToArray()[0] != 10 {
		t.Fatalf("expected [10], got %v", buf.ToArray())
	}
}

func TestCircularBuffer_AddAll(t *testing.T) {
	buf := NewCircularBuffer[int](4)
	buf.AddAll([]int{1, 2, 3, 4})

	arr := buf.ToArray()
	expected := []int{1, 2, 3, 4}
	for i, v := range expected {
		if arr[i] != v {
			t.Fatalf("index %d: expected %d, got %d", i, v, arr[i])
		}
	}
}

func TestCircularBuffer_AddAllWithEviction(t *testing.T) {
	buf := NewCircularBuffer[int](3)
	buf.AddAll([]int{1, 2, 3, 4, 5})

	arr := buf.ToArray()
	expected := []int{3, 4, 5}
	for i, v := range expected {
		if arr[i] != v {
			t.Fatalf("index %d: expected %d, got %d", i, v, arr[i])
		}
	}
}

func TestCircularBuffer_CapacityOne(t *testing.T) {
	buf := NewCircularBuffer[int](1)
	buf.Add(1)
	buf.Add(2)
	buf.Add(3)

	if buf.Length() != 1 {
		t.Fatalf("expected length 1, got %d", buf.Length())
	}
	if buf.ToArray()[0] != 3 {
		t.Fatalf("expected [3], got %v", buf.ToArray())
	}
}

func TestCircularBuffer_ZeroCapacityClampsToOne(t *testing.T) {
	buf := NewCircularBuffer[int](0)
	buf.Add(42)
	if buf.Length() != 1 {
		t.Fatalf("expected capacity clamped to 1, length should be 1, got %d", buf.Length())
	}
}

func TestCircularBuffer_NegativeCapacityClampsToOne(t *testing.T) {
	buf := NewCircularBuffer[int](-5)
	buf.Add(42)
	if buf.Length() != 1 {
		t.Fatalf("expected capacity clamped to 1, length should be 1, got %d", buf.Length())
	}
}

func TestCircularBuffer_StringType(t *testing.T) {
	buf := NewCircularBuffer[string](3)
	buf.Add("a")
	buf.Add("b")
	buf.Add("c")
	buf.Add("d")

	arr := buf.ToArray()
	expected := []string{"b", "c", "d"}
	for i, v := range expected {
		if arr[i] != v {
			t.Fatalf("index %d: expected %s, got %s", i, v, arr[i])
		}
	}
}

func TestCircularBuffer_EvictionOrderPreserved(t *testing.T) {
	buf := NewCircularBuffer[int](5)
	for i := 0; i < 100; i++ {
		buf.Add(i)
	}

	arr := buf.ToArray()
	expected := []int{95, 96, 97, 98, 99}
	for i, v := range expected {
		if arr[i] != v {
			t.Fatalf("index %d: expected %d, got %d", i, v, arr[i])
		}
	}
}
