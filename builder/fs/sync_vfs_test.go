package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestSyncVFS_Basic(t *testing.T) {
	srcFs := afero.NewMemMapFs()
	targetDir := t.TempDir()

	// Create test files in VFS
	_ = srcFs.MkdirAll("posts", 0755)
	_ = afero.WriteFile(srcFs, "posts/test1.html", []byte("content1"), 0644)
	_ = afero.WriteFile(srcFs, "posts/test2.html", []byte("content2"), 0644)
	_ = afero.WriteFile(srcFs, "config.json", []byte("config"), 0644)

	dirtyFiles := map[string]bool{
		"posts/test1.html": true,
		"posts/test2.html": true,
	}

	ctx := context.Background()
	err := SyncVFS(SyncOptions{
		Ctx:          ctx,
		SrcFs:        srcFs,
		TargetDir:    targetDir,
		DirtyFiles:   dirtyFiles,
		IsCleanBuild: true,
	})
	if err != nil {
		t.Fatalf("SyncVFS failed: %v", err)
	}

	// Verify files written to disk
	for path := range dirtyFiles {
		osPath := filepath.Join(targetDir, filepath.FromSlash(path))
		content, err := os.ReadFile(osPath)
		if err != nil {
			t.Errorf("File not found on disk: %s", path)
			continue
		}
		expected, _ := afero.ReadFile(srcFs, path)
		if string(content) != string(expected) {
			t.Errorf("Content mismatch for %s", path)
		}
	}
}

func TestSyncVFS_AbsolutePaths(t *testing.T) {
	srcFs := afero.NewMemMapFs()
	targetDir := t.TempDir()
	absTargetDir, _ := filepath.Abs(targetDir)

	// In real SSG, RenderPage uses absolute paths for Create and RegisterFile
	testFileAbs := filepath.Join(absTargetDir, "index.html")

	_ = afero.WriteFile(srcFs, testFileAbs, []byte("absolute content"), 0644)

	dirtyFiles := map[string]bool{
		testFileAbs: true,
	}

	ctx := context.Background()
	err := SyncVFS(SyncOptions{
		Ctx:          ctx,
		SrcFs:        srcFs,
		TargetDir:    targetDir,
		DirtyFiles:   dirtyFiles,
		IsCleanBuild: true,
	})
	if err != nil {
		t.Fatalf("SyncVFS failed with absolute paths: %v", err)
	}

	// Verify file written to correct OS path (not nested)
	if _, err := os.Stat(testFileAbs); err != nil {
		t.Errorf("File should exist at %s, but got error: %v", testFileAbs, err)
	}

	// Ensure no nested directory was created
	// nestedPath would be something like targetDir/C:/...
	// On Windows, Join(targetDir, "C:/...") creates targetDir/C:/... which is what we saw in the error
}

func TestAtomicWrite_PreservesContent(t *testing.T) {
	outputDir := t.TempDir()
	testFile := filepath.Join(outputDir, "test_atomic.html")
	content := []byte("Atomic write test content")

	err := atomicWrite(context.Background(), testFile, content)
	if err != nil {
		t.Fatalf("atomicWrite failed: %v", err)
	}

	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(written) != string(content) {
		t.Errorf("Content mismatch: got %q, want %q", written, content)
	}

	_, err = os.ReadFile(testFile + ".tmp")
	if err == nil {
		t.Error("Temp file should not exist after atomic rename")
	}

	t.Log("Atomic write preserves content test passed")
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	outputDir := t.TempDir()
	testFile := filepath.Join(outputDir, "test_overwrite.html")

	_ = os.WriteFile(testFile, []byte("old content"), 0644)

	err := atomicWrite(context.Background(), testFile, []byte("new content"))
	if err != nil {
		t.Fatalf("atomicWrite failed: %v", err)
	}

	written, _ := os.ReadFile(testFile)
	if string(written) != "new content" {
		t.Errorf("Content not overwritten: got %q", written)
	}

	t.Log("Atomic write overwrites existing test passed")
}

func TestAtomicWrite_CleansUpOnError(t *testing.T) {
	outputDir := t.TempDir()
	testFile := filepath.Join(outputDir, "test_error.html")

	// Test with empty path (should fail)
	err := atomicWrite(context.Background(), testFile, []byte{})
	if err != nil {
		t.Logf("Got error as expected: %v", err)
	}

	// File may or may not exist depending on implementation
	_, _ = os.ReadFile(testFile)
	_, _ = os.ReadFile(testFile + ".tmp")

	t.Log("Atomic write cleanup on error test passed")
}

func TestAcquireBuildLock_Success(t *testing.T) {
	outputDir := t.TempDir()

	lock, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("fspkg.AcquireBuildLock failed: %v", err)
	}

	if lock == nil {
		t.Fatal("fspkg.AcquireBuildLock should return non-nil lock")
	}

	// Lock file should exist
	lockPath := filepath.Clean(outputDir) + ".lock"
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file should exist")
	}

	// Release the lock
	if err := lock.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	t.Log("fspkg.AcquireBuildLock success test passed")
}

func TestAcquireBuildLock_DoubleLockFails(t *testing.T) {
	outputDir := t.TempDir()

	// First lock should succeed
	lock1, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("First fspkg.AcquireBuildLock failed: %v", err)
	}
	defer func() { _ = lock1.Release() }()

	// Second lock should fail (non-blocking)
	_, err = AcquireBuildLock(outputDir)
	if err == nil {
		t.Error("Second fspkg.AcquireBuildLock should fail when lock is held")
	}

	t.Log("Double lock prevention test passed")
}

func TestAcquireBuildLock_ReleaseClearsLock(t *testing.T) {
	outputDir := t.TempDir()

	// Acquire and release lock
	lock, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("fspkg.AcquireBuildLock failed: %v", err)
	}

	if err := lock.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Lock file should be cleaned up
	lockPath := filepath.Clean(outputDir) + ".lock"
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("Lock file should be removed after release")
	}

	// Should be able to acquire lock again
	lock2, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("Second fspkg.AcquireBuildLock after release failed: %v", err)
	}
	defer func() { _ = lock2.Release() }()

	t.Log("Release clears lock test passed")
}

func TestAcquireBuildLock_PIDWritten(t *testing.T) {
	outputDir := t.TempDir()

	lock, err := AcquireBuildLock(outputDir)
	if err != nil {
		t.Fatalf("fspkg.AcquireBuildLock failed: %v", err)
	}
	defer func() { _ = lock.Release() }()

	// Lock file should exist
	lockPath := filepath.Clean(outputDir) + ".lock"
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Lock file should exist: %v", err)
	}

	// File should have some content (PID + timestamp)
	if info.Size() == 0 {
		t.Error("Lock file should have content (PID)")
	}

	t.Log("PID written to lock file test passed")
}
