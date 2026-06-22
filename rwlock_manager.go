package main

import (
	"sync"
)

// ─── Reader-Writer Lock (MiMo-Code 4) ──────────────────────────────────────
//
// Keyed reader-writer lock with writer-priority semantics.
//
// MiMo-Code source: util/lock.ts (96 lines)

// RWLockManager manages keyed reader-writer locks.
type RWLockManager struct {
	mu     sync.Mutex
	locks  map[string]*rwLock
}

// rwLock represents a single reader-writer lock.
type rwLock struct {
	readers    int
	writers    int
	waitWriter int
	cond       *sync.Cond
}

// NewRWLockManager creates a new lock manager.
func NewRWLockManager() *RWLockManager {
	return &RWLockManager{
		locks: make(map[string]*rwLock),
	}
}

// getOrCreateLock gets or creates a lock for a key.
func (m *RWLockManager) getOrCreateLock(key string) *rwLock {
	lock, exists := m.locks[key]
	if !exists {
		lock = &rwLock{}
		lock.cond = sync.NewCond(&sync.Mutex{})
		m.locks[key] = lock
	}
	return lock
}

// AcquireRead acquires a read lock for the given key.
func (m *RWLockManager) AcquireRead(key string) {
	m.mu.Lock()
	lock := m.getOrCreateLock(key)
	m.mu.Unlock()

	lock.cond.L.Lock()
	for lock.writers > 0 || lock.waitWriter > 0 {
		lock.cond.Wait()
	}
	lock.readers++
	lock.cond.L.Unlock()
}

// ReleaseRead releases a read lock for the given key.
func (m *RWLockManager) ReleaseRead(key string) {
	m.mu.Lock()
	lock, exists := m.locks[key]
	m.mu.Unlock()

	if !exists {
		return
	}

	lock.cond.L.Lock()
	lock.readers--
	if lock.readers == 0 {
		lock.cond.Broadcast()
	}
	lock.cond.L.Unlock()
}

// AcquireWrite acquires a write lock for the given key.
func (m *RWLockManager) AcquireWrite(key string) {
	m.mu.Lock()
	lock := m.getOrCreateLock(key)
	m.mu.Unlock()

	lock.cond.L.Lock()
	lock.waitWriter++
	for lock.readers > 0 || lock.writers > 0 {
		lock.cond.Wait()
	}
	lock.waitWriter--
	lock.writers++
	lock.cond.L.Unlock()
}

// ReleaseWrite releases a write lock for the given key.
func (m *RWLockManager) ReleaseWrite(key string) {
	m.mu.Lock()
	lock, exists := m.locks[key]
	m.mu.Unlock()

	if !exists {
		return
	}

	lock.cond.L.Lock()
	lock.writers--
	lock.cond.Broadcast()
	lock.cond.L.Unlock()
}

// Cleanup removes unused locks.
func (m *RWLockManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, lock := range m.locks {
		lock.cond.L.Lock()
		if lock.readers == 0 && lock.writers == 0 && lock.waitWriter == 0 {
			delete(m.locks, key)
		}
		lock.cond.L.Unlock()
	}
}
