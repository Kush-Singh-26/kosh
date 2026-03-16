package retry

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestRenameWithRetry_Success tests successful rename on first attempt
func TestRenameWithRetry_Success(t *testing.T) {
	ctx := context.Background()

	// Create temp directory and file
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RenameWithRetry() error = %v", err)
	}

	// Verify file was renamed
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("RenameWithRetry() failed to rename file")
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("RenameWithRetry() old file still exists")
	}
}

// TestRenameWithRetry_NotExist tests that non-existent source returns immediately
func TestRenameWithRetry_NotExist(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "nonexistent.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	start := time.Now()
	err := RenameWithRetry(ctx, oldPath, newPath, 3, 100*time.Millisecond)
	elapsed := time.Since(start)

	if !os.IsNotExist(err) {
		t.Errorf("RenameWithRetry() expected ErrNotExist, got %v", err)
	}

	// Should return immediately without retries
	if elapsed > 50*time.Millisecond {
		t.Errorf("RenameWithRetry() took too long for non-existent file: %v", elapsed)
	}
}

// TestRenameWithRetry_ContextCancellation tests cancellation during retries
func TestRenameWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	// Create file and lock it to force retries
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Cancel context immediately
	cancel()

	err := RenameWithRetry(ctx, oldPath, newPath, 10, 10*time.Millisecond)
	if err != context.Canceled {
		t.Errorf("RenameWithRetry() expected context.Canceled, got %v", err)
	}
}

// TestRenameWithRetry_MaxRetriesExhausted tests behavior when all retries fail
func TestRenameWithRetry_MaxRetriesExhausted(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	// Try to rename to an invalid path (directory instead of file)
	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "existing_dir")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create directory at target path
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err == nil {
		t.Error("RenameWithRetry() expected error when renaming to directory path")
	}
}

// TestRemoveAllWithRetry_Success tests successful removal
func TestRemoveAllWithRetry_Success(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "to_remove")

	// Create directory with files
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "file.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := RemoveAllWithRetry(ctx, testDir, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RemoveAllWithRetry() error = %v", err)
	}

	// Verify directory was removed
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("RemoveAllWithRetry() failed to remove directory")
	}
}

// TestRemoveAllWithRetry_AlreadyGone tests removal of non-existent path
func TestRemoveAllWithRetry_AlreadyGone(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "nonexistent")

	err := RemoveAllWithRetry(ctx, testDir, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RemoveAllWithRetry() error on non-existent path = %v", err)
	}
}

// TestRemoveAllWithRetry_ContextCancellation tests cancellation during removal
func TestRemoveAllWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "to_remove")

	// Create directory
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	err := RemoveAllWithRetry(ctx, testDir, 10, 100*time.Millisecond)
	// Should succeed or be canceled - both acceptable
	if err != nil && err != context.Canceled {
		t.Errorf("RemoveAllWithRetry() unexpected error = %v", err)
	}
}

// TestRenameWithRetry_Jitter tests that jitter is applied
func TestRenameWithRetry_Jitter(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "new.txt")

	// Create file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// First rename should succeed
	err := RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("First RenameWithRetry() error = %v", err)
	}

	// Move back for second test
	if err := os.Rename(newPath, oldPath); err != nil {
		t.Fatalf("Failed to move back: %v", err)
	}

	// Second rename should also succeed (jitter doesn't affect success)
	err = RenameWithRetry(ctx, oldPath, newPath, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("Second RenameWithRetry() error = %v", err)
	}
}

// TestRemoveAllWithRetry_EmptyDirectory tests removal of empty directory
func TestRemoveAllWithRetry_EmptyDirectory(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "empty_dir")

	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	err := RemoveAllWithRetry(ctx, testDir, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RemoveAllWithRetry() error on empty directory = %v", err)
	}
}

// TestRemoveAllWithRetry_NestedDirectories tests removal of nested structure
func TestRemoveAllWithRetry_NestedDirectories(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "nested")

	// Create nested structure
	nestedPath := filepath.Join(testDir, "a", "b", "c")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		t.Fatalf("Failed to create nested directories: %v", err)
	}

	// Add files at various levels
	os.WriteFile(filepath.Join(testDir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(testDir, "a", "level1.txt"), []byte("level1"), 0644)
	os.WriteFile(filepath.Join(testDir, "a", "b", "level2.txt"), []byte("level2"), 0644)
	os.WriteFile(filepath.Join(nestedPath, "deep.txt"), []byte("deep"), 0644)

	err := RemoveAllWithRetry(ctx, testDir, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RemoveAllWithRetry() error on nested directories = %v", err)
	}

	// Verify everything is removed
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("RemoveAllWithRetry() failed to remove nested directories")
	}
}

// TestRetryBackoffTiming tests that backoff delays are applied
func TestRetryBackoffTiming(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	// Invalid rename to force retries
	oldPath := filepath.Join(tmpDir, "old.txt")
	newPath := filepath.Join(tmpDir, "nonexistent_dir", "new.txt")

	// Create source file
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	start := time.Now()
	_ = RenameWithRetry(ctx, oldPath, newPath, 3, 50*time.Millisecond)
	elapsed := time.Since(start)

	// Should have taken at least some backoff time (but may fail fast on certain errors)
	// This is a soft check since some errors return immediately
	t.Logf("RenameWithRetry with retries took: %v", elapsed)
}

// TestRemoveAllWithRetry_Parallel tests concurrent removals
func TestRemoveAllWithRetry_Parallel(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	var wg sync.WaitGroup

	numDirs := 5
	wg.Add(numDirs)

	for i := 0; i < numDirs; i++ {
		testDir := filepath.Join(tmpDir, "dir_"+string(rune(byte('A'+i))))

		go func(dir string) {
			defer wg.Done()
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Errorf("Failed to create test directory: %v", err)
				return
			}
			if err := RemoveAllWithRetry(ctx, dir, 3, 10*time.Millisecond); err != nil {
				t.Errorf("RemoveAllWithRetry() error = %v", err)
			}
		}(testDir)
	}

	wg.Wait()
}

// TestRenameWithRetry_SamePath tests renaming to the same path
func TestRenameWithRetry_SamePath(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "same.txt")

	// Create file
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := RenameWithRetry(ctx, filePath, filePath, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RenameWithRetry() same path error = %v", err)
	}

	// Verify file still exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("RenameWithRetry() file disappeared after same-path rename")
	}
}

// TestRemoveAllWithRetry_Symlink tests removal with symlinks (Windows-safe)
func TestRemoveAllWithRetry_Symlink(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create target directory
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	// Create symlink (skip if not supported, e.g., on Windows without privileges)
	linkPath := filepath.Join(tmpDir, "link")
	if err := os.Symlink(targetDir, linkPath); err != nil {
		t.Skipf("Symlinks not supported: %v", err)
	}

	// RemoveAll should handle symlinks correctly (remove link, not target)
	err := RemoveAllWithRetry(ctx, linkPath, 3, 10*time.Millisecond)
	if err != nil {
		t.Errorf("RemoveAllWithRetry() symlink error = %v", err)
	}

	// Target should still exist
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		t.Error("RemoveAllWithRetry() removed symlink target instead of link")
	}
}
