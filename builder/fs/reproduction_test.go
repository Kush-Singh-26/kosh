package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskSink_NestingBug(t *testing.T) {
	// 1. Setup staging and output dirs
	// Use absolute paths to mirror real behavior
	cwd, _ := os.Getwd()
	staging := filepath.Join(cwd, "public.tmp")
	realOut := filepath.Join(cwd, "public")

	// Ensure they don't exist
	_ = os.RemoveAll(staging)
	_ = os.RemoveAll(realOut)
	defer func() { _ = os.RemoveAll(staging) }()
	defer func() { _ = os.RemoveAll(realOut) }()

	sink := NewDiskSink(staging, realOut)

	// 2. Call WriteFile with path containing OutputDir prefix
	// This mirrors how orchestration calls it: filepath.Join(b.Cfg.OutputDir, "404.html")
	path := filepath.Join("public", "404.html")
	data := []byte("404")

	if err := sink.WriteFile(path, data); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 3. Check where it was written in staging
	// It SHOULD be in staging/404.html
	expectedStagedPath := filepath.Join(staging, "404.html")
	if _, err := os.Stat(expectedStagedPath); os.IsNotExist(err) {
		// If it's not there, let's see where it actually is
		actualStagedPath := filepath.Join(staging, "public", "404.html")
		if _, err := os.Stat(actualStagedPath); err == nil {
			t.Errorf("Nesting bug confirmed: file was written to %s instead of %s", actualStagedPath, expectedStagedPath)
		} else {
			t.Errorf("File was not found in staging at all!")
		}
	}
}

func TestDiskSink_RegisterNestingBug(t *testing.T) {
	cwd, _ := os.Getwd()
	staging := filepath.Join(cwd, "public.tmp")
	realOut := filepath.Join(cwd, "public")

	sink := NewDiskSink(staging, realOut)

	// Path with OutputDir prefix
	path := filepath.Join("public", "404.html")
	sink.Register(path)

	writtenFiles := sink.GetWrittenFiles()
	expectedFinalPath := filepath.Join(realOut, "404.html")

	found := false
	for f := range writtenFiles {
		if f == expectedFinalPath {
			found = true
			break
		}
	}

	if !found {
		// Check if it's nested
		nestedPath := filepath.Join(realOut, "public", "404.html")
		for f := range writtenFiles {
			if f == nestedPath {
				t.Errorf("Register nesting bug confirmed: file was registered as %s instead of %s", f, expectedFinalPath)
			}
		}
	}
}
