//go:build windows && !wasm

package utils

import (
	"syscall"
	"unsafe"
)

var (
	copyKernel32  = syscall.NewLazyDLL("kernel32.dll")
	procCopyFileW = copyKernel32.NewProc("CopyFileW")
)

// CopyFileInternal uses the Windows CopyFileW syscall.
func CopyFileInternal(src, dst string) error {
	srcPtr, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstPtr, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}

	// BOOL CopyFileW(LPCWSTR lpExistingFileName, LPCWSTR lpNewFileName, BOOL bFailIfExists);
	ret, _, err := procCopyFileW.Call(
		uintptr(unsafe.Pointer(srcPtr)),
		uintptr(unsafe.Pointer(dstPtr)),
		0, // false: do not fail if destination exists
	)
	if ret == 0 {
		return err // err will contain the system error code
	}
	return nil
}
