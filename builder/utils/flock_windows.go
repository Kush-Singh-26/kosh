//go:build windows

package utils

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	lockFileExProc = kernel32.NewProc("LockFileEx")
	unlockFileProc = kernel32.NewProc("UnlockFile")
)

const (
	LOCKFILE_EXCLUSIVE_LOCK   = 2
	LOCKFILE_FAIL_IMMEDIATELY = 1
)

func tryLock(file *os.File) error {
	var overlapped syscall.Overlapped

	// LockFileEx: exclusive, fail immediately
	// Lock entire file (0 to 0xFFFFFFFF for both low and high parts)
	ret, _, err := lockFileExProc.Call(
		uintptr(file.Fd()),
		uintptr(LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY),
		0,
		0xFFFFFFFF,
		0xFFFFFFFF,
		uintptr(unsafe.Pointer(&overlapped)),
	)

	if ret == 0 {
		return err
	}
	return nil
}

func unlock(file *os.File) error {
	// UnlockFile: unlock entire file
	ret, _, err := unlockFileProc.Call(
		uintptr(file.Fd()),
		0, 0, // offset low, high
		0xFFFFFFFF, 0xFFFFFFFF, // length low, high
	)

	if ret == 0 {
		return err
	}
	return nil
}
