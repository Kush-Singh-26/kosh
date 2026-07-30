package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
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
	defer func() { _ = os.Chdir(wd) }()

	// 2. Load config and engine
	cfg := config.Load(nil)
	cfg.IsDev = true // Enable dev mode for incremental

	// Override paths
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.ThemeDir = filepath.Join(repoRoot, "tests", "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")

	sourceFs := afero.NewOsFs()
	reporter := &mockReporter{}
	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(reporter),
	)
	defer func() { engine.Close() }()

	// 3. Initial Build
	err = engine.Build(context.Background())
	require.NoError(t, err)

	// Verify initial state
	// Note: since it's in content/blogs/post.md, output is public/blogs/post.html
	htmlPath := filepath.Join(cfg.OutputDir, "blogs", "post.html")
	initialHTML, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(initialHTML), "Initial Title")

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
	updatedHTML, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(updatedHTML), "Updated Title")
	assert.NotContains(t, string(updatedHTML), "Initial Title")

	// Check if tags index was updated (blogs/tags/index.html)
	tagsIndexPath := filepath.Join(cfg.OutputDir, "blogs", "tags", "index.html")
	tagsIndexHTML, err := os.ReadFile(tagsIndexPath)
	require.NoError(t, err)
	assert.Contains(t, string(tagsIndexHTML), "new")
}

// TestIncrementalMoveContent verifies that moving (renaming) a content file
// removes its stale entry from listings and renders the file at the new path.
func TestIncrementalMoveContent(t *testing.T) {
	wd, _ := os.Getwd()
	repoRoot := filepath.Dir(wd)
	mockSiteDir := filepath.Join(repoRoot, "tests", "fixtures", "mock-site-move")

	err := os.RemoveAll(mockSiteDir)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(mockSiteDir, "content", "blogs"), 0755)
	require.NoError(t, err)

	koshYaml := `
title: "Move Site"
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

	err = os.Chdir(mockSiteDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(wd) }()

	cfg := config.Load(nil)
	cfg.IsDev = true
	cfg.OutputDir = filepath.Join(mockSiteDir, "public")
	cfg.ContentDir = filepath.Join(mockSiteDir, "content")
	cfg.ThemeDir = filepath.Join(repoRoot, "tests", "themes")
	cfg.Theme = "blog"
	cfg.TemplateDir = filepath.Join(cfg.ThemeDir, cfg.Theme, "templates")

	sourceFs := afero.NewOsFs()
	reporter := &mockReporter{}
	engine := orchestration.NewEngine(
		orchestration.WithConfig(cfg),
		orchestration.WithFs(sourceFs),
		orchestration.WithReporter(reporter),
	)
	defer func() { engine.Close() }()

	require.NoError(t, engine.Build(context.Background()))

	oldHTML := filepath.Join(cfg.OutputDir, "blogs", "post.html")
	require.FileExists(t, oldHTML)

	// Simulate a move: rename the source file to a new path within the section.
	newPath := filepath.Join(mockSiteDir, "content", "blogs", "renamed.md")
	require.NoError(t, os.Rename(postPath, newPath))

	// fsnotify reports a Rename on the old path and a Create on the new path.
	engine.Incremental.BuildSingleFileChange(context.Background(), postPath, fsnotify.Rename)
	engine.Incremental.BuildSingleFileChange(context.Background(), newPath, fsnotify.Create)

	// Old output should be gone, new output should exist.
	assert.NoFileExists(t, oldHTML, "stale output for moved file should be removed")
	newHTML := filepath.Join(cfg.OutputDir, "blogs", "renamed.html")
	require.FileExists(t, newHTML, "moved file should be rendered at new path")

	// The section listing must no longer reference the old path.
	listingPath := filepath.Join(cfg.OutputDir, "blogs", "index.html")
	listingHTML, err := os.ReadFile(listingPath)
	require.NoError(t, err)
	assert.NotContains(t, string(listingHTML), "blogs/post.html", "stale listing entry should be removed")
	assert.Contains(t, string(listingHTML), "blogs/renamed.html", "listing should reference moved file")
}
