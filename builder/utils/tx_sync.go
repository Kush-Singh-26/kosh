package utils

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
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

	// O(1) duplicate detection
	if tx.writtenSet[osPath] {
		return nil
	}
	tx.writtenSet[osPath] = true

	// Check if the file already exists (overwrite case)
	if _, err := os.Stat(osPath); err == nil {
		backupPath := osPath + ".kosh-rollback"
		if err := streamCopyFile(osPath, backupPath); err != nil {
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

	// Clean up backup files
	for _, backup := range tx.backups {
		_ = os.Remove(backup)
	}

	if tx.logger != nil && len(tx.written) > 0 {
		tx.logger.Debug("TxSync committed", "files", len(tx.written))
	}
}

// Rollback restores all backed-up files and removes newly created files.
// This is safe to call after Commit (becomes a no-op).
func (tx *TxSync) Rollback() {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return
	}

	rolled := 0

	// Restore backed-up files (overwritten originals)
	for original, backup := range tx.backups {
		if err := os.Rename(backup, original); err != nil {
			if tx.logger != nil {
				tx.logger.Warn("TxSync rollback: failed to restore backup",
					"original", original, "backup", backup, "error", err)
			}
		} else {
			rolled++
		}
	}

	// Remove newly created files (those without backups)
	for _, path := range tx.written {
		if _, hasBackup := tx.backups[path]; !hasBackup {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				if tx.logger != nil {
					tx.logger.Warn("TxSync rollback: failed to remove new file",
						"path", path, "error", err)
				}
			} else {
				rolled++
			}
		}
	}

	// Clean up any remaining backup files
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

// streamCopyFile copies src to dst using buffered streaming for memory efficiency.
func streamCopyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	// 64KB buffer for efficient file copying
	bw := bufio.NewWriterSize(d, 64*1024)
	if _, err := io.Copy(bw, s); err != nil {
		return err
	}
	return bw.Flush()
}
