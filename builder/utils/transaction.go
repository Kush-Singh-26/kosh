package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// BuildTransaction manages the output directory lifecycle and ensures atomic publishes
type BuildTransaction interface {
	StagingDir() string
	Commit() error
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
			_ = os.RemoveAll(filepath.Join(parent, name))
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

func (tx *DirectoryTx) Commit() error {
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
		if err := RenameWithRetry(tx.realOutputDir, backupDir, 12, 20*time.Millisecond); err != nil {
			return fmt.Errorf("failed to backup output directory: %w", err)
		}
	}

	// 2. Rename outputDir.tmp -> outputDir
	if err := RenameWithRetry(tx.stagingDir, tx.realOutputDir, 12, 20*time.Millisecond); err != nil {
		// Attempt to restore backup on failure
		if backupDir != "" {
			_ = RenameWithRetry(backupDir, tx.realOutputDir, 12, 20*time.Millisecond)
		}
		return fmt.Errorf("failed to publish staging directory: %w", err)
	}

	// 3. Delete backup dir in the background
	if backupDir != "" {
		go func() {
			_ = os.RemoveAll(backupDir)
		}()
	}

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
func RenameWithRetry(oldPath, newPath string, maxRetries int, baseDelay time.Duration) error {
	var err error
	for i := range maxRetries {
		err = os.Rename(oldPath, newPath)
		if err == nil {
			return nil
		}

		// Use a capped backoff with jitter
		delay := min(baseDelay*time.Duration(1<<uint(i)), 2*time.Second)
		// Simple jitter without math/rand: ±10%
		jitter := time.Duration(time.Now().UnixNano() % int64(delay/5+1))
		time.Sleep(delay + jitter)
	}
	return err
}
