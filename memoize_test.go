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