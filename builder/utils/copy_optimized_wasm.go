//go:build wasm || js

package utils

import "errors"

// CopyFileInternal is a no-op for WASM/JS
func CopyFileInternal(src, dst string) error {
	return errors.New("CopyFileInternal not implemented for wasm/js")
}
