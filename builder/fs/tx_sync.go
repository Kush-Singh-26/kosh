package fs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/retry"
)

// TxSync provides transactional file-system sync with rollback capability.
// It tracks files written during a sync pass and can undo them if any
// part of the sync fails, preventing partially-written output directories.
type TxSync struct {
	mu          sync.Mutex        // protects written, writtenSet, backups, isCommitted
	written     []string          // new files created in this transaction
	writtenSet  map[string]bool   // O(1) duplicate detection
	backups     map[string]string // original path -> backup path (for overwrites)
	isCommitted bool
	logger      *slog.Logger
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
func (syncTx *TxSync) TrackWrite(osPath string) error {
	syncTx.mu.Lock()
	defer syncTx.mu.Unlock()

	if syncTx.writtenSet[osPath] {
		return nil
	}
	syncTx.writtenSet[osPath] = true

	if _, err := os.Stat(osPath); err == nil {
		backupPath := osPath + ".kosh-rollback"
		if err := StreamCopyFile(osPath, backupPath); err != nil {
			return fmt.Errorf("backup failed for %s: %w", osPath, err)
		}
		syncTx.backups[osPath] = backupPath
	}

	syncTx.written = append(syncTx.written, osPath)
	return nil
}

// Commit finalizes the transaction, removes all backup files.
// After Commit, Rollback becomes a no-op.
func (syncTx *TxSync) Commit() {
	syncTx.mu.Lock()
	defer syncTx.mu.Unlock()

	syncTx.isCommitted = true

	for _, backup := range syncTx.backups {
		_ = os.Remove(backup)
	}

	if syncTx.logger != nil && len(syncTx.written) > 0 {
		syncTx.logger.Debug("TxSync committed", "files", len(syncTx.written))
	}
}

// Rollback restores all backed-up files and removes newly created files.
// This is safe to call after Commit (becomes a no-op).
// Uses retry logic with context support for Windows robustness.
func (syncTx *TxSync) Rollback(ctx context.Context) {
	syncTx.mu.Lock()
	defer syncTx.mu.Unlock()

	if syncTx.isCommitted {
		return
	}

	rolled := 0

	for original, backup := range syncTx.backups {
		if err := retry.RenameWithRetry(retry.RenameOptions{
			Ctx:        ctx,
			OldPath:    backup,
			NewPath:    original,
			MaxRetries: txSyncMaxRetries,
			BaseDelay:  txSyncBaseDelay,
		}); err != nil {
			if syncTx.logger != nil {
				syncTx.logger.Warn("TxSync rollback: failed to restore backup",
					"path", original, "error", err)
			}
		} else {
			rolled++
		}
	}

	for _, path := range syncTx.written {
		if _, hasBackup := syncTx.backups[path]; !hasBackup {
			if err := retry.RemoveAllWithRetry(ctx, path, txSyncMaxRetries, txSyncBaseDelay); err != nil && !os.IsNotExist(err) {
				if syncTx.logger != nil {
					syncTx.logger.Warn("TxSync rollback: failed to remove new file",
						"path", path, "error", err)
				}
			} else {
				rolled++
			}
		}
	}

	for _, backup := range syncTx.backups {
		_ = os.Remove(backup)
	}

	if syncTx.logger != nil {
		syncTx.logger.Info("TxSync rolled back", "restored", rolled, "total_tracked", len(syncTx.written))
	}
}

// FileCount returns the number of files tracked in the transaction.
func (syncTx *TxSync) FileCount() int {
	syncTx.mu.Lock()
	defer syncTx.mu.Unlock()
	return len(syncTx.written)
}

// IsCommitted reports whether the transaction has been committed.
func (syncTx *TxSync) IsCommitted() bool {
	syncTx.mu.Lock()
	defer syncTx.mu.Unlock()
	return syncTx.isCommitted
}

const (
	txSyncMaxRetries = 12
	txSyncBaseDelay  = 20 * time.Millisecond
)
