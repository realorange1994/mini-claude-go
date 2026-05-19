//go:build windows

package tools

import (
	"context"
	"os"
	"syscall"
)

var (
	modkernel32       = syscall.NewLazyDLL("kernel32.dll")
	procWaitForSingle = modkernel32.NewProc("WaitForSingleObject")
)

const (
	_WAIT_OBJECT_0  = 0
	_WAIT_TIMEOUT   = 0x00000102
	_WAIT_FAILED    = 0xFFFFFFFF
	_WAIT_ABANDONED = 0x00000080
)

// readLineWithContext reads a line from stdin with context cancellation support.
// It uses WaitForSingleObject to poll stdin with a 100ms timeout between each byte,
// allowing context cancellation to interrupt the read without leaving orphaned
// goroutines that consume REPL input.
func readLineWithContext(ctx context.Context) (string, error) {
	h := syscall.Handle(os.Stdin.Fd())
	var line []byte
	buf := make([]byte, 1)

	for {
		select {
		case <-ctx.Done():
			return string(line), ctx.Err()
		default:
		}

		ret, _, _ := procWaitForSingle.Call(uintptr(h), 100) // 100ms timeout

		switch uint32(ret) {
		case _WAIT_OBJECT_0:
			// Input available
			nr, err := os.Stdin.Read(buf)
			if nr > 0 {
				if buf[0] == '\n' {
					return string(line), nil
				}
				if buf[0] == '\r' {
					continue
				}
				line = append(line, buf[0])
			}
			if err != nil {
				return string(line), err
			}
		case _WAIT_TIMEOUT:
			continue // retry
		case _WAIT_ABANDONED, _WAIT_FAILED:
			// Fall back to blocking read for non-console handles
			return readLineBlocking()
		}
	}
}

func readLineBlocking() (string, error) {
	buf := make([]byte, 0, 256)
	reader := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(reader)
		if n > 0 {
			if reader[0] == '\n' {
				return string(buf), nil
			}
			if reader[0] != '\r' {
				buf = append(buf, reader[0])
			}
		}
		if err != nil {
			return string(buf), err
		}
	}
}
