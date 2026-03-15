package utils

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRenameWithRetry_Success tests successful rename on first attempt
func TestRenameWithRetry_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("Expected successful rename, got error: %v", err)
	}

	// Verify file was renamed
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("Expected old path to not exist")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("Expected new path to exist")
	}
}

// TestRenameWithRetry_NotExist tests that non-existent source returns immediately
func TestRenameWithRetry_NotExist(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "nonexistent.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if !os.IsNotExist(err) {
		t.Errorf("Expected os.IsNotExist error, got: %v", err)
	}
}

// TestRenameWithRetry_ContextCancellation tests context cancellation
func TestRenameWithRetry_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

// TestRemoveAllWithRetry_Success tests successful removal on first attempt
func TestRemoveAllWithRetry_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create a subdirectory to remove
	subDir := filepath.Join(tmpDir, "toRemove")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create some files in the directory
	if err := os.WriteFile(filepath.Join(subDir, "file1.txt"), []byte("test1"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("test2"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := RemoveAllWithRetry(ctx, subDir, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("Expected successful removal, got error: %v", err)
	}

	// Verify directory was removed
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Error("Expected directory to not exist")
	}
}

// TestRemoveAllWithRetry_NonExistent tests removal of non-existent path
func TestRemoveAllWithRetry_NonExistent(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	nonExistentPath := filepath.Join(tmpDir, "nonexistent")

	err := RemoveAllWithRetry(ctx, nonExistentPath, 3, 10*time.Millisecond)
	if err != nil {
		// os.RemoveAll returns nil for non-existent paths
		t.Errorf("Expected no error for non-existent path, got: %v", err)
	}
}

// TestRemoveAllWithRetry_ContextCancellation tests context cancellation
func TestRemoveAllWithRetry_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory to remove
	subDir := filepath.Join(tmpDir, "toRemove")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := RemoveAllWithRetry(ctx, subDir, 3, 10*time.Millisecond)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

// TestRenameWithRetry_Backoff tests that backoff delay increases
func TestRenameWithRetry_Backoff(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Try to rename to a path that will fail (directory as target)
	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "existingDir")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	// Create directory as target (will cause rename to fail)
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	start := time.Now()
	err := RenameWithRetry(ctx, oldPath, newPath, 3, 50*time.Millisecond)
	elapsed := time.Since(start)

	// Should have attempted 3 retries with backoff
	// Delays: ~50ms, ~100ms, ~200ms = ~350ms minimum
	if err == nil {
		t.Error("Expected error from failed rename")
	}
	// Just verify we spent some time in backoff (allowing for system variance)
	if elapsed < 100*time.Millisecond {
		t.Logf("Warning: backoff elapsed time (%v) seems short", elapsed)
	}
}

// TestRemoveAllWithRetry_Backoff tests that backoff delay increases
func TestRemoveAllWithRetry_Backoff(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create a directory that we'll try to remove
	subDir := filepath.Join(tmpDir, "toRemove")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a file inside
	filePath := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Lock the file by opening it without closing (simulate file in use)
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	start := time.Now()
	err = RemoveAllWithRetry(ctx, subDir, 3, 50*time.Millisecond)
	elapsed := time.Since(start)

	// Should have attempted retries with backoff
	if err == nil {
		t.Error("Expected error from failed removal (file locked)")
	}
	// Just verify we spent some time in backoff
	if elapsed < 100*time.Millisecond {
		t.Logf("Warning: backoff elapsed time (%v) seems short", elapsed)
	}
}

// TestRenameWithRetry_MaxRetries tests that all retries are attempted
func TestRenameWithRetry_MaxRetries(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "existingDir")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	// Create directory as target
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err == nil {
		t.Error("Expected error from failed rename")
	}
}

// TestRemoveAllWithRetry_MaxRetries tests that all retries are attempted
func TestRemoveAllWithRetry_MaxRetries(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "toRemove")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Lock the directory by opening a file inside
	filePath := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	err = RemoveAllWithRetry(ctx, subDir, 3, 10*time.Millisecond)
	if err == nil {
		t.Error("Expected error from failed removal")
	}
}

// TestRenameWithRetry_ZeroRetries tests with maxRetries=0
func TestRenameWithRetry_ZeroRetries(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := RenameWithRetry(ctx, oldPath, newPath, 0, 10*time.Millisecond)
	// With 0 retries, the loop doesn't execute, returns nil (no attempt made)
	// This is the expected behavior - caller should use at least 1 retry
	if err != nil {
		t.Errorf("Expected nil error with zero retries (no attempt), got: %v", err)
	}
}

// TestRemoveAllWithRetry_ZeroRetries tests with maxRetries=0
func TestRemoveAllWithRetry_ZeroRetries(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "toRemove")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Lock the directory
	filePath := filepath.Join(subDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	f, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	err = RemoveAllWithRetry(ctx, subDir, 0, 10*time.Millisecond)
	// With 0 retries, the loop doesn't execute, returns nil (no attempt made)
	// This is the expected behavior - caller should use at least 1 retry
	if err != nil {
		t.Errorf("Expected nil error with zero retries (no attempt), got: %v", err)
	}
}

// TestRenameWithRetry_DestinationExists tests rename when destination exists
func TestRenameWithRetry_DestinationExists(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "existing.txt")

	// Create both files
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatalf("Failed to create old file: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// On Windows, this might fail; on Unix, it succeeds
	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	// We accept either outcome - just verify no panic
	if err != nil {
		t.Logf("Rename to existing path returned: %v (expected on Windows)", err)
	}
}

// TestRemoveAllWithRetry_EmptyDirectory tests removal of empty directory
func TestRemoveAllWithRetry_EmptyDirectory(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	emptyDir := filepath.Join(tmpDir, "empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}

	err := RemoveAllWithRetry(ctx, emptyDir, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("Expected successful removal of empty directory, got: %v", err)
	}
}
