//go:build !windows

// Package repl provides REPL functionality with signal handling.
package repl

import (
	"bufio"
	"context"
	"fmt"
	"os"
)

// InstallCtrlCHandler is a no-op on non-Windows platforms.
func InstallCtrlCHandler() {}

// SetInterruptHandler sets the callback for non-Windows platforms.
func SetInterruptHandler(fn func()) {}

// EnsureConsoleInputMode is a no-op on non-Windows platforms.
func EnsureConsoleInputMode() {}

// ReadLine reads a line from stdin.
// For Windows, we use fmt.Scanln instead (see ctrlhandler_windows.go).
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