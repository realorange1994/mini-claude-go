// Package timings provides startup timing instrumentation.
// Aligned to pi's timings.ts.
// Only active when TIMING=1 environment variable is set.
package timings

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	entries []timingEntry
	enabled bool
)

type timingEntry struct {
	Label string
	Ts    time.Time
	Since time.Duration
}

func init() {
	enabled = os.Getenv("TIMING") == "1"
}

// IsEnabled returns whether timing instrumentation is active.
func IsEnabled() bool {
	return enabled
}

// Reset clears all recorded timings.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	entries = nil
}

// Time records a timing point with the given label.
// If timing is not enabled, this is a no-op.
func Time(label string) {
	if !enabled {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	var since time.Duration
	if len(entries) > 0 {
		since = now.Sub(entries[len(entries)-1].Ts)
	}
	entries = append(entries, timingEntry{
		Label: label,
		Ts:    now,
		Since: since,
	})
}

// Print outputs all recorded timings to stderr.
func Print() {
	if !enabled {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	if len(entries) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("=== Startup Timings ===\n")
	for i, e := range entries {
		if i == 0 {
			fmt.Fprintf(&b, "  %s: %s\n", e.Label, e.Ts.Format("15:04:05.000"))
		} else {
			fmt.Fprintf(&b, "  %s: %s (+%s)\n", e.Label, e.Ts.Format("15:04:05.000"), e.Since.Round(time.Microsecond))
		}
	}
	if len(entries) > 1 {
		total := entries[len(entries)-1].Ts.Sub(entries[0].Ts)
		fmt.Fprintf(&b, "  Total: %s\n", total.Round(time.Microsecond))
	}
	fmt.Fprint(os.Stderr, b.String())
}

// Entries returns a copy of the timing entries.
func Entries() []timingEntry {
	mu.Lock()
	defer mu.Unlock()
	result := make([]timingEntry, len(entries))
	copy(result, entries)
	return result
}
