// Package testutil provides testing utilities and fixtures
package testutil

import (
	"html/template"
	"strings"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

const (
	sampleYear         = 2026
	sampleMonth        = time.January
	sampleDay          = 15
	sampleHour         = 10
	sampleWeight       = 10
	sampleWordCount    = 150
	sampleDocLen       = 10
	largeHTMLSize      = 35000
	samplePostsPerPage = 10
)

// CreateSamplePostMeta creates a valid PostMeta for testing
func CreateSamplePostMeta() *cache.PostMeta {
	return &cache.PostMeta{
		PostID:      "posts/test-post.md",
		Path:        "content/posts/test-post.md",
		Title:       "Test Post",
		Date:        time.Date(sampleYear, sampleMonth, sampleDay, sampleHour, 0, 0, 0, time.UTC),
		Taxonomies:  map[string][]string{"tags": {"test", "go", "tutorial"}},
		Description: "A test post for testing purposes",
		IsDraft:     false,
		Weight:      sampleWeight,
		WordCount:   sampleWordCount,
		ReadingTime: 1,
		// Meta values mirror YAML frontmatter decoding (string, bool, int/float64, time.Time, []any, map[string]any).
		Meta: make(map[string]any),
	}
}

// CreateSamplePageData creates valid PageData for testing
func CreateSamplePageData() models.PageData {
	return models.PageData{
		Title:       "Test Page",
		Description: "Test page description",
		Content:     template.HTML("<p>Test content</p>"),
		// Meta values mirror YAML frontmatter decoding (string, bool, int/float64, time.Time, []any, map[string]any).
		Meta: map[string]any{
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
		BM25Data:        map[string]int{"test": 1, "post": 1},
		DocLen:          sampleDocLen,
		Taxonomies:      map[string][]string{"tags": {"test", "go"}},
		NormalizedTaxs:  map[string][]string{"tags": {"test", "go"}},
	}
}

// CreateSampleDependencies creates valid Dependencies for testing
func CreateSampleDependencies() *cache.Dependencies {
	return &cache.Dependencies{
		Templates:  []string{"layouts/post.html", "partials/header.html"},
		Taxonomies: map[string][]string{"tags": {"go", "tutorial"}},
		Includes:   []string{"partials/footer.html"},
	}
}

// CreateSampleConfig creates a valid Config for testing
func CreateSampleConfig() *config.Config {
	return &config.Config{
		SiteConfig: config.SiteConfig{
			Title:       "Test Site",
			Description: "A test site",
			BaseURL:     "https://example.com",
			Author:      models.AuthorConfig{Name: "Test Author", URL: "https://author.example.com"},
			Taxonomies:  map[string]string{"tags": "tags"},
		},
		PathConfig: config.PathConfig{
			ContentDir:  "content",
			OutputDir:   "public",
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: "themes/test-theme/templates",
			StaticDir:   "themes/test-theme/static",
			CacheDir:    ".kosh-cache",
		},
		BuildOptions: config.BuildOptions{
			PostsPerPage: samplePostsPerPage,
		},
		Features: models.FeaturesConfig{
			Generators: models.GeneratorsConfig{
				IsSitemapEnabled: true,
				IsRSSEnabled:     true,
				Graph:           models.GraphConfig{IsEnabled: true, ShowsTaxonomies: true},
				IsPWAEnabled:     false,
				IsSearchEnabled:  true,
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
	var result strings.Builder
	result.WriteString(tags[0])
	for i := 1; i < len(tags); i++ {
		result.WriteString(`", "` + tags[i])
	}
	return result.String()
}

// CreateSmallHTML creates HTML content smaller than models.InlineHTMLThreshold
func CreateSmallHTML() []byte {
	return []byte("<p>Small content</p>")
}

// CreateLargeHTML creates HTML content larger than models.InlineHTMLThreshold
func CreateLargeHTML() []byte {
	// Create content larger than 32KB
	content := make([]byte, largeHTMLSize)
	for i := range content {
		content[i] = 'x'
	}
	return content
}
