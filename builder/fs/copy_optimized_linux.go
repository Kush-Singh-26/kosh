//go:build linux && !wasm

package fs

import (
	"os"

	"golang.org/x/sys/unix"
)

// CopyFileInternal uses the Linux copy_file_range syscall with pooled fallback.
func CopyFileInternal(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	for size > 0 {
		// Use unix.CopyFileRange for zero-copy transfer
		n, err := unix.CopyFileRange(int(in.Fd()), nil, int(out.Fd()), nil, int(size), 0)
		if err != nil {
			// Fallback to pooled stream copy for cross-filesystem or other errors
			return StreamCopyFile(src, dst)
		}
		if n == 0 {
			break
		}
		size -= int64(n)
	}
	return nil
}
