//go:build windows

package main

import (
	"os"
	"syscall"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
	procUnlockFile = modkernel32.NewProc("UnlockFile")
)

const (
	_LOCKFILE_EXCLUSIVE_LOCK = 0x00000002
)

// lockFileEx acquires an exclusive lock on the given file handle using
// Windows syscall LockFileEx via kernel32.dll.
func lockFileEx(f *os.File, exclusive bool) error {
	h := syscall.Handle(f.Fd())
	var flags uint32
	if exclusive {
		flags = _LOCKFILE_EXCLUSIVE_LOCK
	}
	// LockFileEx(handle, flags, 0, 0xFFFFFFFF, 0xFFFFFFFF, nil)
	r1, _, err := syscall.Syscall6(
		procLockFileEx.Addr(),
		6,
		uintptr(h),
		uintptr(flags),
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		0, // NULL for synchronous operation
	)
	if r1 == 0 {
		return err
	}
	return nil
}

// unlockFileEx releases a previously acquired lock. We use UnlockFile
// (not UnlockFileEx) since it doesn't require the exact same overlap struct.
func unlockFileEx(f *os.File, exclusive bool) error {
	h := syscall.Handle(f.Fd())
	// UnlockFile(handle, 0, 0, 0xFFFFFFFF, 0xFFFFFFFF)
	r1, _, err := syscall.Syscall6(
		procUnlockFile.Addr(),
		5,
		uintptr(h),
		0,
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		0,
	)
	if r1 == 0 {
		return err
	}
	return nil
}
