package main

import (
	"container/list"
	"encoding/json"
	"sync"
	"time"
)

// memoizeWithTTL creates a memoized function with a time-to-live cache.
// Returns cached values while refreshing in parallel when stale.
// Ported from upstream memoize.ts memoizeWithTTL.
type MemoizedWithTTL struct {
	fn    func(args ...interface{}) interface{}
	cache map[string]*ttlCacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

type ttlCacheEntry struct {
	value      interface{}
	timestamp  time.Time
	refreshing bool
}

// MemoizeWithTTL creates a new TTL-memoized function.
func MemoizeWithTTL(fn func(args ...interface{}) interface{}, ttl time.Duration) *MemoizedWithTTL {
	return &MemoizedWithTTL{
		fn:    fn,
		cache: make(map[string]*ttlCacheEntry),
		ttl:   ttl,
	}
}

// Call invokes the memoized function. Returns cached value if fresh,
// stale value if expired (with background refresh), or computes fresh value.
func (m *MemoizedWithTTL) Call(args ...interface{}) interface{} {
	key := argsToKey(args)
	now := time.Now()

	m.mu.Lock()
	cached, ok := m.cache[key]
	if !ok {
		value := m.fn(args...)
		m.cache[key] = &ttlCacheEntry{value: value, timestamp: now}
		m.mu.Unlock()
		return value
	}

	// If stale and not already refreshing, trigger background refresh
	if now.Sub(cached.timestamp) > m.ttl && !cached.refreshing {
		cached.refreshing = true
		m.mu.Unlock()

		// Background refresh (goroutine)
		go func() {
			newValue := m.fn(args...)
			m.mu.Lock()
			if m.cache[key] == cached {
				m.cache[key] = &ttlCacheEntry{
					value:      newValue,
					timestamp:  time.Now(),
					refreshing: false,
				}
			}
			m.mu.Unlock()
		}()

		return cached.value
	}

	m.mu.Unlock()
	return cached.value
}

// Clear empties the cache.
func (m *MemoizedWithTTL) Clear() {
	m.mu.Lock()
	m.cache = make(map[string]*ttlCacheEntry)
	m.mu.Unlock()
}

// lruCache is a simple LRU cache implementation.
type lruCache struct {
	maxSize int
	items   map[string]*list.Element
	order   *list.List
	mu      sync.RWMutex
}

type lruEntry struct {
	key   string
	value interface{}
}

func newLRUCache(maxSize int) *lruCache {
	return &lruCache{
		maxSize: maxSize,
		items:   make(map[string]*list.Element),
		order:   list.New(),
	}
}

func (c *lruCache) get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*lruEntry).value, true
}

func (c *lruCache) peek(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	return elem.Value.(*lruEntry).value, true
}

func (c *lruCache) set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*lruEntry).value = value
		return
	}
	if c.order.Len() >= c.maxSize {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*lruEntry).key)
		}
	}
	entry := &lruEntry{key: key, value: value}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

func (c *lruCache) delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return false
	}
	c.order.Remove(elem)
	delete(c.items, key)
	return true
}

func (c *lruCache) clear() {
	c.mu.Lock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
	c.mu.Unlock()
}

func (c *lruCache) size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

func (c *lruCache) has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.items[key]
	return ok
}

// MemoizedWithLRU is a memoized function with an LRU eviction policy.
type MemoizedWithLRU struct {
	fn    func(args ...interface{}) interface{}
	keyFn func(args ...interface{}) string
	cache *lruCache
}

// MemoizeWithLRU creates a new LRU-memoized function.
// Ported from upstream memoize.ts memoizeWithLRU.
func MemoizeWithLRU(
	fn func(args ...interface{}) interface{},
	keyFn func(args ...interface{}) string,
	maxSize int,
) *MemoizedWithLRU {
	return &MemoizedWithLRU{
		fn:    fn,
		keyFn: keyFn,
		cache: newLRUCache(maxSize),
	}
}

// Call invokes the memoized function, using LRU cache.
func (m *MemoizedWithLRU) Call(args ...interface{}) interface{} {
	key := m.keyFn(args...)
	if cached, ok := m.cache.peek(key); ok {
		return cached
	}
	result := m.fn(args...)
	m.cache.set(key, result)
	return result
}

// CacheClear empties the LRU cache.
func (m *MemoizedWithLRU) CacheClear() { m.cache.clear() }

// CacheSize returns the current number of entries.
func (m *MemoizedWithLRU) CacheSize() int { return m.cache.size() }

// CacheDelete removes an entry by key.
func (m *MemoizedWithLRU) CacheDelete(key string) bool { return m.cache.delete(key) }

// CacheGet returns a value without updating recency (peek).
func (m *MemoizedWithLRU) CacheGet(key string) (interface{}, bool) { return m.cache.peek(key) }

// CacheHas checks if a key exists.
func (m *MemoizedWithLRU) CacheHas(key string) bool { return m.cache.has(key) }

// argsToKey converts function arguments to a cache key using JSON serialization.
func argsToKey(args []interface{}) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		// Fallback: use string representation
		return err.Error()
	}
	return string(encoded)
}
