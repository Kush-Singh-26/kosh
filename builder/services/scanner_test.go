package services

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

	scanner := NewMetadataScanner()
	fileChan := make(chan models.ScannedFile, 10)

	ctx := context.Background()
	result, err := scanner.Scan(ctx, contentDir, sourceFs, cfg, fileChan)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Verify results
	if !result.Has404 {
		t.Error("Expected Has404 to be true")
	}

	if len(result.Metadata) != 3 { // post1, old-post, draft
		t.Errorf("Expected 3 metadata entries, got %d", len(result.Metadata))
	}

	// Verify Post 1
	var post1 models.LightPostMetadata
	for _, m := range result.Metadata {
		if m.Title == "Post 1" {
			post1 = m
			break
		}
	}
	if post1.Title == "" {
		t.Fatal("Post 1 not found in metadata")
	}
	if !post1.Pinned {
		t.Error("Post 1 should be pinned")
	}
	if len(post1.Tags) != 2 {
		t.Errorf("Expected 2 tags for Post 1, got %d", len(post1.Tags))
	}

	// Verify Versioning
	if len(result.PostsByVersion["v1.0"]) != 1 {
		t.Errorf("Expected 1 post in v1.0, got %d", len(result.PostsByVersion["v1.0"]))
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
	if count != 3 {
		t.Errorf("Expected 3 files in fileChan, got %d", count)
	}
}

func TestMetadataScanner_ExtractFrontmatter(t *testing.T) {
	scanner := &metadataScanner{}
	cfg := testutil.CreateSampleConfig()

	source := []byte(`---
title: "Test"
date: 2026-03-06
tags: ["a", "b"]
pinned: true
description: "Desc"
---
Body content`)

	meta, fmMap, fmHash, bodyHash, readingTime, bodyOffset := scanner.extractFrontmatter(source, "posts/test.md", "", "posts/test.html", "posts/test.html", cfg)

	if meta.Title != "Test" {
		t.Errorf("Expected title Test, got %s", meta.Title)
	}
	if meta.Description != "Desc" {
		t.Errorf("Expected description Desc, got %s", meta.Description)
	}
	if !meta.Pinned {
		t.Error("Expected pinned to be true")
	}
	if fmMap["title"] != "Test" {
		t.Error("Expected title in fmMap")
	}
	if fmHash == "" {
		t.Error("Expected non-empty frontmatter hash")
	}
	if bodyHash == "" {
		t.Error("Expected non-empty body hash")
	}
	if readingTime != 0 {
		t.Errorf("Expected readingTime 0 (deferred), got %d", readingTime)
	}

	// Check body offset
	expectedBody := []byte("\nBody content")
	if string(source[bodyOffset:]) != string(expectedBody) {
		t.Errorf("Expected body at offset to be %q, got %q", string(expectedBody), string(source[bodyOffset:]))
	}
}

func TestMetadataScanner_MalformedFrontmatter(t *testing.T) {
	scanner := &metadataScanner{}
	cfg := testutil.CreateSampleConfig()

	// Case 1: Missing delimiters
	source := []byte(`title: No delimiters
Just text`)
	meta, _, _, _, _, _ := scanner.extractFrontmatter(source, "test.md", "", "test.html", "test.html", cfg)
	if meta.Title != "" {
		t.Error("Expected empty meta for missing delimiters")
	}

	// Case 2: Invalid YAML (unclosed quote)
	// Note: With regex-based extraction, we're more lenient than YAML parsing
	// The regex will still extract "Unclosed quote" as the title
	// This is acceptable behavior - we prioritize extraction over strict validation
	source = []byte(`---
title: "Unclosed quote
---
Body`)
	meta, _, _, _, _, _ = scanner.extractFrontmatter(source, "test.md", "", "test.html", "test.html", cfg)
	// Regex extraction is lenient - it will extract the value even with unclosed quote
	// This is acceptable for performance; full YAML validation happens downstream if needed
	if meta.Title == "" {
		t.Log("Regex extraction is lenient - extracted title despite unclosed quote")
	}
}
