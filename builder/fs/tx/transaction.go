// Package tx provides build transaction management for atomic directory operations.
package tx

import (
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"

	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/retry"
)

// BuildTransaction manages the output directory lifecycle and ensures atomic publishes
type BuildTransaction interface {
	StagingDir() string
	Commit(ctx context.Context) error
	Rollback() error
	GetLastBuildTime() time.Time
}

// DirectoryTx implements BuildTransaction with staging/backup directories
type DirectoryTx struct {
	realOutputDir string
	stagingDir    string
	backupDir     string
	isCleanBuild  bool
	committed     bool
}

var buildTxnCounter atomic.Uint64

// CleanupStaleBuildDirs removes stale staging and backup directories from previous builds.
// This should be called explicitly before NewBuildTransaction if cleanup is desired.
// Only removes directories older than 1 hour to avoid deleting active builds.
func CleanupStaleBuildDirs(outputDir string) {
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
					_ = retry.RemoveAllWithRetry(context.Background(), fullPath, 5, 10*time.Millisecond)
				}
			}
		}
	}
}

// NewBuildTransaction initializes a transaction. If isCleanBuild is true, it uses a .tmp directory for staging.
// Caller is responsible for calling CleanupStaleBuildDirs(outputDir) before this function if cleanup is desired.
func NewBuildTransaction(outputDir string, isCleanBuild bool) *DirectoryTx {
	outputDir = fspkg.NormalizePath(outputDir)
	var stagingDir string
	var backupDir string
	if isCleanBuild {
		// Cleanup is now caller's responsibility - call CleanupStaleBuildDirs(outputDir) explicitly before this
		// Clean up old backup directories from previous builds to reduce I/O pressure during publish
		parent := filepath.Dir(outputDir)
		base := filepath.Base(outputDir)
		entries, _ := os.ReadDir(parent)
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, base+".bak-") {
				fullPath := filepath.Join(parent, name)
				_ = retry.RemoveAllWithRetry(context.Background(), fullPath, 5, 10*time.Millisecond)
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
		_ = retry.RemoveAllWithRetry(ctx, backupDir, 5, 10*time.Millisecond)
		if err := retry.RenameWithRetry(retry.RenameOptions{
			Ctx:        ctx,
			OldPath:    tx.realOutputDir,
			NewPath:    backupDir,
			MaxRetries: 12,
			BaseDelay:  20 * time.Millisecond,
		}); err != nil {
			return fmt.Errorf("failed to backup output directory: %w", err)
		}
	}

	// 2. Rename outputDir.tmp -> outputDir
	if err := retry.RenameWithRetry(retry.RenameOptions{
		Ctx:        ctx,
		OldPath:    tx.stagingDir,
		NewPath:    tx.realOutputDir,
		MaxRetries: 12,
		BaseDelay:  20 * time.Millisecond,
	}); err != nil {
		// Attempt to restore backup on failure
		var rollbackErr error
		if backupDir != "" {
			rollbackErr = retry.RenameWithRetry(retry.RenameOptions{
				Ctx:        ctx,
				OldPath:    backupDir,
				NewPath:    tx.realOutputDir,
				MaxRetries: 12,
				BaseDelay:  20 * time.Millisecond,
			})
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

	// 3. Commit complete. Remove backup directory as it's no longer needed for rollback.
	if tx.backupDir != "" {
		_ = retry.RemoveAllWithRetry(ctx, tx.backupDir, 5, 10*time.Millisecond)
	}
	tx.committed = true
	return nil
}

func (tx *DirectoryTx) Rollback() error {
	if tx.committed || !tx.isCleanBuild {
		return nil
	}
	// Clean up staging dir on failure
	return retry.RemoveAllWithRetry(context.Background(), tx.stagingDir, 5, 10*time.Millisecond)
}

func (tx *DirectoryTx) GetLastBuildTime() time.Time {
	info, err := os.Stat(tx.realOutputDir)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
