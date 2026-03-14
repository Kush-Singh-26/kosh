package generators

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
)

func TestGenerateSW(t *testing.T) {
	sink := mocks.NewMemSink()
	destDir := "output"
	buildVersion := int64(123)
	forceRebuild := true
	baseURL := "https://example.com"
	assets := map[string]string{
		"/static/css/layout.css": "layout.abc.css",
	}

	err := GenerateSW(sink, destDir, buildVersion, forceRebuild, baseURL, assets)
	if err != nil {
		t.Fatalf("GenerateSW failed: %v", err)
	}

	// Dump written files for debugging
	// for k := range sink.Files {
	// 	t.Logf("Written file: %s", k)
	// }

	// Use filepath.ToSlash for consistent mapping check
	swPath := filepath.ToSlash(filepath.Join(destDir, "sw.js"))
	content, ok := sink.Files[swPath]
	if !ok {
		// Try without ToSlash just in case
		swPath = filepath.Join(destDir, "sw.js")
		content, ok = sink.Files[swPath]
	}

	if !ok {
		t.Fatalf("sw.js not written to sink (tried %s)", swPath)
	}

	swStr := string(content)
	if !strings.Contains(swStr, "self.addEventListener") {
		t.Error("sw.js missing service worker code")
	}
}

func TestGenerateManifest(t *testing.T) {
	sink := mocks.NewMemSink()
	destDir := "output"
	baseURL := "https://example.com"
	siteTitle := "My Blog"
	siteDescription := "Blog Description"
	forceRebuild := true

	err := GenerateManifest(sink, destDir, baseURL, siteTitle, siteDescription, forceRebuild)
	if err != nil {
		t.Fatalf("GenerateManifest failed: %v", err)
	}

	manifestPath := filepath.ToSlash(filepath.Join(destDir, "manifest.json"))
	content, ok := sink.Files[manifestPath]
	if !ok {
		manifestPath = filepath.Join(destDir, "manifest.json")
		content, ok = sink.Files[manifestPath]
	}

	if !ok {
		t.Fatalf("manifest.json not written to sink (tried %s)", manifestPath)
	}

	manifestStr := string(content)
	if !strings.Contains(manifestStr, "\"name\": \"My Blog\"") {
		t.Error("manifest.json missing name")
	}
	if !strings.Contains(manifestStr, "\"description\": \"Blog Description\"") {
		t.Error("manifest.json missing description")
	}
}
