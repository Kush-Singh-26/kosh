package utils

import (
	"fmt"
	"os"
	"path/filepath"
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
	isCleanBuild  bool
	committed     bool
}

// NewBuildTransaction initializes a transaction. If isCleanBuild is true, it uses a .tmp directory for staging.
func NewBuildTransaction(outputDir string, isCleanBuild bool) *DirectoryTx {
	outputDir = filepath.Clean(outputDir)
	var stagingDir string
	if isCleanBuild {
		stagingDir = outputDir + ".tmp"
	} else {
		stagingDir = outputDir // directly write to output in watch mode
	}

	return &DirectoryTx{
		realOutputDir: outputDir,
		stagingDir:    stagingDir,
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
	backupDir := tx.realOutputDir + ".bak"
	if _, err := os.Stat(tx.realOutputDir); err == nil {
		// Try to remove old backup if it somehow exists
		_ = os.RemoveAll(backupDir)
		if err := RenameWithRetry(tx.realOutputDir, backupDir, 5, 100*time.Millisecond); err != nil {
			return fmt.Errorf("failed to backup output directory: %w", err)
		}
	}

	// 2. Rename outputDir.tmp -> outputDir
	if err := RenameWithRetry(tx.stagingDir, tx.realOutputDir, 5, 100*time.Millisecond); err != nil {
		// Attempt to restore backup on failure
		_ = RenameWithRetry(backupDir, tx.realOutputDir, 5, 100*time.Millisecond)
		return fmt.Errorf("failed to publish staging directory: %w", err)
	}

	// 3. Delete backup dir in the background
	go func() {
		_ = os.RemoveAll(backupDir)
	}()

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
	for i := 0; i < maxRetries; i++ {
		err = os.Rename(oldPath, newPath)
		if err == nil {
			return nil
		}
		time.Sleep(baseDelay)
		baseDelay *= 2
	}
	return err
}
