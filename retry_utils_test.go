package main

import (
	"testing"
	"time"
)

func TestJitteredBackoffBase(t *testing.T) {
	// attempt=1: delay = base * 2^0 = base = 5s, + jitter
	d := jitteredBackoff(1)
	if d < 5*time.Second || d > 8*time.Second {
		t.Errorf("jitteredBackoff(1) = %v, expected ~5-7.5s", d)
	}
}

func TestJitteredBackoffMaxDelay(t *testing.T) {
	// With small max, should cap quickly
	d := jitteredBackoff(10, WithJitterMax(10*time.Second))
	if d > 15*time.Second { // max + jitter (0.5 * max = 5s)
		t.Errorf("jitteredBackoff(10) with max=10s = %v, expected <=15s", d)
	}
}

func TestJitteredBackoffOverflowProtection(t *testing.T) {
	// attempt >= 64 means exponent >= 63, should return maxDelay directly
	d := jitteredBackoff(64)
	if d != 120*time.Second {
		t.Errorf("jitteredBackoff(64) = %v, expected 120s (maxDelay)", d)
	}
	d = jitteredBackoff(100)
	if d != 120*time.Second {
		t.Errorf("jitteredBackoff(100) = %v, expected 120s (maxDelay)", d)
	}
}

func TestJitteredBackoffNegativeAttempt(t *testing.T) {
	// attempt=0 or negative: exponent clamped to 0
	d := jitteredBackoff(0)
	if d < 5*time.Second {
		t.Errorf("jitteredBackoff(0) = %v, expected >= 5s", d)
	}
}

func TestJitteredBackoffCustomOptions(t *testing.T) {
	d := jitteredBackoff(1,
		WithJitterBase(1*time.Second),
		WithJitterMax(30*time.Second),
		WithJitterRatio(0),
	)
	// With ratio=0, no jitter, delay = base * 2^0 = 1s
	if d != 1*time.Second {
		t.Errorf("jitteredBackoff(1, base=1s, max=30s, ratio=0) = %v, expected 1s", d)
	}
}

func TestJitteredBackoffExponentialGrowth(t *testing.T) {
	// Verify exponential growth: attempt 1->5s, 2->10s, 3->20s, 4->40s, 5->80s
	// With ratio=0 for deterministic results
	for attempt, expected := range map[int]time.Duration{
		1: 5 * time.Second,
		2: 10 * time.Second,
		3: 20 * time.Second,
		4: 40 * time.Second,
		5: 80 * time.Second,
	} {
		d := jitteredBackoff(attempt, WithJitterRatio(0))
		if d != expected {
			t.Errorf("jitteredBackoff(%d, ratio=0) = %v, expected %v", attempt, d, expected)
		}
	}
}

func TestJitteredBackoffCapsAtMax(t *testing.T) {
	// attempt=6: base*2^5 = 160s, but max=120s -> capped
	d := jitteredBackoff(6, WithJitterRatio(0))
	if d != 120*time.Second {
		t.Errorf("jitteredBackoff(6, ratio=0) = %v, expected 120s (capped)", d)
	}
}