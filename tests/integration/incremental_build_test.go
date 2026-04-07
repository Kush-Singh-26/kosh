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

func TestIncrementalBuildIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()

	fs := afero.NewOsFs()
	testutil.ScaffoldTestSite(fs)

	cfg := config.LoadFs(fs, []string{})

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
	cfg.IsDev = true // Enable dev mode for incremental rebuilds

	b := orchestration.NewEngine(orchestration.WithFs(fs), orchestration.WithConfig(cfg))
	defer b.Close()

	ctx := context.Background()

	// 1. Initial Build
	if err := b.Build(ctx); err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}
	b.Close()

	// 2. Modify a post
	postPath := filepath.Join(cfg.ContentDir, "posts/hello.md")
	updatedContent := `---
title: "Updated Post"
date: 2026-03-06
tags: ["test"]
---
# Updated Post
This content was updated.`
	_ = afero.WriteFile(fs, postPath, []byte(updatedContent), 0644)

	// Create a NEW builder for the incremental part to ensure isCleanBuild is false
	b = orchestration.NewEngine(orchestration.WithFs(fs), orchestration.WithConfig(cfg))
	defer b.Close()

	// Run incremental build
	if err := b.Build(ctx); err != nil {
		t.Fatalf("Incremental build failed: %v", err)
	}

	// 3. Verify files updated on disk
	expectedPostHTML := filepath.Join(cfg.OutputDir, "posts/hello.html")
	if exists, _ := afero.Exists(fs, expectedPostHTML); !exists {
		t.Error("post page should exist")
	}

	// Verify content change
	postBytes, _ := afero.ReadFile(fs, expectedPostHTML)
	postHTML := string(postBytes)
	if !strings.Contains(postHTML, "Updated Post") {
		t.Error("Updated content not found in post HTML")
	}
}
