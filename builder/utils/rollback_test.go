package utils

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestSyncVFS_RollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an existing file that will be overwritten
	existingFile := filepath.Join(tmpDir, "existing.html")
	if err := os.WriteFile(existingFile, []byte("original content"), 0644); err != nil {
		t.Fatalf("Failed to write existing file: %v", err)
	}

	// To make atomicWrite fail, we can create a file where a directory should be
	// but atomicWrite calls MkdirAll(filepath.Dir(path)).
	failDir := filepath.Join(tmpDir, "fail_dir")
	if err := os.MkdirAll(failDir, 0755); err != nil {
		t.Fatalf("Failed to create failDir: %v", err)
	}
	// Create a file named "blocked"
	blockedPath := filepath.Join(failDir, "blocked")
	if err := os.WriteFile(blockedPath, []byte("I am a file"), 0644); err != nil {
		t.Fatalf("Failed to create blocked file: %v", err)
	}
	// Now try to sync a file "blocked/something.html". MkdirAll should fail.

	srcFs := afero.NewMemMapFs()
	_ = afero.WriteFile(srcFs, "existing.html", []byte("new content"), 0644)
	_ = afero.WriteFile(srcFs, "fail_dir/blocked/something.html", []byte("should fail"), 0644)

	dirtyFiles := map[string]bool{
		"existing.html":                   true,
		"fail_dir/blocked/something.html": true,
	}

	ctx := context.Background()

	err := SyncVFS(ctx, srcFs, tmpDir, dirtyFiles, false)
	if err == nil {
		t.Fatal("SyncVFS should have failed due to MkdirAll error")
	}

	// Verify Rollback: existing.html should have its original content restored
	content, _ := os.ReadFile(existingFile)
	if string(content) != "original content" {
		t.Errorf("Rollback failed: expected 'original content', got '%s'", string(content))
	}
}
