package generators

import (
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestGenerateSitemap(t *testing.T) {
	sink := testutil.NewMemSink()
	baseURL := "https://example.com"
	items := []models.ContentMetadata{
		{
			Title:   "Item 1",
			Link:    "https://example.com/item1",
			DateObj: time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
		},
	}
	taxonomies := map[string]map[string][]models.ContentMetadata{
		"tags": {
			"go": items,
		},
	}
	outputPath := "sitemap.xml"

	_, err := GenerateSitemap(SitemapOptions{
		Sink:       sink,
		BaseURL:    baseURL,
		Items:      items,
		Taxonomies: taxonomies,
		OutputPath: outputPath,
	})
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
	if !strings.Contains(sitemapStr, "<loc>https://example.com/item1</loc>") {
		t.Error("Sitemap missing item link")
	}
	if !strings.Contains(sitemapStr, "<loc>https://example.com/tags/go.html</loc>") {
		t.Error("Sitemap missing tag link")
	}
}

func TestGenerateSitemap_EmptyItems(t *testing.T) {
	sink := testutil.NewMemSink()
	_, err := GenerateSitemap(SitemapOptions{
		Sink:       sink,
		BaseURL:    "https://example.com",
		Items:      []models.ContentMetadata{},
		Taxonomies: nil,
		OutputPath: "sitemap.xml",
	})
	if err != nil {
		t.Fatalf("GenerateSitemap failed with empty items: %v", err)
	}

	content, ok := sink.Files["sitemap.xml"]
	if !ok {
		t.Fatal("Sitemap file not written to sink")
	}

	if !strings.Contains(string(content), "<loc>https://example.com/</loc>") {
		t.Error("Sitemap missing home page link")
	}
}
