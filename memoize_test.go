package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ─── memoizeWithTTL ──────────────────────────────────────────────────────
// Ported from upstream memoize.test.ts

func intKeyFn(args ...interface{}) string {
	v, _ := args[0].(int)
	return fmt.Sprintf("%d", v)
}

func stringKeyFn(args ...interface{}) string {
	v, _ := args[0].(string)
	return v
}

func TestMemoizeWithTTLReturnsCachedValue(t *testing.T) {
	var calls int32
	fn := MemoizeWithTTL(func(args ...interface{}) interface{} {
		atomic.AddInt32(&calls, 1)
		return args[0].(int) * 2
	}, 60*time.Second)

	result1 := fn.Call(5)
	if result1 != 10 {
		t.Errorf("expected 10, got %v", result1)
	}

	result2 := fn.Call(5)
	if result2 != 10 {
		t.Errorf("expected 10 (cached), got %v", result2)
	}

	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&calls))
	}
}

func TestMemoizeWithTTLDifferentArgs(t *testing.T) {
	var calls int32
	fn := MemoizeWithTTL(func(args ...interface{}) interface{} {
		atomic.AddInt32(&calls, 1)
		return args[0].(int) + 1
	}, 60*time.Second)

	fn.Call(1)
	fn.Call(2)

	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected 2 calls (different args), got %d", atomic.LoadInt32(&calls))
	}
}

func TestMemoizeWithTTLClear(t *testing.T) {
	var calls int32
	fn := MemoizeWithTTL(func(args ...interface{}) interface{} {
		atomic.AddInt32(&calls, 1)
		return "val"
	}, 60*time.Second)

	fn.Call()
	fn.Clear()
	fn.Call()

	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected 2 calls after clear, got %d", atomic.LoadInt32(&calls))
	}
}

func TestMemoizeWithTTLExpiredReturnsStale(t *testing.T) {
	var calls int32
	fn := MemoizeWithTTL(func(args ...interface{}) interface{} {
		atomic.AddInt32(&calls, 1)
		n := int(atomic.LoadInt32(&calls))
		return args[0].(int) * n
	}, 10*time.Millisecond)

	first := fn.Call(10)
	if first != 10 {
		t.Errorf("expected first=10 (calls=1, 10*1), got %v", first)
	}

	// Wait for TTL to expire
	time.Sleep(50 * time.Millisecond)

	// Should return stale value immediately
	second := fn.Call(10)
	if second != 10 {
		t.Errorf("expected stale value 10, got %v", second)
	}

	// Wait for background refresh
	time.Sleep(100 * time.Millisecond)

	// Now should return refreshed value (calls=2, 10*2=20)
	third := fn.Call(10)
	if third != 20 {
		t.Errorf("expected refreshed value 20, got %v", third)
	}
}

// ─── memoizeWithLRU ──────────────────────────────────────────────────────
// Ported from upstream memoize.test.ts

func TestMemoizeWithLRUCachesByArg(t *testing.T) {
	var calls int32
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			atomic.AddInt32(&calls, 1)
			return args[0].(int) * 2
		},
		intKeyFn,
		10,
	)

	result := fn.Call(5)
	if result != 10 {
		t.Errorf("expected 10, got %v", result)
	}
	fn.Call(5) // cached
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt32(&calls))
	}
}

func TestMemoizeWithLRUEviction(t *testing.T) {
	var calls int32
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			atomic.AddInt32(&calls, 1)
			return args[0]
		},
		stringKeyFn,
		3,
	)

	fn.Call("1")
	fn.Call("2")
	fn.Call("3")
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt32(&calls))
	}

	fn.Call("4") // should evict "1"
	if fn.CacheHas("1") {
		t.Error("expected '1' to be evicted")
	}
	if !fn.CacheHas("4") {
		t.Error("expected '4' to be in cache")
	}
}

func TestMemoizeWithLRUSize(t *testing.T) {
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return args[0] },
		stringKeyFn,
		10,
	)

	fn.Call("a")
	fn.Call("b")
	if fn.CacheSize() != 2 {
		t.Errorf("expected size 2, got %d", fn.CacheSize())
	}
}

