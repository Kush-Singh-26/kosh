package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
)

const (
	testDirMode           = 0755
	testFileMode          = 0644
	conditionPollInterval = 5 * time.Millisecond
)

// CreateTestCache creates a temporary cache for testing
// Returns the cache manager and a cleanup function
func CreateTestCache(t *testing.T) (*cache.Manager, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	m, err := cache.Open(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to open cache: %v", err)
	}
	return m, func() {
		_ = m.Close()
	}
}

// CreateTestCacheWithID creates a cache with a specific ID
func CreateTestCacheWithID(t *testing.T, cacheID string) (*cache.Manager, func()) {
	t.Helper()
	m, cleanup := CreateTestCache(t)
	if err := m.SetCacheID(cacheID); err != nil {
		t.Fatalf("Failed to set cache ID: %v", err)
	}
	return m, cleanup
}

// CreateTestFilesystem creates source and destination filesystems for testing
func CreateTestFilesystem() (afero.Fs, afero.Fs) {
	return afero.NewMemMapFs(), afero.NewMemMapFs()
}

// CreateTestFilesystemWithContent creates filesystems with initial content
func CreateTestFilesystemWithContent(files map[string]string) (afero.Fs, afero.Fs) {
	sourceFs, destFs := CreateTestFilesystem()
	for path, content := range files {
		dir := filepath.Dir(path)
		if err := sourceFs.MkdirAll(dir, testDirMode); err != nil {
			panic(err)
		}
		if err := afero.WriteFile(sourceFs, path, []byte(content), testFileMode); err != nil {
			panic(err)
		}
	}
	return sourceFs, destFs
}

// AssertFileExists checks if a file exists in the filesystem
func AssertFileExists(t *testing.T, fs afero.Fs, path string) {
	t.Helper()
	exists, err := afero.Exists(fs, path)
	if err != nil {
		t.Fatalf("Error checking file existence: %v", err)
	}
	if !exists {
		t.Errorf("Expected file to exist: %s", path)
	}
}

// AssertFileNotExists checks if a file does not exist
func AssertFileNotExists(t *testing.T, fs afero.Fs, path string) {
	t.Helper()
	exists, err := afero.Exists(fs, path)
	if err != nil {
		t.Fatalf("Error checking file existence: %v", err)
	}
	if exists {
		t.Errorf("Expected file to not exist: %s", path)
	}
}

// AssertFileContent checks if a file has the expected content
func AssertFileContent(t *testing.T, fs afero.Fs, path string, expected []byte) {
	t.Helper()
	content, err := afero.ReadFile(fs, path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	if string(content) != string(expected) {
		t.Errorf("File %s content mismatch:\nexpected: %s\ngot: %s", path, expected, content)
	}
}

// CreateTempDir creates a temporary directory for testing
func CreateTempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// CreateTempFile creates a temporary file with content
func CreateTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	//nolint:errcheck // Cleanup in test helper
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	return tmpFile.Name()
}

// ScaffoldTestSite creates a minimal kosh site in the given filesystem
func ScaffoldTestSite(fs afero.Fs) {
	// 1. Create kosh.yaml
	koshYaml := `
title: "Test Blog"
baseURL: "https://example.com"
theme: "test-theme"
themeDir: "themes"
contentDir: "content"
outputDir: "public"
cacheDir: ".kosh-cache"
`
	_ = afero.WriteFile(fs, "kosh.yaml", []byte(koshYaml), testFileMode)

	// 2. Create content
	_ = fs.MkdirAll("content/posts", testDirMode)
	postContent := `---
title: "Latest Post"
date: 2026-03-06
tags: ["test"]
---
# Latest Post
This is a test post.
`
	_ = afero.WriteFile(fs, "content/posts/hello.md", []byte(postContent), testFileMode)

	// Create 404 page
	_ = afero.WriteFile(fs, "content/404.md", []byte("---\ntitle: \"404\"\n---\nPage not found."), testFileMode)

	// 3. Create theme
	themeDir := "themes/test-theme/templates"
	_ = fs.MkdirAll(themeDir, testDirMode)

	layoutTmpl := `{{ define "content" }}{{ .Content }}{{ end }}`
	indexTmpl := `{{ define "content" }}{{ range .Posts }}<h2>{{ .Title }}</h2>{{ end }}{{ .Content }}{{ end }}`

	_ = afero.WriteFile(fs, filepath.Join(themeDir, "layout.html"), []byte(layoutTmpl), testFileMode)
	_ = afero.WriteFile(fs, filepath.Join(themeDir, "index.html"), []byte(indexTmpl), testFileMode)
	_ = afero.WriteFile(fs, filepath.Join(themeDir, "404.html"), []byte(layoutTmpl), testFileMode)
}

// WaitForCondition polls a condition until it's met or a timeout occurs
func WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	start := time.Now()
	for time.Since(start) < timeout {
		if condition() {
			return
		}
		time.Sleep(conditionPollInterval)
	}
	t.Fatalf("timed out waiting for condition after %v", timeout)
}
