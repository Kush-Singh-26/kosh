package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type mockReporter struct{}

func (m *mockReporter) Start(_ string)                                {}
func (m *mockReporter) StartPhase(_ ui.Phase)                         {}
func (m *mockReporter) UpdateProgress(_ ui.Phase, _, _ int, _ string) {}
func (m *mockReporter) EndPhase(_ ui.Phase, _ time.Duration)          {}
func (m *mockReporter) Info(_ string, _ ...any)                       {}
func (m *mockReporter) Warn(_ string, _ ...any)                       {}
func (m *mockReporter) Error(_ string, _ error, _ ...any)             {}
func (m *mockReporter) Success(_ string)                              {}
func (m *mockReporter) Status(_ string)                               {}
func (m *mockReporter) Finish(_ ui.BuildStats)                        {}

func TestMockSiteBuild(t *testing.T) {
	// 1. Setup paths
	wd, _ := os.Getwd()
	repoRoot := filepath.Dir(wd)
	mockSiteDir := filepath.Join(repoRoot, "tests", "fixtures", "mock-site")

	// Change working directory so config.Load finds kosh.yaml
	err := os.Chdir(mockSiteDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(wd) }()

	// 2. Load config
	cfg := config.Load(nil)
	cfg.ParserWorkers = 1
	cfg.ImageWorkers = 1

	// Override paths to be absolute for the test
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.StaticDir = filepath.Join(mockSiteDir, "static")
	cfg.ThemeDir = filepath.Join(repoRoot, "tests", "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")
	// Make sure search WASM is found (usually in repo root or embedded)
	// For tests, it's often better to disable WASM or ensure it's built.

	// 3. Initialize Engine
	sourceFs := afero.NewOsFs()
	reporter := &mockReporter{}
	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(reporter),
	)
	defer func() { engine.Close() }()

	// 4. Run Build
	err = engine.Build(context.Background())
	require.NoError(t, err)

	// 5. Assertions

	// Check sitemap
	// Based on output listing, it's in public/sitemap/sitemap.xml
	sitemapPath := filepath.Join(cfg.OutputDir, "sitemap", "sitemap.xml")
	assert.FileExists(t, sitemapPath)

	// Check WebP conversion
	webpPath := filepath.Join(cfg.OutputDir, "static", "images", "test.webp")
	assert.FileExists(t, webpPath)

	// Check post rendering
	// Nested post
	postPath := filepath.Join(cfg.OutputDir, "blogs", "2026", "04", "14", "first-post.html")
	assert.FileExists(t, postPath)

	// Root post
	rootPostPath := filepath.Join(cfg.OutputDir, "post2.html")
	assert.FileExists(t, rootPostPath)

	// Check graph page
	graphPath := filepath.Join(cfg.OutputDir, "graph.html")
	assert.FileExists(t, graphPath)
	graphDataPath := filepath.Join(cfg.OutputDir, "graph.json")
	assert.FileExists(t, graphDataPath)

	// Check RSS
	// Based on output listing, it's in public/rss.xml
	rssPath := filepath.Join(cfg.OutputDir, "rss.xml")
	assert.FileExists(t, rssPath)

}
