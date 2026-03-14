package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiskSink_CopyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kosh-test-copy-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	stagingDir := filepath.Join(tempDir, "staging")
	outputDir := filepath.Join(tempDir, "output")
	_ = os.MkdirAll(stagingDir, 0755)
	_ = os.MkdirAll(outputDir, 0755)

	sink := NewDiskSink(stagingDir, outputDir)

	// Create a source file
	srcPath := filepath.Join(tempDir, "source.txt")
	content := []byte("hello kernel copy")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Test CopyFile
	destPath := "static/dest.txt"
	if err := sink.CopyFile(srcPath, destPath); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	// Verify file exists in staging
	stagingFile := filepath.Join(stagingDir, "static", "dest.txt")
	got, err := os.ReadFile(stagingFile)
	if err != nil {
		t.Fatalf("Failed to read destination file from staging: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("Expected content %q, got %q", content, got)
	}

	// Verify it's registered
	written := sink.GetWrittenFiles()
	realDest := filepath.Join(outputDir, "static", "dest.txt")
	if !written[realDest] {
		t.Errorf("Destination path %q not registered as written", realDest)
	}
}
