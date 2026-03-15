package generators

import (
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestGenerateSitemap(t *testing.T) {
	sink := testutil.NewMemSink()
	baseURL := "https://example.com"
	posts := []models.PostMetadata{
		{
			Title:   "Post 1",
			Link:    "https://example.com/post1",
			DateObj: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
		},
	}
	tags := map[string][]models.PostMetadata{
		"go": posts,
	}
	outputPath := "sitemap.xml"

	_, err := GenerateSitemap(sink, baseURL, posts, tags, outputPath)
	if err != nil {
		t.Fatalf("GenerateSitemap failed: %v", err)
	}

	content, ok := sink.Files[outputPath]
	if !ok {
		t.Fatal("Sitemap file not written to sink")
	}

	sitemapStr := string(content)
	if !strings.Contains(sitemapStr, "<loc>https://example.com/</loc>") {
		t.Error("Sitemap missing home page")
	}
	if !strings.Contains(sitemapStr, "<loc>https://example.com/post1</loc>") {
		t.Error("Sitemap missing post link")
	}
	if !strings.Contains(sitemapStr, "<loc>https://example.com/tags/go.html</loc>") {
		t.Error("Sitemap missing tag link")
	}
}

func TestGenerateSitemap_EmptyPosts(t *testing.T) {
	sink := testutil.NewMemSink()
	_, err := GenerateSitemap(sink, "https://example.com", []models.PostMetadata{}, nil, "sitemap.xml")
	if err != nil {
		t.Fatalf("GenerateSitemap failed with empty posts: %v", err)
	}

	content, ok := sink.Files["sitemap.xml"]
	if !ok {
		t.Fatal("Sitemap file not written to sink")
	}

	if !strings.Contains(string(content), "<loc>https://example.com/</loc>") {
		t.Error("Sitemap missing home page link")
	}
}
