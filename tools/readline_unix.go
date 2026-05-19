//go:build !windows

package tools

import (
	"context"
	"os"
	"syscall"
)

// readLineWithContext reads a line from stdin with context cancellation support.
// It polls stdin using syscall.Select with a 100ms timeout between each byte,
// allowing context cancellation to interrupt the read without leaving orphaned
// goroutines that consume REPL input.
func readLineWithContext(ctx context.Context) (string, error) {
	fd := int(os.Stdin.Fd())
	var line []byte
	buf := make([]byte, 1)

	for {
		select {
		case <-ctx.Done():
			return string(line), ctx.Err()
		default:
		}

		rfds := &syscall.FdSet{}
		rfds.Bits[fd/64] |= 1 << (uint(fd) % 64)
		tv := &syscall.Timeval{Sec: 0, Usec: 100000} // 100ms

		n, err := syscall.Select(fd+1, rfds, nil, nil, tv)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return string(line), err
		}

		isSet := rfds.Bits[fd/64]&(1<<(uint(fd)%64)) != 0
		if n > 0 && isSet {
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
		}
	}
}
