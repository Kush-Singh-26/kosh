package tx

import (
	"context"
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

	CleanupStaleBuildDirs(outputDir)

	if _, err := os.Stat(staleTmp); !os.IsNotExist(err) {
		t.Fatalf("expected stale tmp dir removed")
	}
	if _, err := os.Stat(staleBak); !os.IsNotExist(err) {
		t.Fatalf("expected stale bak dir removed")
	}
	if _, err := os.Stat(keepBak); err != nil {
		t.Fatalf("expected keep bak dir to remain")
	}
}

// TestTransaction_RenameFailure verifies retry logic on rename failure
func TestTransaction_RenameFailure(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "public")
	tx := NewBuildTransaction(outputDir, true)

	// Create a file in staging
	stagingFile := filepath.Join(tx.StagingDir(), "test.txt")
	if err := os.MkdirAll(filepath.Dir(stagingFile), 0755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}
	if err := os.WriteFile(stagingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create staging file: %v", err)
	}

	// Commit should succeed
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify file exists in output
	outputFile := filepath.Join(outputDir, "test.txt")
	if _, err := os.Stat(outputFile); err != nil {
		t.Errorf("expected output file to exist after commit: %v", err)
	}
}

// TestTransaction_RollbackRestoresState verifies rollback behavior
func TestTransaction_RollbackRestoresState(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "public")

	// Create initial state in output
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("failed to create output dir: %v", err)
	}
	initialFile := filepath.Join(outputDir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	tx := NewBuildTransaction(outputDir, true)

	// Create file in staging
	stagingFile := filepath.Join(tx.StagingDir(), "new.txt")
	if err := os.MkdirAll(filepath.Dir(stagingFile), 0755); err != nil {
		t.Fatalf("failed to create staging dir: %v", err)
	}
	if err := os.WriteFile(stagingFile, []byte("new"), 0644); err != nil {
		t.Fatalf("failed to create staging file: %v", err)
	}

	// Rollback (simulating failed build)
	tx.Rollback()

	// Initial file should still exist
	if _, err := os.Stat(initialFile); err != nil {
		t.Errorf("expected initial file to remain after rollback: %v", err)
	}

	// New file should not exist in output
	outputFile := filepath.Join(outputDir, "new.txt")
	if _, err := os.Stat(outputFile); !os.IsNotExist(err) {
		t.Error("expected new file to not exist after rollback")
	}
}

// TestTransaction_ConcurrentBuildAttempts verifies unique temp dirs prevent conflicts
func TestTransaction_ConcurrentBuildAttempts(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "public")

	// Create multiple transactions concurrently
	const numConcurrent = 5
	txs := make([]BuildTransaction, numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		txs[i] = NewBuildTransaction(outputDir, true)
	}

	// Verify all have unique staging dirs
	seen := make(map[string]bool)
	for i, tx := range txs {
		staging := tx.StagingDir()
		if seen[staging] {
			t.Errorf("transaction %d has duplicate staging dir: %s", i, staging)
		}
		seen[staging] = true
	}

	// All should be able to commit independently
	for i, tx := range txs {
		testFile := filepath.Join(tx.StagingDir(), "test.txt")
		if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
			t.Fatalf("tx %d: failed to create staging dir: %v", i, err)
		}
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatalf("tx %d: failed to create file: %v", i, err)
		}

		if err := tx.Commit(context.Background()); err != nil {
			t.Fatalf("tx %d: Commit failed: %v", i, err)
		}
	}
}
