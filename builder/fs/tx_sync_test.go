package fs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestTxSyncCommit(t *testing.T) {
	dir, err := os.MkdirTemp("", "kosh-txsync-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	tx := NewTxSync(slog.Default())

	// Create a dummy existing file
	existingPath := filepath.Join(dir, "existing.txt")
	_ = os.WriteFile(existingPath, []byte("old"), 0644)

	// Track write (backs up automatically)
	_ = tx.TrackWrite(existingPath)
	_ = os.WriteFile(existingPath, []byte("new"), 0644)

	// Track a brand new file
	newPath := filepath.Join(dir, "new.txt")
	_ = os.WriteFile(newPath, []byte("brand_new"), 0644)
	_ = tx.TrackWrite(newPath)

	// Commit should remove backups and keep files
	tx.Commit()

	if _, err := os.Stat(existingPath + ".kosh-rollback"); !os.IsNotExist(err) {
		t.Error("Backup file should be deleted on Commit")
	}

	content, _ := os.ReadFile(existingPath)
	if string(content) != "new" {
		t.Errorf("Expected 'new', got '%s'", string(content))
	}
}

func TestTxSyncRollback(t *testing.T) {
	dir, err := os.MkdirTemp("", "kosh-txsync-test-rollback-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	tx := NewTxSync(slog.Default())

	// Create a dummy existing file
	existingPath := filepath.Join(dir, "existing.txt")
	_ = os.WriteFile(existingPath, []byte("old"), 0644)

	// Track write (backs up automatically)
	_ = tx.TrackWrite(existingPath)
	_ = os.WriteFile(existingPath, []byte("new"), 0644)

	// Track a brand new file
	newPath := filepath.Join(dir, "new.txt")
	_ = tx.TrackWrite(newPath)
	_ = os.WriteFile(newPath, []byte("brand_new"), 0644)

	// Rollback should restore backups and delete brand new files
	tx.Rollback(context.Background())

	content, _ := os.ReadFile(existingPath)
	if string(content) != "old" {
		t.Errorf("Rollback failed, expected 'old', got '%s'", string(content))
	}

	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Error("Brand new file should be deleted on Rollback")
	}
}
