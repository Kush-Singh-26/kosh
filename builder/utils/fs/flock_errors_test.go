package fs

import (

	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAcquireBuildLock_MkdirError tests that AcquireBuildLock returns error when creating output directory fails
func TestAcquireBuildLock_MkdirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows - requires different approach")
	}

	// Create a temp directory, then make its parent read-only
	tmpBase := t.TempDir()
	parentDir := filepath.Join(tmpBase, "parent")
	if err := os.Mkdir(parentDir, 0755); err != nil {
		t.Fatalf("Failed to create parent dir: %v", err)
	}
	// Make parent directory read-only
	if err := os.Chmod(parentDir, 0555); err != nil {
		t.Skipf("Cannot set directory permissions: %v", err)
	}
	defer func() { _ = os.Chmod(parentDir, 0755) }() // Cleanup

	outputDir := filepath.Join(parentDir, "output")
	_, err := AcquireBuildLock(outputDir)
	if err == nil {
		t.Error("AcquireBuildLock should fail when cannot create output directory")
	}
	if !strings.Contains(err.Error(), "failed to create lock directory") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestAcquireBuildLock_OpenFileError tests that AcquireBuildLock returns error when creating lock file fails
func TestAcquireBuildLock_OpenFileError(t *testing.T) {
	outputDir := t.TempDir()

	// Create lock file manually with no write permissions
	lockPath := filepath.Clean(outputDir) + ".lock"
	// Create the file first (as normal)
	if err := os.WriteFile(lockPath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}
	// Make it read-only
	if err := os.Chmod(lockPath, 0444); err != nil {
		t.Skipf("Cannot set file permissions: %v", err)
	}
	defer func() { _ = os.Chmod(lockPath, 0644) }() // Cleanup

	// Attempt to acquire lock should fail because OpenFile needs write access
	_, err := AcquireBuildLock(outputDir)
	// On Unix, OpenFile with O_RDWR will fail on read-only file
	// On Windows, behavior may differ
	if err == nil {
		t.Error("AcquireBuildLock should fail when lock file is read-only")
	}
}

// TestAcquireBuildLock_StaleLockFile tests that AcquireBuildLock succeeds when lock file exists but is not locked
func TestAcquireBuildLock_StaleLockFile(t *testing.T) {
	outputDir := t.TempDir()

	// Create a lock file manually without holding the lock
	lockPath := filepath.Clean(outputDir) + ".lock"
	if err := os.WriteFile(lockPath, []byte("stale lock\n"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Should be able to acquire lock because file is not actually locked by any process
	lock, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("AcquireBuildLock failed with stale lock file: %v", err)
	}
	defer func() { _ = lock.Release() }()

	// Read the lock file content directly from the locked file handle to avoid Windows sharing issues
	_, _ = lock.file.Seek(0, 0)
	content := make([]byte, 100)
	n, _ := lock.file.Read(content)
	content = content[:n]

	// The lock file should now contain new PID
	if !strings.Contains(string(content), fmt.Sprintf("%d", os.Getpid())) {
		t.Error("Lock file should be overwritten with new PID")
	}
}

// TestAcquireBuildLock_WritePIDError tests that AcquireBuildLock still returns lock even if PID write fails
func TestAcquireBuildLock_WritePIDError(t *testing.T) {
	// Skip this test as it requires mocking or tricky setup
	t.Skip("Requires mocking tryLock or file methods to simulate write error")
}

// TestFileLock_Release_Nil tests that Release on a nil FileLock is safe
func TestFileLock_Release_Nil(t *testing.T) {
	var fl *FileLock
	// Should not panic
	err := fl.Release()
	if err != nil {
		t.Errorf("Release on nil FileLock should return nil, got: %v", err)
	}
}

// TestFileLock_Release_FileCloseError tests that Release returns error if file close fails
func TestFileLock_Release_FileCloseError(t *testing.T) {
	t.Skip("Testing file close error requires mocking or special setup")
}

// TestFileLock_Release_UnlockError tests that Release continues even if unlock fails
func TestFileLock_Release_UnlockError(t *testing.T) {
	// Skip this test due to difficulty simulating unlock failure
	t.Skip("Testing unlock error requires mocking platform-specific unlock function")
}

// TestAcquireBuildLock_LockContention tests that second builder wait? Actually build lock is non-blocking, so it should fail fast
func TestAcquireBuildLock_LockContention(t *testing.T) {
	outputDir := t.TempDir()

	// First lock acquisition should succeed
	lock1, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("First AcquireBuildLock failed: %v", err)
	}
	defer func() { _ = lock1.Release() }()

	// Second acquisition should fail with "another build is in progress"
	_, err = AcquireBuildLock(outputDir)
	if err == nil {
		t.Error("Second AcquireBuildLock should fail with lock held")
	}
	if !strings.Contains(err.Error(), "another build is in progress") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestAcquireBuildLock_WindowsCompatibility ensures lock works on Windows
func TestAcquireBuildLock_WindowsCompatibility(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Test is only relevant for Windows")
	}
	outputDir := t.TempDir()

	lock, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("AcquireBuildLock failed on Windows: %v", err)
	}
	defer func() { _ = lock.Release() }()

	if lock.file == nil {
		t.Error("Lock should have file handle on Windows")
	}
}
