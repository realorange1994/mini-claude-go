package tools

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/term"
)

// readLineHidden reads a line from stdin with hidden input (password mode).
// It uses golang.org/x/term.ReadPassword which works on both Unix and Windows.
// Falls back to readLineWithContext if stdin is not a terminal.
// The returned string does NOT include the trailing newline.
func readLineHidden(ctx context.Context) (string, error) {
	// Check if stdin is a terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Not a terminal (piped input, redirected, etc.) - use regular read
		return readLineWithContext(ctx)
	}

	// Use a goroutine to make ReadPassword cancellable via context
	type result struct {
		line []byte
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		line, err := term.ReadPassword(int(os.Stdin.Fd()))
		// ReadPassword consumes the input but doesn't print a newline,
		// so the next output would be on the same line. Print a newline.
		fmt.Println()
		resultCh <- result{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - we can't interrupt ReadPassword,
		// but we can return early. The goroutine will eventually
		// finish and send to resultCh (which will be discarded).
		return "", ctx.Err()
	case r := <-resultCh:
		if r.err != nil {
			return "", r.err
		}
		return string(r.line), nil
	}
}
