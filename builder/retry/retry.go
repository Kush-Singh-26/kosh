package retry

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"
)

var (
	rngMutex sync.Mutex
	rng      = rand.New(rand.NewSource(time.Now().UnixNano()))
)

const (
	firstAttemptIndex = 0
	backoffCap        = 2 * time.Second
	jitterDivisor     = 5
	jitterOffset      = 1
)

// RenameOptions configures RenameWithRetry.
type RenameOptions struct {
	Ctx        context.Context
	OldPath    string
	NewPath    string
	MaxRetries int
	BaseDelay  time.Duration
}

// RenameWithRetry attempts to rename a path with exponential backoff.
// Critical for Windows where antivirus/indexers can briefly lock directories.
func RenameWithRetry(opts RenameOptions) error {
	ctx := opts.Ctx
	oldPath := opts.OldPath
	newPath := opts.NewPath
	maxRetries := opts.MaxRetries
	baseDelay := opts.BaseDelay

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

		if i == firstAttemptIndex {
			slog.Debug("Rename failed, retrying with backoff...", "old", oldPath, "new", newPath, "error", err)
		}

		// Use a capped backoff with jitter
		delay := min(baseDelay*time.Duration(1<<uint(i)), backoffCap)
		// Proper jitter using crypto/rand equivalent
		rngMutex.Lock()
		jitterN := rng.Int63n(int64(delay / jitterDivisor))
		rngMutex.Unlock()
		jitter := time.Duration(jitterN) + jitterOffset

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

		if i == firstAttemptIndex {
			slog.Debug("RemoveAll failed, retrying with backoff...", "path", path, "error", err)
		}

		// Use a capped backoff with jitter
		delay := min(baseDelay*time.Duration(1<<uint(i)), backoffCap)
		// Proper jitter using crypto/rand equivalent
		rngMutex.Lock()
		jitterN := rng.Int63n(int64(delay / jitterDivisor))
		rngMutex.Unlock()
		jitter := time.Duration(jitterN) + jitterOffset

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
