package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

func TestFullBuildIntegration(t *testing.T) {
	// Use real disk for integration tests because esbuild needs it
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	fs := afero.NewOsFs()
	testutil.ScaffoldTestSite(fs)

	// Load config from VFS (OsFs)
	cfg := config.LoadFs(fs, []string{})

	// Ensure absolute paths for everything
	absOutputDir, _ := filepath.Abs("public")
	absContentDir, _ := filepath.Abs("content")
	absCacheDir, _ := filepath.Abs(".kosh-cache")
	absTemplateDir, _ := filepath.Abs("themes/test-theme/templates")
	absStaticDir, _ := filepath.Abs("themes/test-theme/static")

	cfg.OutputDir = absOutputDir
	cfg.ContentDir = absContentDir
	cfg.CacheDir = absCacheDir
	cfg.TemplateDir = absTemplateDir
	cfg.StaticDir = absStaticDir

	cfg.Features.Generators.Search = true
	cfg.Features.Generators.RSS = true
	cfg.Features.Generators.Sitemap = true
	cfg.Features.Generators.Graph = true

	// Create Builder
	b := orchestration.NewEngineWithFs(fs, cfg)
	defer b.Close()

	// Execute Build
	ctx := context.Background()
	if err := b.Build(ctx); err != nil {
		t.Fatalf("Integration build failed: %v", err)
	}

	// Verify outputs on disk
	expectedFiles := []string{
		filepath.Join(cfg.OutputDir, "index.html"),
		filepath.Join(cfg.OutputDir, "404.html"),
		filepath.Join(cfg.OutputDir, "posts/hello.html"),
		filepath.Join(cfg.OutputDir, "sitemap/sitemap.xml"),
		filepath.Join(cfg.OutputDir, "rss.xml"),
		filepath.Join(cfg.OutputDir, "search.bin"),
		filepath.Join(cfg.OutputDir, "graph.json"),
		filepath.Join(cfg.OutputDir, ".nojekyll"),
	}

	for _, f := range expectedFiles {
		if exists, _ := afero.Exists(fs, f); !exists {
			t.Errorf("Expected output file %s missing from disk", f)
			// Debug: list files in OutputDir
			files, _ := afero.ReadDir(fs, cfg.OutputDir)
			t.Logf("Files in %s:", cfg.OutputDir)
			for _, file := range files {
				t.Logf("  - %s", file.Name())
			}
		}
	}

	// Basic content check
	indexBytes, _ := afero.ReadFile(fs, filepath.Join(cfg.OutputDir, "index.html"))
	indexContent := string(indexBytes)
	if !strings.Contains(indexContent, "Latest Post") {
		t.Error("index.html missing post title")
	}

	postBytes, _ := afero.ReadFile(fs, filepath.Join(cfg.OutputDir, "posts/hello.html"))
	postContent := string(postBytes)
	if !strings.Contains(postContent, "Latest Post") {
		t.Error("post page missing title")
	}
	if !strings.Contains(postContent, "This is a test post.") {
		t.Error("post page missing body content")
	}
}
