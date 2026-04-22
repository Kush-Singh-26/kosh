package scanner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestMetadataScanner_Scan(t *testing.T) {
	sourceFs := afero.NewMemMapFs()
	cfg := testutil.CreateSampleConfig()
	contentDir := cfg.ContentDir

	// Create a test site structure
	files := map[string]string{
		filepath.Join(contentDir, "posts/post1.md"): `---
title: "Post 1"
date: 2026-03-01
tags: ["go", "testing"]
pinned: true
---
Body of post 1`,
		filepath.Join(contentDir, "v1.0/posts/old-post.md"): `---
title: "Old Post"
date: 2025-01-01
tags: ["legacy"]
---
Body of old post`,
		filepath.Join(contentDir, "404.md"): `---
title: "404"
---
Not found`,
		filepath.Join(contentDir, "static/image.png"): "fake-png-content",
		filepath.Join(contentDir, "posts/draft.md"): `---
title: "Draft Post"
draft: true
---
Body of draft`,
	}

	for path, content := range files {
		_ = sourceFs.MkdirAll(filepath.Dir(path), 0755)
		_ = afero.WriteFile(sourceFs, path, []byte(content), 0644)
	}

	scanner := NewScanner()
	fileChan := make(chan models.ScannedResource, 10)

	ctx := context.Background()
	result, err := scanner.Scan(ScanOptions{
		Ctx:        ctx,
		ContentDir: contentDir,
		SrcFs:      sourceFs,
		Cfg:        cfg,
		FileChan:   fileChan,
	})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Verify results
	if !result.Has404 {
		t.Error("Expected Has404 to be true")
	}

	// In the updated scanner, result.Files contains all markdown files
	if len(result.Files) != 4 { // post1, old-post, draft, 404
		t.Errorf("Expected 4 markdown files, got %d", len(result.Files))
	}

	// Verify Post 1
	var post1 models.ScannedResource
	for _, f := range result.Files {
		if f.Title == "Post 1" {
			post1 = f
			break
		}
	}
	if post1.Title == "" {
		t.Fatal("Post 1 not found in scanned files")
	}
	if !post1.IsPinned {
		t.Error("Post 1 should be pinned")
	}
	if len(post1.Taxonomies["tags"]) != 2 {
		t.Errorf("Expected 2 tags for Post 1, got %d", len(post1.Taxonomies["tags"]))
	}

	// Verify Assets
	foundImage := false
	for _, asset := range result.ContentAssets {
		if filepath.Base(asset.Path) == "image.png" {
			foundImage = true
			break
		}
	}
	if !foundImage {
		t.Error("Static image asset not found")
	}

	// Verify fileChan
	close(fileChan)
	count := 0
	for range fileChan {
		count++
	}
	if count != 4 {
		t.Errorf("Expected 4 files in fileChan, got %d", count)
	}
}

func TestMetadataScanner_ScanFile(t *testing.T) {
	scanner := NewScanner()
	cfg := testutil.CreateSampleConfig()
	sourceFs := afero.NewMemMapFs()
	path := filepath.Join(cfg.ContentDir, "test.md")

	content := `---
title: "Test Post"
date: 2026-03-01
tags: ["go", "test"]
pinned: true
description: "Test Desc"
---
Body content`

	_ = sourceFs.MkdirAll(cfg.ContentDir, 0755)
	_ = afero.WriteFile(sourceFs, path, []byte(content), 0644)

	sf, err := scanner.ScanFile(sourceFs, cfg, path)
	if err != nil {
		t.Fatalf("ScanFile failed: %v", err)
	}

	if sf.Title != "Test Post" {
		t.Errorf("Expected title Test Post, got %s", sf.Title)
	}
	if sf.Description != "Test Desc" {
		t.Errorf("Expected description Test Desc, got %s", sf.Description)
	}
	if sf.Date != "2026-03-01" {
		t.Errorf("Expected date 2026-03-01, got %s", sf.Date)
	}
	if !sf.IsPinned {
		t.Error("Expected pinned to be true")
	}
	if len(sf.Taxonomies["tags"]) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(sf.Taxonomies["tags"]))
	}
	if sf.FrontmatterHash == "" {
		t.Error("Expected non-empty frontmatter hash")
	}
	if sf.BodyHash != "" {
		t.Error("Expected empty body hash due to lazy loading")
	}
	if sf.SourceLoader == nil {
		t.Error("Expected non-nil SourceLoader due to lazy loading")
	}
	if sf.BodyOffset == 0 {
		t.Error("Expected non-zero body offset")
	}
}

func TestBuildCascadedMetadataForPath(t *testing.T) {
	cfg := testutil.CreateSampleConfig()
	sourceFs := afero.NewMemMapFs()

	rootIndexPath := filepath.Join(cfg.ContentDir, "_index.md")
	sectionIndexPath := filepath.Join(cfg.ContentDir, "blogs", "_index.md")
	postPath := filepath.Join(cfg.ContentDir, "blogs", "sample.md")

	_ = sourceFs.MkdirAll(filepath.Dir(postPath), 0755)
	_ = afero.WriteFile(sourceFs, rootIndexPath, []byte(`---
description: "Root Description"
cascade:
  layout: "home"
  tags: ["root"]
---
Root`), 0644)
	_ = afero.WriteFile(sourceFs, sectionIndexPath, []byte(`---
cascade:
  tags: ["section"]
  author: "Kosh"
---
Section`), 0644)

	source := []byte(`---
title: "Sample"
tags: ["item"]
---
Hello world`)
	_ = afero.WriteFile(sourceFs, postPath, source, 0644)

	merged, bodyOffset, err := BuildCascadedMetadataForPath(sourceFs, cfg, postPath, source)
	if err != nil {
		t.Fatalf("BuildCascadedMetadataForPath failed: %v", err)
	}

	if merged["title"] != "Sample" {
		t.Fatalf("expected title to come from file frontmatter, got %#v", merged["title"])
	}
	if merged["layout"] != "home" {
		t.Fatalf("expected layout from root cascade, got %#v", merged["layout"])
	}
	if merged["description"] != "Root Description" {
		t.Fatalf("expected root description to be inherited, got %#v", merged["description"])
	}
	if merged["author"] != "Kosh" {
		t.Fatalf("expected author from section cascade, got %#v", merged["author"])
	}

	tags, ok := merged["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "item" {
		t.Fatalf("expected file frontmatter tags to override cascade tags, got %#v", merged["tags"])
	}

	if bodyOffset <= 0 {
		t.Fatalf("expected non-zero body offset, got %d", bodyOffset)
	}
}
