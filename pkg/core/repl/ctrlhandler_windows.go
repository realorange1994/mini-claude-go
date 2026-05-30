//go:build windows

// Package repl provides REPL functionality with signal handling.
package repl

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
	procGetStdHandle          = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode        = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode        = kernel32.NewProc("SetConsoleMode")
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

var lastCtrlC atomic.Int64

// InstallCtrlCHandler installs SetConsoleCtrlHandler to prevent process
// termination on Ctrl+C. Must be called from main(), NOT from init().
func InstallCtrlCHandler() {
	callback := syscall.NewCallback(func(ctrlType uintptr) uintptr {
		switch ctrlType {
		case 0, 1: // CTRL_C_EVENT, CTRL_BREAK_EVENT
			now := time.Now().UnixMilli()
			last := lastCtrlC.Load()
			if last > 0 && now-last < 1500 {
				os.Exit(0)
			}
			lastCtrlC.Store(now)
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

// ReadLine reads a line using fmt.Scanln which works reliably on Windows.
func ReadLine() (string, error) {
	var line string
	_, err := fmt.Scanln(&line)
	return line, err
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