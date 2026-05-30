//go:build !windows

// Package repl provides REPL functionality with signal handling.
package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

var lastInterrupt atomic.Int64 // timestamp of last Ctrl+C for double-press exit detection

// InstallCtrlCHandler is a no-op on non-Windows platforms.
// On Unix, Ctrl+C is handled via signal.Notify in the REPL.
func InstallCtrlCHandler() {}

// SetInterruptHandler is a no-op on non-Windows platforms.
// Signal handling is done directly in the REPL loop.
func SetInterruptHandler(fn func()) {}

// EnsureConsoleInputMode is a no-op on non-Windows platforms.
func EnsureConsoleInputMode() {}

// ReadLine reads a line from stdin.
func ReadLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return line, err
	}
	return line, nil
}

// ReadLineInterruptible reads a line from stdin using ReadString.
// Context cancellation is checked between reads.
func ReadLineInterruptible(ctx context.Context) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	resultChan := make(chan struct {
		line string
		err  error
	}, 1)

	go func() {
		line, err := reader.ReadString('\n')
		resultChan <- struct {
			line string
			err  error
		}{line, err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-resultChan:
		return result.line, result.err
	}
}

// CheckDoubleInterrupt returns true if two interrupts happened within 1.5 seconds.
// Used by the REPL to implement double-press-to-exit behavior on Unix.
func CheckDoubleInterrupt() bool {
	now := time.Now().UnixMilli()
	last := lastInterrupt.Load()
	if last > 0 && now-last < 1500 {
		return true
	}
	lastInterrupt.Store(now)
	return false
}