package tx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewBuildTransaction_UsesUniqueDirsForCleanBuild(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "public")
	tx1 := NewBuildTransaction(outputDir, true)
	tx2 := NewBuildTransaction(outputDir, true)

	if tx1.StagingDir() == tx2.StagingDir() {
		t.Fatalf("expected unique staging dirs, got %s", tx1.StagingDir())
	}
	if tx1.backupDir == tx2.backupDir {
		t.Fatalf("expected unique backup dirs, got %s", tx1.backupDir)
	}
}

func TestCleanupStaleBuildDirs_RemovesOldTempDirs(t *testing.T) {
	base := t.TempDir()
	outputDir := filepath.Join(base, "public")
	staleTmp := outputDir + ".tmp-123"
	staleBak := outputDir + ".bak-123"
	keepBak := outputDir + ".bak"

	if err := os.MkdirAll(staleTmp, 0755); err != nil {
		t.Fatalf("failed to create stale tmp dir: %v", err)
	}
	if err := os.MkdirAll(staleBak, 0755); err != nil {
		t.Fatalf("failed to create stale bak dir: %v", err)
	}
	if err := os.MkdirAll(keepBak, 0755); err != nil {
		t.Fatalf("failed to create keep bak dir: %v", err)
	}

	// Set stale directories to be older than 1 hour
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(staleTmp, oldTime, oldTime)
	_ = os.Chtimes(staleBak, oldTime, oldTime)

	cleanupStaleBuildDirs(outputDir)

	if _, err := os.Stat(staleTmp); !os.IsNotExist(err) {
		t.Fatalf("expected stale tmp dir removed")
	}
	if _, err := os.Stat(staleBak); !os.IsNotExist(err) {
		t.Fatalf("expected stale bak dir removed")
	}
	if _, err := os.Stat(keepBak); err != nil {
		t.Fatalf("expected canonical bak dir kept: %v", err)
	}
}
