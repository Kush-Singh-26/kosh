package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateOverride(t *testing.T) {
	// 1. Setup paths
	wd, _ := os.Getwd()
	repoRoot := filepath.Dir(wd)
	mockSiteDir := filepath.Join(repoRoot, "tests", "fixtures", "override-site")

	// Cleanup and create fresh mock site
	_ = os.RemoveAll(mockSiteDir)
	require.NoError(t, os.MkdirAll(filepath.Join(mockSiteDir, "content"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(mockSiteDir, "layouts"), 0755))

	koshYaml := `
title: "Override Site"
theme: "blog"
themeDir: "../../themes"
layoutsDir: "layouts"
contentDir: "content"
outputDir: "public"
`
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "kosh.yaml"), []byte(koshYaml), 0644))

	// Site-level override layout
	overrideLayout := `{{ define "content" }}OVERRIDE_ACTIVE: {{ .Title }}{{ end }}`
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "layouts", "layout.html"), []byte(overrideLayout), 0644))

	// Content
	postContent := `---
title: "Test Post"
---
Body`
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "content", "post.md"), []byte(postContent), 0644))

	// 2. Load Config
	err := os.Chdir(mockSiteDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(wd) }()

	cfg := config.Load(nil)
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.LayoutsDir = filepath.Join(mockSiteDir, "layouts")
	cfg.ThemeDir = filepath.Join(repoRoot, "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")

	// 3. Initialize Engine
	sourceFs := afero.NewOsFs()
	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(&mockReporter{}),
	)

	// 4. Run Build
	err = engine.Build(context.Background())
	require.NoError(t, err)

	// 5. Assertions
	htmlPath := filepath.Join(cfg.OutputDir, "post.html")
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)

	// Verify that the override template was used
	assert.Contains(t, string(htmlContent), "OVERRIDE_ACTIVE: Test Post")
	assert.Contains(t, string(htmlContent), "<!doctype html>") // Base layout has DOCTYPE
}

func TestSectionDetection(t *testing.T) {
	// Setup same paths as above or use existing
	wd, _ := os.Getwd()
	repoRoot := filepath.Dir(wd)
	mockSiteDir := filepath.Join(repoRoot, "tests", "fixtures", "section-site")

	_ = os.RemoveAll(mockSiteDir)
	require.NoError(t, os.MkdirAll(filepath.Join(mockSiteDir, "content", "projects"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(mockSiteDir, "content", "blogs"), 0755))

	koshYaml := `
title: "Section Site"
themeDir: "../../themes"
contentDir: "content"
outputDir: "public"
`
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "kosh.yaml"), []byte(koshYaml), 0644))

	// Projects content
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "content", "projects", "p1.md"), []byte("---\ntitle: P1\n---\n"), 0644))
	// Blogs content
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "content", "blogs", "b1.md"), []byte("---\ntitle: B1\n---\n"), 0644))

	_ = os.Chdir(mockSiteDir) // best-effort cleanup
	defer func() { _ = os.Chdir(wd) }()

	cfg := config.Load(nil)
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.ThemeDir = filepath.Join(repoRoot, "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")

	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(afero.NewOsFs()),
		orchestration.WithReporter(&mockReporter{}),
	)

	err := engine.Build(context.Background())
	require.NoError(t, err)

	// we need a way to check Section metadata.
	// Since we don't have a direct way to inspect the model without modifying the engine to return it,
	// let's use a custom layout that renders the section.

	require.NoError(t, os.MkdirAll(filepath.Join(mockSiteDir, "layouts"), 0755))
	sectionLayout := `{{ define "content" }}SECTION: {{ .Section }}{{ end }}`
	require.NoError(t, os.WriteFile(filepath.Join(mockSiteDir, "layouts", "layout.html"), []byte(sectionLayout), 0644))
	cfg.LayoutsDir = filepath.Join(mockSiteDir, "layouts")

	// Delete public and cache to force cold rebuild!
	_ = os.RemoveAll(cfg.OutputDir)
	_ = os.RemoveAll(filepath.Join(mockSiteDir, ".kosh-cache"))
	cfg.ShouldForceRebuild = true

	// Re-initialize engine to be absolutely sure
	engine = orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(afero.NewOsFs()),
		orchestration.WithReporter(&mockReporter{}),
	)

	err = engine.Build(context.Background())
	require.NoError(t, err)

	p1Html, _ := os.ReadFile(filepath.Join(cfg.OutputDir, "projects", "p1.html"))
	if !assert.Contains(t, string(p1Html), "SECTION: projects") {
		t.Logf("P1 HTML: %s", string(p1Html))
	}

	b1Html, _ := os.ReadFile(filepath.Join(cfg.OutputDir, "blogs", "b1.html"))
	if !assert.Contains(t, string(b1Html), "SECTION: blogs") {
		t.Logf("B1 HTML: %s", string(b1Html))
	}
}
