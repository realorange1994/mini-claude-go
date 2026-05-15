//go:build windows

package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

// ensureConsoleInputMode ensures the Windows console input mode has the
// required flags for interactive input. MCP child processes (node.exe) may
// alter the console input mode when they start, breaking ReadString.
func ensureConsoleInputMode() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const STD_INPUT_HANDLE = ^uintptr(10) // -10
	const (
		ENABLE_PROCESSED_INPUT        = 0x0001
		ENABLE_LINE_INPUT             = 0x0002
		ENABLE_ECHO_INPUT             = 0x0004
		ENABLE_EXTENDED_FLAGS         = 0x0080
		ENABLE_VIRTUAL_TERMINAL_INPUT = 0x0200
	)

	h, _, _ := getStdHandle.Call(uintptr(STD_INPUT_HANDLE))
	if h == 0 {
		return
	}

	var mode uint32
	ret, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	// Ensure the critical flags are set
	needMode := mode
	needMode |= ENABLE_PROCESSED_INPUT
	needMode |= ENABLE_LINE_INPUT
	needMode |= ENABLE_ECHO_INPUT
	needMode &^= ENABLE_VIRTUAL_TERMINAL_INPUT
	needMode |= ENABLE_EXTENDED_FLAGS

	if needMode != mode {
		setConsoleMode.Call(h, uintptr(needMode))
	}
}

// reopenStdin reopens the Windows console input device.
func reopenStdin() *bufio.Reader {
	f, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return bufio.NewReader(f)
}

// interruptStdin is a no-op on Windows. Ctrl+C already unblocks
// ReadString on Windows when console input mode is set correctly.
func interruptStdin() {}

// checkAndClearSigint always returns false on Windows.
func checkAndClearSigint() bool {
	return false
}

// readLineInterruptible reads a line from stdin.
func readLineInterruptible(stdinReader *bufio.Reader) (string, error) {
	return stdinReader.ReadString('\n')
}
