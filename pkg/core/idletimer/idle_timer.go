// Package idletimer tracks user idle time to optimize context window usage.
// Aligned to pi's idle-timer.ts.
package idletimer

import (
	"sync"
	"time"
)

const (
	// DefaultIdleThresholdMs is the default time before a session is considered idle.
	DefaultIdleThresholdMs = 5 * 60 * 1000 // 5 minutes

	// IdleContextReserve is how much context to reserve when idle.
	IdleContextReserve = 32000
)

// IdleTimer tracks whether the user has been idle.
type IdleTimer struct {
	mu             sync.Mutex
	lastActivity   time.Time
	threshold      time.Duration
	onIdleCallback func()
	onActiveCallback func()
	isIdle         bool
	timer          *time.Timer
}

// NewIdleTimer creates a new idle timer.
func NewIdleTimer(thresholdMs int) *IdleTimer {
	threshold := time.Duration(DefaultIdleThresholdMs) * time.Millisecond
	if thresholdMs > 0 {
		threshold = time.Duration(thresholdMs) * time.Millisecond
	}

	return &IdleTimer{
		lastActivity: time.Now(),
		threshold:    threshold,
		isIdle:       false,
	}
}

// RecordActivity records user activity and resets the idle timer.
func (it *IdleTimer) RecordActivity() {
	it.mu.Lock()
	defer it.mu.Unlock()

	it.lastActivity = time.Now()
	wasIdle := it.isIdle
	it.isIdle = false

	if it.timer != nil {
		it.timer.Stop()
	}

	// Schedule idle check
	it.timer = time.AfterFunc(it.threshold, func() {
		it.mu.Lock()
		it.isIdle = true
		cb := it.onIdleCallback
		it.mu.Unlock()

		if cb != nil && wasIdle != it.isIdle {
			cb()
		}
	})

	if wasIdle && it.onActiveCallback != nil {
		it.onActiveCallback()
	}
}

// IsIdle returns whether the user is currently idle.
func (it *IdleTimer) IsIdle() bool {
	it.mu.Lock()
	defer it.mu.Unlock()
	return it.isIdle
}

// IdleDuration returns how long the user has been idle.
func (it *IdleTimer) IdleDuration() time.Duration {
	it.mu.Lock()
	defer it.mu.Unlock()

	if it.isIdle {
		return time.Since(it.lastActivity)
	}
	return 0
}

// SetOnIdle sets the callback for when the user becomes idle.
func (it *IdleTimer) SetOnIdle(cb func()) {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.onIdleCallback = cb
}

// SetOnActive sets the callback for when the user returns from idle.
func (it *IdleTimer) SetOnActive(cb func()) {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.onActiveCallback = cb
}

// Stop stops the idle timer.
func (it *IdleTimer) Stop() {
	it.mu.Lock()
	defer it.mu.Unlock()

	if it.timer != nil {
		it.timer.Stop()
		it.timer = nil
	}
}
