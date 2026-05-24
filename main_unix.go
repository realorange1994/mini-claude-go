//go:build !windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
)

// [STUB] ensureConsoleInputMode is a no-op on non-Windows platforms.
func ensureConsoleInputMode() {}

// sigintFlag is set to 1 when SIGINT is received.
var sigintFlag int32

// interruptStdin sets the SIGINT flag.
// The select-based input loop will detect this.
func interruptStdin() {
	atomic.StoreInt32(&sigintFlag, 1)
}

// checkAndClearSigint checks if SIGINT was received and clears the flag.
func checkAndClearSigint() bool {
	return atomic.CompareAndSwapInt32(&sigintFlag, 1, 0)
}

// reopenStdin reopens the console input device on Unix via /dev/tty.
func reopenStdin() *bufio.Reader {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to reopen /dev/tty: %v\n", err)
		f, err = os.OpenFile("/dev/stdin", os.O_RDWR, 0)
		if err != nil {
			return nil
		}
	}
	return bufio.NewReader(f)
}

// readLineInterruptible reads a line from stdin using syscall.Select
// to poll with a timeout. This allows SIGINT detection via sigintFlag
// between polls, without relying on closing the fd to interrupt the read.
func readLineInterruptible(_ *bufio.Reader) (string, error) {
	fd := int(os.Stdin.Fd())
	var line []byte

	// Poll stdin until a complete line is available or SIGINT is received.
	for {
		// Check for SIGINT
		if checkAndClearSigint() {
			return "", fmt.Errorf("interrupted")
		}

		rfds := &syscall.FdSet{}
		rfds.Bits[fd/64] |= 1 << (uint(fd) % 64)
		tv := &syscall.Timeval{Sec: 0, Usec: 100000} // 100ms timeout

		n, err := syscall.Select(fd+1, rfds, nil, nil, tv)
		if err != nil {
			if err == syscall.EINTR {
				continue // interrupted by signal, retry
			}
			return "", err
		}

		isSet := rfds.Bits[fd/64]&(1<<(uint(fd)%64)) != 0
		if n > 0 && isSet {
			// Read one byte
			buf := make([]byte, 1)
			nr, err := os.Stdin.Read(buf)
			if err != nil {
				if err == syscall.EINTR {
					continue // interrupted, retry
				}
				return string(line), err
			}
			if nr == 0 {
				return string(line), nil
			}
			if buf[0] == '\n' {
				return string(line), nil
			}
			if buf[0] == '\r' {
				// Check for \r\n
				// Try to peek if next byte is \n
				rfds2 := &syscall.FdSet{}
				rfds2.Bits[fd/64] |= 1 << (uint(fd) % 64)
				tv2 := &syscall.Timeval{Sec: 0, Usec: 10000} // 10ms
				n2, _ := syscall.Select(fd+1, rfds2, nil, nil, tv2)
				isSet2 := rfds2.Bits[fd/64]&(1<<(uint(fd)%64)) != 0
				if n2 > 0 && isSet2 {
					buf2 := make([]byte, 1)
					nr2, _ := os.Stdin.Read(buf2)
					if nr2 == 0 || buf2[0] != '\n' {
						// Not \r\n, put back the byte
						if nr2 > 0 && buf2[0] != '\n' {
							line = append(line, buf2[0])
						}
					}
				}
				return string(line), nil
			}
			// Backspace handling
			if buf[0] == '\b' || buf[0] == 127 {
				if len(line) > 0 {
					line = line[:len(line)-1]
				}
				continue
			}
			line = append(line, buf[0])
		}
	}
}
