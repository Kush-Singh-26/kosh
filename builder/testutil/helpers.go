package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
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
		if err := sourceFs.MkdirAll(dir, 0755); err != nil {
			panic(err)
		}
		if err := afero.WriteFile(sourceFs, path, []byte(content), 0644); err != nil {
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
	ScaffoldTestSiteWithVersions(fs, false)
}

// ScaffoldTestSiteWithVersions creates a kosh site, optionally with multiple versions
func ScaffoldTestSiteWithVersions(fs afero.Fs, multiVersion bool) {
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
	if multiVersion {
		koshYaml += `
versions:
  - name: "v2.0"
    path: ""
    isLatest: true
  - name: "v1.0"
    path: "v1.0"
    isLatest: false
`
	}
	_ = afero.WriteFile(fs, "kosh.yaml", []byte(koshYaml), 0644)

	// 2. Create content
	_ = fs.MkdirAll("content/posts", 0755)
	postContent := `---
title: "Latest Post"
date: 2026-03-06
tags: ["test"]
---
# Latest Post
This is the latest version.
`
	_ = afero.WriteFile(fs, "content/posts/hello.md", []byte(postContent), 0644)

	// Create 404 page
	_ = afero.WriteFile(fs, "content/404.md", []byte("---\ntitle: \"404\"\n---\nPage not found."), 0644)

	if multiVersion {
		_ = fs.MkdirAll("content/v1.0/posts", 0755)
		oldPostContent := `---
title: "Old Post"
date: 2025-03-06
tags: ["test"]
---
# Old Post
This is an old version.
`
		_ = afero.WriteFile(fs, "content/v1.0/posts/old.md", []byte(oldPostContent), 0644)
	}

	// 3. Create theme
	themeDir := "themes/test-theme/templates"
	_ = fs.MkdirAll(themeDir, 0755)

	layoutTmpl := `
<!DOCTYPE html>
<html>
<head><title>{{ .Title }}</title></head>
<body>
    {{ .Content }}
</body>
</html>
`
	_ = afero.WriteFile(fs, filepath.Join(themeDir, "layout.html"), []byte(layoutTmpl), 0644)

	indexTmpl := `
<!DOCTYPE html>
<html>
<head><title>{{ .Title }}</title></head>
<body>
    <h1>Index</h1>
    {{ range .Posts }}
        <a href="{{ .Link }}">{{ .Title }}</a>
    {{ end }}
</body>
</html>
`
	_ = afero.WriteFile(fs, filepath.Join(themeDir, "index.html"), []byte(indexTmpl), 0644)

	notFoundTmpl := `<html><body>404 Not Found</body></html>`
	_ = afero.WriteFile(fs, filepath.Join(themeDir, "404.html"), []byte(notFoundTmpl), 0644)

	graphTmpl := `
<!DOCTYPE html>
<html>
<head><title>{{ .Title }}</title></head>
<body>
    <h1>Graph View</h1>
    <div id="graph"></div>
</body>
</html>
`
	_ = afero.WriteFile(fs, filepath.Join(themeDir, "graph.html"), []byte(graphTmpl), 0644)

	// 4. Create static dir
	_ = fs.MkdirAll("themes/test-theme/static/css", 0755)
	_ = afero.WriteFile(fs, "themes/test-theme/static/css/style.css", []byte("body { color: red; }"), 0644)
}
