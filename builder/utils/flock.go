package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type FileLock struct {
	file *os.File
	path string
}

func AcquireBuildLock(outputDir string) (*FileLock, error) {
	lockPath := filepath.Join(outputDir, ".kosh-build.lock")

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	// Non-blocking lock - fail fast if another build is running
	if err := tryLock(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("another build is in progress (lock file: %s)", lockPath)
	}

	// Write PID for debugging
	pid := fmt.Sprintf("%d\n%s", os.Getpid(), time.Now().Format(time.RFC3339))
	_, _ = file.WriteAt([]byte(pid), 0)

	return &FileLock{file: file, path: lockPath}, nil
}

func (fl *FileLock) Release() error {
	if fl.file == nil {
		return nil
	}

	// Unlock before close
	_ = unlock(fl.file)
	err := fl.file.Close()
	fl.file = nil

	// Best effort cleanup
	_ = os.Remove(fl.path)
	return err
}
