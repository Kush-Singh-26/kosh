//go:build !windows && !linux && !wasm && !js

package utils

import (
	"io"
	"os"
)

// CopyFileInternal is a fallback that uses standard io.CopyBuffer
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

	buf := make([]byte, 64*1024)
	_, err = io.CopyBuffer(out, in, buf)
	return err
}
