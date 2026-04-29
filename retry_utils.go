package main

import (
	"math/rand"
	"time"
)

// jitteredBackoff computes a jittered exponential backoff delay.
//
// Replaces fixed exponential backoff with jittered delays to prevent
// thundering-herd retry spikes when multiple sessions hit the same
// rate-limited provider concurrently.
//
// Parameters:
//   - attempt: 1-based retry attempt number
//   - baseDelay: base delay in seconds for attempt 1 (default: 5)
//   - maxDelay: maximum delay cap in seconds (default: 120)
//   - jitterRatio: fraction of computed delay to use as random jitter range (default: 0.5)
//
// Returns: delay = min(base * 2^(attempt-1), maxDelay) + uniform(0, jitterRatio * delay)
func jitteredBackoff(attempt int, opts ...JitterOpt) time.Duration {
	cfg := jitterConfig{
		baseDelay:   5 * time.Second,
		maxDelay:    120 * time.Second,
		jitterRatio: 0.5,
	}
	for _, o := range opts {
		o(&cfg)
	}

	exponent := attempt - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent >= 63 {
		return cfg.maxDelay
	}

	delay := cfg.baseDelay * (1 << uint(exponent))
	if delay > cfg.maxDelay {
		delay = cfg.maxDelay
	}

	jitter := time.Duration(rand.Float64() * cfg.jitterRatio * float64(delay))
	return delay + jitter
}

type jitterConfig struct {
	baseDelay   time.Duration
	maxDelay    time.Duration
	jitterRatio float64
}

// JitterOpt is a functional option for jitteredBackoff.
type JitterOpt func(*jitterConfig)

// WithJitterBase sets the base delay.
func WithJitterBase(d time.Duration) JitterOpt {
	return func(c *jitterConfig) { c.baseDelay = d }
}

// WithJitterMax sets the maximum delay cap.
func WithJitterMax(d time.Duration) JitterOpt {
	return func(c *jitterConfig) { c.maxDelay = d }
}

// WithJitterRatio sets the jitter ratio (0-1).
func WithJitterRatio(r float64) JitterOpt {
	return func(c *jitterConfig) { c.jitterRatio = r }
}
