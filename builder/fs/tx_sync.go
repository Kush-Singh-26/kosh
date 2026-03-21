package fs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// TxSync provides transactional file-system sync with rollback capability.
// It tracks files written during a sync pass and can undo them if any
// part of the sync fails, preventing partially-written output directories.
type TxSync struct {
	mu         sync.Mutex
	written    []string          // new files created in this transaction
	writtenSet map[string]bool   // O(1) duplicate detection
	backups    map[string]string // original path -> backup path (for overwrites)
	committed  bool
	logger     *slog.Logger
}

// NewTxSync creates a new transactional sync tracker.
func NewTxSync(logger *slog.Logger) *TxSync {
	return &TxSync{
		backups:    make(map[string]string),
		writtenSet: make(map[string]bool),
		logger:     logger,
	}
}

// TrackWrite records a file about to be written.
// It performs a lazy backup of the existing file only if it exists.
func (tx *TxSync) TrackWrite(osPath string) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.writtenSet[osPath] {
		return nil
	}
	tx.writtenSet[osPath] = true

	if _, err := os.Stat(osPath); err == nil {
		backupPath := osPath + ".kosh-rollback"
		if err := StreamCopyFile(osPath, backupPath); err != nil {
			return fmt.Errorf("backup failed for %s: %w", osPath, err)
		}
		tx.backups[osPath] = backupPath
	}

	tx.written = append(tx.written, osPath)
	return nil
}

// Commit finalizes the transaction, removes all backup files.
// After Commit, Rollback becomes a no-op.
func (tx *TxSync) Commit() {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	tx.committed = true

	for _, backup := range tx.backups {
		_ = os.Remove(backup)
	}

	if tx.logger != nil && len(tx.written) > 0 {
		tx.logger.Debug("TxSync committed", "files", len(tx.written))
	}
}

// Rollback restores all backed-up files and removes newly created files.
// This is safe to call after Commit (becomes a no-op).
// Uses retry logic with context support for Windows robustness.
func (tx *TxSync) Rollback(ctx context.Context) {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return
	}

	rolled := 0

	for original, backup := range tx.backups {
		if err := renameWithRetry(ctx, backup, original); err != nil {
			if tx.logger != nil {
				tx.logger.Warn("TxSync rollback: failed to restore backup",
					"original", original, "backup", backup, "error", err)
			}
		} else {
			rolled++
		}
	}

	for _, path := range tx.written {
		if _, hasBackup := tx.backups[path]; !hasBackup {
			if err := removeAllWithRetry(ctx, path); err != nil && !os.IsNotExist(err) {
				if tx.logger != nil {
					tx.logger.Warn("TxSync rollback: failed to remove new file",
						"path", path, "error", err)
				}
			} else {
				rolled++
			}
		}
	}

	for _, backup := range tx.backups {
		_ = os.Remove(backup)
	}

	if tx.logger != nil {
		tx.logger.Info("TxSync rolled back", "restored", rolled, "total_tracked", len(tx.written))
	}
}

func (tx *TxSync) FileCount() int {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return len(tx.written)
}

func (tx *TxSync) IsCommitted() bool {
	tx.mu.Lock()
	defer tx.mu.Unlock()
	return tx.committed
}

// renameWithRetry attempts to rename a path with exponential backoff.
// Uses 12 retries with 20ms base delay - tuned for Windows file locking scenarios.
func renameWithRetry(ctx context.Context, oldPath, newPath string) error {
	const maxRetries = 12
	const baseDelayMs = 20

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
			slog.Debug("TxSync rename failed, retrying with backoff...", "old", oldPath, "new", newPath, "error", err)
		}

		delay := min(baseDelayMs*1<<uint(i), 2000)
		jitter := time.Duration(time.Now().UnixNano()%int64(delay/5+1)) * time.Nanosecond

		timer := time.NewTimer(time.Duration(delay)*time.Millisecond + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

// removeAllWithRetry attempts to remove a directory tree with exponential backoff.
// Uses 12 retries with 20ms base delay - tuned for Windows file locking scenarios.
func removeAllWithRetry(ctx context.Context, path string) error {
	const maxRetries = 12
	const baseDelayMs = 20

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
			slog.Debug("TxSync RemoveAll failed, retrying with backoff...", "path", path, "error", err)
		}

		delay := min(baseDelayMs*1<<uint(i), 2000)
		jitter := time.Duration(time.Now().UnixNano()%int64(delay/5+1)) * time.Nanosecond

		timer := time.NewTimer(time.Duration(delay)*time.Millisecond + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
