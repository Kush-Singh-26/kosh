package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncrementalRebuild(t *testing.T) {
	// 1. Setup paths
	wd, _ := os.Getwd()
	repoRoot := filepath.Dir(wd)
	mockSiteDir := filepath.Join(repoRoot, "tests", "fixtures", "mock-site-incremental")

	// Prepare fresh mock site for incremental test
	err := os.RemoveAll(mockSiteDir)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(mockSiteDir, "content", "blogs"), 0755)
	require.NoError(t, err)

	koshYaml := `
title: "Incremental Site"
baseURL: "https://example.com"
theme: "blog"
themeDir: "../../themes"
outputDir: "public"
staticDir: "static"
contentDir: "content"
contentPrefix: "blogs"
`
	err = os.WriteFile(filepath.Join(mockSiteDir, "kosh.yaml"), []byte(koshYaml), 0644)
	require.NoError(t, err)

	postContent := `---
title: "Initial Title"
date: 2026-04-14
tags: ["old"]
---
Body content`
	postPath := filepath.Join(mockSiteDir, "content", "blogs", "post.md")
	err = os.WriteFile(postPath, []byte(postContent), 0644)
	require.NoError(t, err)

	// Change WD
	err = os.Chdir(mockSiteDir)
	require.NoError(t, err)
	defer os.Chdir(wd)

	// 2. Load config and engine
	cfg := config.Load(nil)
	cfg.IsDev = true // Enable dev mode for incremental

	// Override paths
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.ThemeDir = filepath.Join(repoRoot, "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")

	sourceFs := afero.NewOsFs()
	reporter := &mockReporter{}
	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(reporter),
	)

	// 3. Initial Build
	err = engine.Build(context.Background())
	require.NoError(t, err)

	// Verify initial state
	// Note: since it's in content/blogs/post.md, output is public/blogs/post.html
	htmlPath := filepath.Join(cfg.OutputDir, "blogs", "post.html")
	initialHtml, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(initialHtml), "Initial Title")

	// 4. Modify file (Title change)
	newContent := `---
title: "Updated Title"
date: 2026-04-14
tags: ["new"]
---
Body content`
	err = os.WriteFile(postPath, []byte(newContent), 0644)
	require.NoError(t, err)

	// 5. Trigger Incremental Rebuild synchronously
	engine.Incremental.BuildSingleFileChange(context.Background(), postPath, fsnotify.Write)

	// 6. Assertions
	updatedHtml, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(updatedHtml), "Updated Title")
	assert.NotContains(t, string(updatedHtml), "Initial Title")

	// Check if tags index was updated (blogs/tags/index.html)
	tagsIndexPath := filepath.Join(cfg.OutputDir, "blogs", "tags", "index.html")
	tagsIndexHtml, err := os.ReadFile(tagsIndexPath)
	require.NoError(t, err)
	assert.Contains(t, string(tagsIndexHtml), "new")
}
