// Package testutil provides testing utilities and fixtures
package testutil

import (
	"html/template"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// CreateSamplePostMeta creates a valid PostMeta for testing
func CreateSamplePostMeta() *cache.PostMeta {
	return &cache.PostMeta{
		PostID:      "test-post",
		Title:       "Test Post",
		Path:        "content/posts/test-post.md",
		Date:        time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Tags:        []string{"test", "go", "tutorial"},
		Description: "A test post for testing purposes",
		Draft:       false,
		Weight:      10,
		WordCount:   150,
		ReadingTime: 1,
		Meta:        make(map[string]interface{}),
	}
}

// CreateSamplePostMetaWithVersion creates a PostMeta with version info
func CreateSamplePostMetaWithVersion(version string) *cache.PostMeta {
	post := CreateSamplePostMeta()
	post.Version = version
	post.Path = "content/" + version + "/posts/test-post.md"
	return post
}

// CreateSamplePageData creates valid PageData for testing
func CreateSamplePageData() models.PageData {
	return models.PageData{
		Title:       "Test Page",
		Description: "Test page description",
		Content:     template.HTML("<p>Test content</p>"),
		Meta: map[string]interface{}{
			"title":       "Test Page",
			"description": "Test page description",
		},
	}
}

// CreateSampleSearchRecord creates a valid SearchRecord for testing
func CreateSampleSearchRecord() *cache.SearchRecord {
	return &cache.SearchRecord{
		Title:           "Test Post",
		NormalizedTitle: "test post",
		Tokens:          []string{"test", "post"},
		BM25Data:        map[string]int{"test": 1, "post": 1},
		DocLen:          10,
		Content:         "This is test content for search indexing",
		NormalizedTags:  []string{"test", "go"},
		Words:           []string{"this", "is", "test", "content"},
	}
}

// CreateSampleDependencies creates valid Dependencies for testing
func CreateSampleDependencies() *cache.Dependencies {
	return &cache.Dependencies{
		Templates: []string{"layouts/post.html", "partials/header.html"},
		Tags:      []string{"go", "tutorial"},
		Includes:  []string{"partials/footer.html"},
	}
}

// CreateSampleConfig creates a valid Config for testing
func CreateSampleConfig() *config.Config {
	return &config.Config{
		Title:        "Test Site",
		Description:  "A test site",
		BaseURL:      "https://example.com",
		Author:       config.AuthorConfig{Name: "Test Author", URL: "https://author.example.com"},
		ContentDir:   "content",
		OutputDir:    "public",
		Theme:        "test-theme",
		ThemeDir:     "themes",
		TemplateDir:  "themes/test-theme/templates",
		StaticDir:    "themes/test-theme/static",
		CacheDir:     ".kosh-cache",
		Language:     "en",
		PostsPerPage: 10,
		Features: config.FeaturesConfig{
			Generators: config.GeneratorsConfig{
				Sitemap: true,
				RSS:     true,
				Graph:   true,
				PWA:     false,
				Search:  true,
			},
		},
	}
}

// CreateTestMarkdown creates sample markdown content for testing
func CreateTestMarkdown() string {
	return `---
title: "Test Post"
date: 2026-01-15
tags: ["test", "go"]
---

# Test Post

This is a test post for testing purposes.

## Section 1

Some content here.

## Section 2

More content here with **bold** and *italic* text.

- List item 1
- List item 2
- List item 3

[Link to example](https://example.com)
`
}

// CreateTestMarkdownWithFrontmatter creates markdown with specific frontmatter
func CreateTestMarkdownWithFrontmatter(title string, date time.Time, tags []string) string {
	return `---
title: "` + title + `"
date: ` + date.Format("2006-01-02") + `
tags: ["` + joinTags(tags) + `"]
---

# ` + title + `

Test content for ` + title + `.
`
}

func joinTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	result := tags[0]
	for i := 1; i < len(tags); i++ {
		result += `", "` + tags[i]
	}
	return result
}

// CreateSmallHTML creates HTML content smaller than InlineHTMLThreshold
func CreateSmallHTML() []byte {
	return []byte("<p>Small content</p>")
}

// CreateLargeHTML creates HTML content larger than InlineHTMLThreshold
func CreateLargeHTML() []byte {
	// Create content larger than 32KB
	content := make([]byte, 35000)
	for i := range content {
		content[i] = 'x'
	}
	return content
}