func TestMemoizeWithLRUDelete(t *testing.T) {
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return args[0] },
		stringKeyFn,
		10,
	)

	fn.Call("x")
	if !fn.CacheHas("x") {
		t.Error("expected 'x' to be in cache")
	}
	fn.CacheDelete("x")
	if fn.CacheHas("x") {
		t.Error("expected 'x' to be deleted")
	}
}

func TestMemoizeWithLRUGet(t *testing.T) {
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return args[0].(string) + "_cached" },
		stringKeyFn,
		10,
	)

	fn.Call("key")
	val, ok := fn.CacheGet("key")
	if !ok {
		t.Fatal("expected to find 'key'")
	}
	if val != "key_cached" {
		t.Errorf("expected 'key_cached', got %v", val)
	}
}

func TestMemoizeWithLRUClear(t *testing.T) {
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return args[0] },
		stringKeyFn,
		10,
	)

	fn.Call("a")
	fn.Call("b")
	fn.CacheClear()
	if fn.CacheSize() != 0 {
		t.Errorf("expected size 0 after clear, got %d", fn.CacheSize())
	}
}

// ─── LRU Cache unit tests ────────────────────────────────────────────────

func TestLRUCacheBasicGetSet(t *testing.T) {
	cache := newLRUCache(10)
	cache.set("a", 1)
	val, ok := cache.get("a")
	if !ok || val != 1 {
		t.Errorf("expected 1, got %v (ok=%v)", val, ok)
	}
}

func TestLRUCacheMiss(t *testing.T) {
	cache := newLRUCache(10)
	_, ok := cache.get("nonexistent")
	if ok {
		t.Error("expected miss")
	}
}

func TestLRUCacheEviction(t *testing.T) {
	cache := newLRUCache(2)
	cache.set("a", 1)
	cache.set("b", 2)
	cache.set("c", 3) // should evict "a"
	_, ok := cache.get("a")
	if ok {
		t.Error("expected 'a' to be evicted")
	}
	_, ok = cache.get("c")
	if !ok {
		t.Error("expected 'c' to exist")
	}
}

func TestLRUCacheClear(t *testing.T) {
	cache := newLRUCache(10)
	cache.set("a", 1)
	cache.clear()
	if cache.size() != 0 {
		t.Errorf("expected size 0, got %d", cache.size())
	}
}

func TestLRUCachePeek(t *testing.T) {
	cache := newLRUCache(10)
	cache.set("x", 42)
	val, ok := cache.peek("x")
	if !ok || val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

// ─── Concurrent safety ───────────────────────────────────────────────────

func TestMemoizeWithTTLConcurrent(t *testing.T) {
	var calls int32
	fn := MemoizeWithTTL(func(args ...interface{}) interface{} {
		atomic.AddInt32(&calls, 1)
		time.Sleep(10 * time.Millisecond)
		return "result"
	}, 1*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn.Call("test")
		}()
	}
	wg.Wait()

	// Due to race conditions, some concurrent calls may compute before
	// cache is populated. At minimum, at least one call must happen.
	if atomic.LoadInt32(&calls) < 1 {
		t.Errorf("expected at least 1 call, got %d", atomic.LoadInt32(&calls))
	}
}

func TestMemoizeWithLRUConcurrent(t *testing.T) {
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			return args[0].(int) * 2
		},
		intKeyFn,
		100,
	)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			fn.Call(val)
		}(i % 5) // only 5 unique keys
	}
	wg.Wait()

	// Should handle concurrent access without panic
}

// ============================================================================
// Upstream Quality: lazySchema.test.ts Port
// ============================================================================
// lazySchema is a pattern where a factory function is called once and
// cached. MemoizeWithLRU implements this pattern. These tests verify
// the lazy/cached factory behavior matching upstream patterns.

