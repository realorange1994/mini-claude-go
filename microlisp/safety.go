package microlisp

import (
	"fmt"
	"runtime"
	"time"
)

// -------- Resource Safety Layer --------
//
// This module prevents AI agents (or any external caller) from executing
// Lisp code that consumes excessive memory, CPU, or time, which could
// destabilize the host system.
//
// Limits are enforced via:
//   - Step counting: each eval/apply iteration increments a counter;
//     exceeding the limit aborts with an error.
//   - Time deadline: a wall-clock deadline; exceeding it aborts.
//   - Memory ceiling: runtime memory stats are checked periodically;
//     exceeding the limit aborts.
//   - Cancellation: a context-like cancel channel allows external abort.

// ResourceLimits specifies upper bounds for a single evaluation session.
// A zero-value ResourceLimits means "no limits" (used for REPL / trusted callers).
type ResourceLimits struct {
	MaxSteps    int64         // max eval+apply iterations (0 = unlimited)
	MaxTimeMs   int64         // max wall-clock milliseconds (0 = unlimited)
	MaxMemoryKB int64         // max heap memory in KB (0 = unlimited)
	CancelChan  chan struct{} // external cancellation signal (nil = no cancel)
}

// DefaultLimits returns sensible defaults for AI-agent callers:
//   - 1M eval steps
//   - 30 seconds wall time
//   - 256 MB heap
func DefaultLimits() ResourceLimits {
	return ResourceLimits{
		MaxSteps:    1_000_000,
		MaxTimeMs:   30_000,
		MaxMemoryKB: 256_000,
	}
}

// StrictLimits returns tighter limits for untrusted / exploratory calls:
//   - 100K eval steps
//   - 10 seconds wall time
//   - 64 MB heap
func StrictLimits() ResourceLimits {
	return ResourceLimits{
		MaxSteps:    100_000,
		MaxTimeMs:   10_000,
		MaxMemoryKB: 64_000,
	}
}

// UnlimitedLimits returns generous but still finite limits for REPL mode.
// No mode is truly unlimited — this is the ceiling to prevent system crash.
//   - 50M eval steps
//   - 5 minutes wall time
//   - 1 GB heap
func UnlimitedLimits() ResourceLimits {
	return ResourceLimits{
		MaxSteps:    50_000_000,
		MaxTimeMs:   300_000,
		MaxMemoryKB: 1_000_000,
	}
}

// -------- Global state for the active resource limits --------
// These are set at the start of a limited evaluation session and
// cleared when it finishes. They are protected by evalMu (in eval.go).

var activeLimits ResourceLimits
var evalStepCount int64
var evalDeadline time.Time // zero value means no deadline
var limitsActive bool

// stepCheck is called at every eval iteration and every Apply call.
// It returns an error if any resource limit has been exceeded.
// This is the core enforcement mechanism.
func stepCheck() error {
	if !limitsActive {
		return nil
	}

	// 1. Step count
	evalStepCount++
	if activeLimits.MaxSteps > 0 && evalStepCount > activeLimits.MaxSteps {
		return fmt.Errorf("resource limit: eval step count exceeded (%d > %d)", evalStepCount, activeLimits.MaxSteps)
	}

	// 2. Wall-clock deadline (check every 1024 steps to reduce overhead)
	if evalStepCount&1023 == 0 {
		if !evalDeadline.IsZero() && time.Now().After(evalDeadline) {
			return fmt.Errorf("resource limit: execution time exceeded (deadline was %v)", evalDeadline)
		}

		// 3. External cancellation
		if activeLimits.CancelChan != nil {
			select {
			case <-activeLimits.CancelChan:
				return fmt.Errorf("resource limit: execution cancelled by caller")
			default:
			}
		}

		// 4. Memory ceiling (check every 1024 steps to reduce overhead)
		if activeLimits.MaxMemoryKB > 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			heapKB := int64(m.HeapAlloc / 1024)
			if heapKB > activeLimits.MaxMemoryKB {
				return fmt.Errorf("resource limit: heap memory exceeded (%d KB > %d KB)", heapKB, activeLimits.MaxMemoryKB)
			}
		}
	}

	return nil
}

// startLimits configures the global limit state for a new evaluation session.
// Must be called while holding evalMu.
func startLimits(limits ResourceLimits) {
	activeLimits = limits
	evalStepCount = 0
	limitsActive = true

	if limits.MaxTimeMs > 0 {
		evalDeadline = time.Now().Add(time.Duration(limits.MaxTimeMs) * time.Millisecond)
	} else {
		evalDeadline = time.Time{} // zero = no deadline
	}
}

