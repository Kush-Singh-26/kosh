package fs

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// FileLock represents a build lock held via a lock file.
type FileLock struct {
	file *os.File
	path string
}

// AcquireBuildLock acquires a non-blocking lock for the output directory.
func AcquireBuildLock(outputDir string) (*FileLock, error) {
	// Place the lock file adjacent to the output directory to prevent file locking
	// issues when renaming the output directory during atomic publish.
	lockPath := filepath.Clean(outputDir) + ".lock"

	// Ensure the parent directory of the lock file exists
	if err := os.MkdirAll(filepath.Dir(lockPath), defaultDirMode); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, defaultFileMode)
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
	if _, err := file.WriteAt([]byte(pid), 0); err != nil {
		slog.Warn("Failed to write PID to lock file", "path", lockPath, "error", err)
	}

	return &FileLock{file: file, path: lockPath}, nil
}

// Release releases the lock and removes the lock file.
func (fl *FileLock) Release() error {
	if fl == nil || fl.file == nil {
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
