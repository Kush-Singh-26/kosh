package utils

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// RenameWithRetry attempts to rename a path with exponential backoff.
// Critical for Windows where antivirus/indexers can briefly lock directories.
func RenameWithRetry(ctx context.Context, oldPath, newPath string, maxRetries int, baseDelay time.Duration) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = os.Rename(oldPath, newPath)
		if err == nil {
			return nil
		}
		if os.IsNotExist(err) {
			return err
		}

		if i == 0 {
			slog.Debug("Rename failed, retrying with backoff...", "old", oldPath, "new", newPath, "error", err)
		}

		// Use a capped backoff with jitter
		delay := min(baseDelay*time.Duration(1<<uint(i)), 2*time.Second)
		// Simple jitter without math/rand: ±10%
		jitter := time.Duration(time.Now().UnixNano() % int64(delay/5+1))

		timer := time.NewTimer(delay + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

// RemoveAllWithRetry attempts to remove a directory tree with exponential backoff.
// Critical for Windows where antivirus/indexers can briefly lock files.
func RemoveAllWithRetry(ctx context.Context, path string, maxRetries int, baseDelay time.Duration) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = os.RemoveAll(path)
		if err == nil {
			return nil
		}

		if i == 0 {
			slog.Debug("RemoveAll failed, retrying with backoff...", "path", path, "error", err)
		}

		// Use a capped backoff with jitter
		delay := min(baseDelay*time.Duration(1<<uint(i)), 2*time.Second)
		jitter := time.Duration(time.Now().UnixNano() % int64(delay/5+1))

		timer := time.NewTimer(delay + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