// stopLimits clears the global limit state after an evaluation session.
// Must be called while holding evalMu.
func stopLimits() {
	limitsActive = false
	activeLimits = ResourceLimits{}
	evalStepCount = 0
	evalDeadline = time.Time{}
}

// -------- Thread-safe limited evaluation entry points --------

// SafeEvalWithLimits evaluates a Lisp expression with resource limits.
// It is thread-safe (uses evalMu) and enforces the provided limits.
// If any limit is exceeded, it returns an error describing which limit was hit.
// InitGlobalEnv() must be called once before first use.
func SafeEvalWithLimits(s string, limits ResourceLimits) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = "", fmt.Errorf("lisp panic: %v", r)
		}
	}()
	evalMu.Lock()
	defer evalMu.Unlock()

	startLimits(limits)
	defer stopLimits()

	v, err := EvalString(s, globalEnv)
	if err != nil {
		return "", err
	}
	return ToString(v), nil
}

// SafeEvalStringCaptureWithLimits evaluates a Lisp expression with resource limits
// and captures all stdout output. Returns (capturedStdout, returnValue, evalError).
func SafeEvalStringCaptureWithLimits(s string, limits ResourceLimits) (captured, returnValue string, err error) {
	defer func() {
		if r := recover(); r != nil {
			captured, returnValue, err = "", "", fmt.Errorf("lisp panic: %v", r)
		}
	}()
	evalMu.Lock()
	defer evalMu.Unlock()

	startLimits(limits)
	defer stopLimits()

	var result *Value
	captured, evalErr := captureStdout(func() error {
		var e error
		result, e = EvalString(s, globalEnv)
		return e
	})
	err = evalErr
	if err == nil && result != nil {
		returnValue = ToString(result)
	}
	return
}

// SafeLoadFileWithLimits loads a Lisp source file with resource limits.
// Returns captured stdout output and any error.
func SafeLoadFileWithLimits(fname string, limits ResourceLimits) (output string, err error) {
	evalMu.Lock()
	defer evalMu.Unlock()

	startLimits(limits)
	defer stopLimits()

	output, loadErr := captureStdout(func() error {
		_, e := LoadFile(fname, globalEnv)
		return e
	})
	err = loadErr
	return
}

// -------- Lint helpers --------

// SafeLintWithLimits parses Lisp source without evaluating, with resource limits.
// Returns any syntax errors found. Thread-safe (uses evalMu).
func SafeLintWithLimits(s string, limits ResourceLimits) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("lisp panic: %v", r)
		}
	}()
	evalMu.Lock()
	defer evalMu.Unlock()

	startLimits(limits)
	defer stopLimits()

	return LintString(s)
}

// SafeLintFileWithLimits parses a Lisp file without evaluating, with resource limits.
// Returns any syntax errors found. Thread-safe (uses evalMu).
func SafeLintFileWithLimits(fname string, limits ResourceLimits) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("lisp panic: %v", r)
		}
	}()
	evalMu.Lock()
	defer evalMu.Unlock()

	startLimits(limits)
	defer stopLimits()

	return LintFile(fname)
}

// -------- Cancellation helper --------

// NewCancelChannel creates a channel that can be used to cancel
// an evaluation session. The caller should close the channel
// (or send on it) to trigger cancellation.
func NewCancelChannel() chan struct{} {
	return make(chan struct{})
}

// -------- Memory stats for diagnostics --------

// EvalResourceStats returns diagnostic information about the last
// evaluation session's resource usage. Useful for debugging limit hits.
type EvalResourceStats struct {
	StepsUsed  int64
	TimeUsedMs int64
	HeapUsedKB int64
	LimitsHit  string // empty if no limit was hit, or description of which limit
}

// GetResourceStats returns stats from the current/last limited session.
// Must be called while holding evalMu or after evaluation completes.
func GetResourceStats() EvalResourceStats {
	stats := EvalResourceStats{
		StepsUsed: evalStepCount,
	}
	if !evalDeadline.IsZero() {
		stats.TimeUsedMs = time.Since(evalDeadline.Add(-time.Duration(activeLimits.MaxTimeMs) * time.Millisecond)).Milliseconds()
	}
	if activeLimits.MaxMemoryKB > 0 {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		stats.HeapUsedKB = int64(m.HeapAlloc / 1024)
	}
	return stats
}
