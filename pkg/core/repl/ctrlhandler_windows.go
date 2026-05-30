//go:build windows

// Package repl provides REPL functionality with signal handling.
package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler  = kernel32.NewProc("SetConsoleCtrlHandler")
	procGetStdHandle          = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode        = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode        = kernel32.NewProc("SetConsoleMode")
	procWaitForSingleObject   = kernel32.NewProc("WaitForSingleObject")
	procReadFile              = kernel32.NewProc("ReadFile")
)

const (
	STD_INPUT_HANDLE              = ^uintptr(9)
	ENABLE_PROCESSED_INPUT        = 0x0001
	ENABLE_LINE_INPUT             = 0x0002
	ENABLE_ECHO_INPUT             = 0x0004
	ENABLE_EXTENDED_FLAGS         = 0x0080
	ENABLE_VIRTUAL_TERMINAL_INPUT = 0x0200
)

// interruptHandler is set by the REPL to handle Ctrl+C.
var interruptHandler atomic.Pointer[func()]

var lastInterrupt atomic.Int64    // timestamp of last Ctrl+C for double-press exit detection
var interruptPending atomic.Int64 // set when Ctrl+C is pending (ReadLine should return canceled)

// initLastInterrupt ensures the atomic is properly initialized.
func init() {
	lastInterrupt.Store(0)
}

// InstallCtrlCHandler installs SetConsoleCtrlHandler to prevent process
// termination on Ctrl+C. Must be called from main(), NOT from init().
func InstallCtrlCHandler() {
	callback := syscall.NewCallback(func(ctrlType uintptr) uintptr {
		switch ctrlType {
		case 0, 1: // CTRL_C_EVENT, CTRL_BREAK_EVENT
			now := time.Now().UnixMilli()
			last := lastInterrupt.Load()
			if last > 0 && now-last < 1500 {
				// Double Ctrl+C within 1.5s — exit immediately
				os.Exit(0)
			}
			lastInterrupt.Store(now)
			// Signal ReadLine to return canceled
			interruptPending.Store(1)
			// Call the interrupt handler
			if fn := interruptHandler.Load(); fn != nil {
				(*fn)()
			}
			return 1 // TRUE — prevent default termination
		default:
			return 0
		}
	})
	procSetConsoleCtrlHandler.Call(callback, 1)
}

// SetInterruptHandler sets the callback invoked by SetConsoleCtrlHandler on Ctrl+C.
func SetInterruptHandler(fn func()) {
	interruptHandler.Store(&fn)
}

// EnsureConsoleInputMode ensures the Windows console input mode has the
// required flags for interactive input.
func EnsureConsoleInputMode() {
	h, _, _ := procGetStdHandle.Call(uintptr(STD_INPUT_HANDLE))
	if h == 0 {
		return
	}

	var mode uint32
	ret, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	needMode := mode
	needMode |= ENABLE_PROCESSED_INPUT
	needMode |= ENABLE_LINE_INPUT
	needMode |= ENABLE_ECHO_INPUT
	needMode &^= ENABLE_VIRTUAL_TERMINAL_INPUT
	needMode |= ENABLE_EXTENDED_FLAGS

	if needMode != mode {
		procSetConsoleMode.Call(h, uintptr(needMode))
	}
}

// ReadLine reads a line from stdin using Windows API ReadFile directly.
// This allows Ctrl+C to be handled via SetConsoleCtrlHandler.
func ReadLine() (string, error) {
	// Clear any pending interrupt flag at start
	interruptPending.Store(0)
	
	// Get console input handle
	h, _, _ := procGetStdHandle.Call(uintptr(0xFFFFFFF6)) // STD_INPUT_HANDLE
	if h == 0 {
		return "", io.EOF
	}
	
	var buf []byte
	var tmp [1]byte

	for {
		// Wait for input with 100ms timeout to allow Ctrl+C handler to run
		ret, _, _ := procWaitForSingleObject.Call(h, 100)
		
		// Check if Ctrl+C was pressed (interruptPending flag set by handler)
		if interruptPending.Load() == 1 {
			interruptPending.Store(0)
			return string(buf), context.Canceled
		}
		
		if ret == 0 { // _WAIT_OBJECT_0 - input available
			var bytesRead uint32
			res, _, _ := procReadFile.Call(h, uintptr(unsafe.Pointer(&tmp[0])), 1, uintptr(unsafe.Pointer(&bytesRead)), 0)
			if res == 0 {
				return string(buf), fmt.Errorf("ReadFile failed")
			}
			if bytesRead > 0 {
				if tmp[0] == '\n' {
					return string(buf), nil
				}
				if tmp[0] != '\r' {
					buf = append(buf, tmp[0])
				}
			}
		} else if ret == 0x102 { // _WAIT_TIMEOUT
			continue
		} else {
			// Error
			return string(buf), fmt.Errorf("WaitForSingleObject returned %d", ret)
		}
	}
}

// ReadLineInterruptible reads a line from stdin with context cancellation support.
// Uses a goroutine to allow context cancellation to interrupt the read.
func ReadLineInterruptible(ctx context.Context) (string, error) {
	resultChan := make(chan struct {
		line string
		err  error
	}, 1)

	go func() {
		var line string
		_, err := fmt.Scanln(&line)
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
// On Unix, this implements the double-press-to-exit behavior.
// On Windows, use GetLastInterrupt/SetLastInterrupt instead.
func CheckDoubleInterrupt() bool {
	now := time.Now().UnixMilli()
	last := lastInterrupt.Load()
	if last > 0 && now-last < 1500 {
		return true
	}
	lastInterrupt.Store(now)
	return false
}

// GetLastInterrupt returns the timestamp of the last Ctrl+C interrupt.
func GetLastInterrupt() int64 {
	return lastInterrupt.Load()
}

// SetLastInterrupt stores the timestamp of the last Ctrl+C interrupt.
func SetLastInterrupt(ts int64) {
	lastInterrupt.Store(ts)
}