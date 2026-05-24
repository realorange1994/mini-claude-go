//go:build !windows

package main

import "os"

// [STUB] lockFileEx is a no-op on Unix platforms. Advisory locking is not reliably
// implemented across all Unix-like systems. Concurrent writes rely on OS
// write semantics (atomic for < PIPE_BUF bytes).
func lockFileEx(f *os.File, exclusive bool) error {
	return nil
}

// [STUB] unlockFileEx is a no-op on Unix platforms.
func unlockFileEx(f *os.File, exclusive bool) error {
	return nil
}