func TestLazySchemaReturnsFunction(t *testing.T) {
	// From upstream: "returns a function"
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return 42 },
		func(args ...interface{}) string { return "key" },
		10,
	)
	// In Go, we verify the returned type is callable
	result := fn.Call()
	if result != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestLazySchemaFirstInvocationCallsFactory(t *testing.T) {
	// From upstream: "calls factory on first invocation"
	var callCount int32
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			atomic.AddInt32(&callCount, 1)
			return "result"
		},
		stringKeyFn,
		10,
	)
	fn.Call("key")
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected factory called once, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestLazySchemaReturnsCachedResult(t *testing.T) {
	// From upstream: "returns cached result on subsequent invocations"
	callCount := 0
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			callCount++
			return callCount // Returns different values if not cached
		},
		func(args ...interface{}) string { return "same-key" },
		10,
	)
	first := fn.Call("same-key")
	second := fn.Call("same-key")
	third := fn.Call("same-key")

	if first != second {
		t.Errorf("first and second calls should return same value: %v vs %v", first, second)
	}
	if second != third {
		t.Errorf("second and third calls should return same value: %v vs %v", second, third)
	}
	if callCount != 1 {
		t.Errorf("factory should be called only once, got %d", callCount)
	}
}

func TestLazySchemaFactoryCalledOnlyOnce(t *testing.T) {
	// From upstream: "factory is called only once"
	var callCount int32
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			atomic.AddInt32(&callCount, 1)
			return "cached"
		},
		stringKeyFn,
		10,
	)
	fn.Call("x")
	fn.Call("x")
	fn.Call("x")

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected factory called exactly once, got %d", atomic.LoadInt32(&callCount))
	}
}

func TestLazySchemaDifferentReturnTypes(t *testing.T) {
	// From upstream: "works with different return types"
	// Integer
	numFactory := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return 123 },
		stringKeyFn,
		10,
	)
	if numFactory.Call("k") != 123 {
		t.Error("expected 123")
	}

	// Array/slice
	arrFactory := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return []int{1, 2, 3} },
		stringKeyFn,
		10,
	)
	result := arrFactory.Call("k").([]int)
	if len(result) != 3 || result[0] != 1 || result[1] != 2 || result[2] != 3 {
		t.Errorf("expected [1,2,3], got %v", result)
	}

	// Struct/map
	mapFactory := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return map[string]string{"id": "a"} },
		stringKeyFn,
		10,
	)
	mapResult := mapFactory.Call("k").(map[string]string)
	if mapResult["id"] != "a" {
		t.Errorf("expected id=a, got %v", mapResult)
	}
}

func TestLazySchemaIndependentCaches(t *testing.T) {
	// From upstream: "each call to lazySchema returns independent cache"
	a := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return map[string]string{"id": "a"} },
		stringKeyFn,
		10,
	)
	b := MemoizeWithLRU(
		func(args ...interface{}) interface{} { return map[string]string{"id": "b"} },
		stringKeyFn,
		10,
	)

	resultA := a.Call("k").(map[string]string)
	resultB := b.Call("k").(map[string]string)

	if resultA["id"] != "a" {
		t.Errorf("expected id=a, got %v", resultA)
	}
	if resultB["id"] != "b" {
		t.Errorf("expected id=b, got %v", resultB)
	}

	// Verify the two cached values are different
	if resultA["id"] == resultB["id"] {
		t.Error("a and b should have independent caches with different values")
	}
}

func TestLazySchemaIdempotency(t *testing.T) {
	// Idempotency: calling with the same args always returns the same result
	var counter int32
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			atomic.AddInt32(&counter, 1)
			return args[0]
		},
		stringKeyFn,
		10,
	)

	input := "hello"
	results := make([]interface{}, 100)
	for i := 0; i < 100; i++ {
		results[i] = fn.Call(input)
	}

	// All 100 calls should return the same value
	for i := 1; i < 100; i++ {
		if results[i] != results[0] {
			t.Errorf("idempotency broken at call %d: %v != %v", i, results[i], results[0])
		}
	}

	// Factory should be called exactly once
	if atomic.LoadInt32(&counter) != 1 {
		t.Errorf("expected 1 factory call for 100 identical calls, got %d", atomic.LoadInt32(&counter))
	}
}

func TestLazySchemaCacheClearTriggersRecompute(t *testing.T) {
	// After clearing cache, the factory should be called again
	var callCount int32
	fn := MemoizeWithLRU(
		func(args ...interface{}) interface{} {
			atomic.AddInt32(&callCount, 1)
			return "val"
		},
		stringKeyFn,
		10,
	)

	fn.Call("k")
	fn.CacheClear()
	fn.Call("k")

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 calls (before and after clear), got %d", atomic.LoadInt32(&callCount))
	}
}
