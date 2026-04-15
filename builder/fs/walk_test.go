package fs

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
)

func TestParallelWalk(t *testing.T) {
	// Create a temporary directory structure
	root, err := os.MkdirTemp("", "kosh-walk-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(root) }()

	dirs := []string{
		"a",
		"b",
		"a/c",
		"a/d",
		"b/e",
	}
	files := []string{
		"f1.txt",
		"a/f2.txt",
		"a/c/f3.txt",
		"b/f4.txt",
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(root, f), []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	// Test ParallelWalk on OsFs
	sourceFs := afero.NewOsFs()
	ctx := context.Background()
	var fileCount int32
	var dirCount int32

	err = ParallelWalk(WalkOptions{
		Ctx:         ctx,
		SourceFs:    sourceFs,
		Root:        root,
		Concurrency: 4,
		WalkFn: func(_ string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				atomic.AddInt32(&dirCount, 1)
			} else {
				atomic.AddInt32(&fileCount, 1)
			}
			return nil
		},
	})

	if err != nil {
		t.Errorf("ParallelWalk failed: %v", err)
	}

	// +1 for the root itself
	expectedDirs := int32(len(dirs) + 1)
	expectedFiles := int32(len(files))

	if dirCount != expectedDirs {
		t.Errorf("expected %d dirs, got %d", expectedDirs, dirCount)
	}
	if fileCount != expectedFiles {
		t.Errorf("expected %d files, got %d", expectedFiles, fileCount)
	}
}

func TestParallelWalk_MemMapFsFallback(t *testing.T) {
	sourceFs := afero.NewMemMapFs()
	root := "/test"
	_ = sourceFs.MkdirAll("/test/a/b", 0755)
	_ = afero.WriteFile(sourceFs, "/test/f1.txt", []byte("test"), 0644)
	_ = afero.WriteFile(sourceFs, "/test/a/f2.txt", []byte("test"), 0644)

	ctx := context.Background()
	var fileCount int32
	var dirCount int32

	err := ParallelWalk(WalkOptions{
		Ctx:         ctx,
		SourceFs:    sourceFs,
		Root:        root,
		Concurrency: 4,
		WalkFn: func(_ string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				atomic.AddInt32(&dirCount, 1)
			} else {
				atomic.AddInt32(&fileCount, 1)
			}
			return nil
		},
	})

	if err != nil {
		t.Errorf("ParallelWalk fallback failed: %v", err)
	}

	// /test, /test/a, /test/a/b
	expectedDirs := int32(3)
	// f1.txt, f2.txt
	expectedFiles := int32(2)

	if dirCount != expectedDirs {
		t.Errorf("expected %d dirs, got %d", expectedDirs, dirCount)
	}
	if fileCount != expectedFiles {
		t.Errorf("expected %d files, got %d", expectedFiles, fileCount)
	}
}

func TestParallelWalk_SkipDir(t *testing.T) {
	root, _ := os.MkdirTemp("", "kosh-walk-skip")
	defer func() { _ = os.RemoveAll(root) }()

	_ = os.MkdirAll(filepath.Join(root, "skip_me/inner"), 0755)
	_ = os.WriteFile(filepath.Join(root, "skip_me/f1.txt"), []byte("test"), 0644)
	_ = os.WriteFile(filepath.Join(root, "keep_me.txt"), []byte("test"), 0644)

	ctx := context.Background()
	var seenSkipInner bool

	err := ParallelWalk(WalkOptions{
		Ctx:         ctx,
		SourceFs:    afero.NewOsFs(),
		Root:        root,
		Concurrency: 2,
		WalkFn: func(path string, _ fs.FileInfo, _ error) error {
			if filepath.Base(path) == "skip_me" {
				return filepath.SkipDir
			}
			if filepath.Base(path) == "inner" || filepath.Base(path) == "f1.txt" {
				seenSkipInner = true
			}
			return nil
		},
	})

	if err != nil {
		t.Errorf("ParallelWalk with SkipDir failed: %v", err)
	}
	if seenSkipInner {
		t.Error("should not have visited contents of skip_me")
	}
}
