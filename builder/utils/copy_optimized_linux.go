//go:build linux && !wasm

package utils

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// CopyFileInternal uses the Linux copy_file_range syscall.
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
			// Fallback to manual copy if copy_file_range fails (e.g. cross-filesystem)
			buf := make([]byte, 64*1024)
			_, err = in.Seek(info.Size()-size, 0)
			if err != nil {
				return err
			}
			_, err = out.Seek(info.Size()-size, 0)
			if err != nil {
				return err
			}
			for size > 0 {
				nr, er := in.Read(buf)
				if nr > 0 {
					nw, ew := out.Write(buf[0:nr])
					if nw < 0 || nr < nw {
						nw = 0
						if ew == nil {
							ew = os.ErrInvalid
						}
					}
					size -= int64(nw)
					if ew != nil {
						return ew
					}
					if nr != nw {
						return io.ErrShortWrite
					}
				}
				if er != nil {
					if er == io.EOF {
						break
					}
					return er
				}
			}
			return nil
		}
		if n == 0 {
			break
		}
		size -= int64(n)
	}
	return nil
}
