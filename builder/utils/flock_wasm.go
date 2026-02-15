//go:build js && wasm

package utils

import (
	"os"
)

func tryLock(file *os.File) error {
	return nil
}

func unlock(file *os.File) error {
	return nil
}
