// Package outputguard provides stdout takeover to prevent other code
// from writing to stdout (which is reserved for structured output).
// Aligned to pi's output-guard.ts.
package outputguard

import (
	"io"
	"os"
	"sync"
)

var (
	mu           sync.Mutex
	takenOver    bool
	origStdout   *os.File
	rawStdoutMu  sync.Mutex
)

// TakeOverStdout redirects os.Stdout to os.Stderr so that stray prints
// don't corrupt the structured JSON output on stdout.
// Call RestoreStdout to undo.
func TakeOverStdout() {
	mu.Lock()
	defer mu.Unlock()

	if takenOver {
		return
	}

	origStdout = os.Stdout
	// Redirect stdout to stderr
	os.Stdout = os.Stderr
	takenOver = true
}

// RestoreStdout restores the original os.Stdout after a TakeOverStdout call.
func RestoreStdout() {
	mu.Lock()
	defer mu.Unlock()

	if !takenOver {
		return
	}

	os.Stdout = origStdout
	takenOver = false
}

// IsStdoutTakenOver returns whether stdout is currently redirected.
func IsStdoutTakenOver() bool {
	mu.Lock()
	defer mu.Unlock()
	return takenOver
}

// WriteRawStdout writes text directly to the original stdout,
// bypassing the takeover redirect. Thread-safe.
func WriteRawStdout(text string) (int, error) {
	rawStdoutMu.Lock()
	defer rawStdoutMu.Unlock()

	if origStdout == nil {
		// Not taken over, write to current stdout
		return io.WriteString(os.Stdout, text)
	}
	return io.WriteString(origStdout, text)
}

// FlushRawStdout is a no-op on Go (writes are unbuffered at the os.File level).
// Provided for API compatibility with the TS version.
func FlushRawStdout() error {
	return nil
}
