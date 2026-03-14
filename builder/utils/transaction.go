package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// BuildTransaction manages the output directory lifecycle and ensures atomic publishes
type BuildTransaction interface {
	StagingDir() string
	Commit(ctx context.Context) error
	Rollback() error
	GetLastBuildTime() time.Time
}

type DirectoryTx struct {
	realOutputDir string
	stagingDir    string
	backupDir     string
	isCleanBuild  bool
	committed     bool
}

var buildTxnCounter atomic.Uint64

func cleanupStaleBuildDirs(outputDir string) {
	parent := filepath.Dir(outputDir)
	base := filepath.Base(outputDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == base || name == base+".bak" {
			continue
		}
		if strings.HasPrefix(name, base+".tmp-") || strings.HasPrefix(name, base+".bak-") {
			fullPath := filepath.Join(parent, name)
			if info, err := os.Stat(fullPath); err == nil {
				// Only cleanup if older than 1 hour to avoid deleting active staging dirs from other processes
				if time.Since(info.ModTime()) > time.Hour {
					_ = os.RemoveAll(fullPath)
				}
			}
		}
	}
}

// NewBuildTransaction initializes a transaction. If isCleanBuild is true, it uses a .tmp directory for staging.
func NewBuildTransaction(outputDir string, isCleanBuild bool) *DirectoryTx {
	outputDir = filepath.Clean(outputDir)
	var stagingDir string
	var backupDir string
	if isCleanBuild {
		cleanupStaleBuildDirs(outputDir)
		// Clean up old backup directories from previous builds to reduce I/O pressure during publish
		parent := filepath.Dir(outputDir)
		base := filepath.Base(outputDir)
		entries, _ := os.ReadDir(parent)
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, base+".bak-") {
				fullPath := filepath.Join(parent, name)
				_ = os.RemoveAll(fullPath)
			}
		}
		ts := fmt.Sprintf("%d-%d", time.Now().UnixNano(), buildTxnCounter.Add(1))
		stagingDir = fmt.Sprintf("%s.tmp-%s", outputDir, ts)
		backupDir = fmt.Sprintf("%s.bak-%s", outputDir, ts)
	} else {
		stagingDir = outputDir // directly write to output in watch mode
	}

	return &DirectoryTx{
		realOutputDir: outputDir,
		stagingDir:    stagingDir,
		backupDir:     backupDir,
		isCleanBuild:  isCleanBuild,
	}
}

func (tx *DirectoryTx) StagingDir() string {
	return tx.stagingDir
}

func (tx *DirectoryTx) Commit(ctx context.Context) error {
	if tx.committed {
		return nil
	}
	if !tx.isCleanBuild {
		tx.committed = true
		return nil // No swap needed for incremental builds
	}

	// 1. Rename outputDir -> outputDir.bak (if it exists)
	backupDir := tx.backupDir
	if _, err := os.Stat(tx.realOutputDir); err == nil {
		// Try to remove old backup if it somehow exists
		_ = os.RemoveAll(backupDir)
		if err := RenameWithRetry(ctx, tx.realOutputDir, backupDir, 12, 20*time.Millisecond); err != nil {
			return fmt.Errorf("failed to backup output directory: %w", err)
		}
	}

	// 2. Rename outputDir.tmp -> outputDir
	if err := RenameWithRetry(ctx, tx.stagingDir, tx.realOutputDir, 12, 20*time.Millisecond); err != nil {
		// Attempt to restore backup on failure
		var rollbackErr error
		if backupDir != "" {
			rollbackErr = RenameWithRetry(ctx, backupDir, tx.realOutputDir, 12, 20*time.Millisecond)
			if rollbackErr != nil {
				slog.Error("CRITICAL: Both publish and rollback failed",
					"publish_error", err,
					"rollback_error", rollbackErr,
					"staging_dir", tx.stagingDir,
					"backup_dir", backupDir)
			}
		}
		if rollbackErr != nil {
			return fmt.Errorf("failed to publish staging directory: %w (rollback also failed: %w)", err, rollbackErr)
		}
		return fmt.Errorf("failed to publish staging directory: %w (rolled back successfully)", err)
	}

	// 3. Commit complete
	tx.committed = true
	return nil
}

func (tx *DirectoryTx) Rollback() error {
	if tx.committed || !tx.isCleanBuild {
		return nil
	}
	// Clean up staging dir on failure
	return os.RemoveAll(tx.stagingDir)
}

func (tx *DirectoryTx) GetLastBuildTime() time.Time {
	info, err := os.Stat(tx.realOutputDir)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

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
