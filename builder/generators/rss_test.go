package generators

import (
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestGenerateRSS(t *testing.T) {
	sink := testutil.NewMemSink()
	baseURL := "https://example.com"
	items := []models.ContentMetadata{
		{
			Title:       "Item 1",
			Link:        "/item1",
			Description: "Description 1",
			DateObj:     time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			Title:       "Item 2",
			Link:        "/item2",
			Description: "Description 2",
			DateObj:     time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
		},
	}
	title := "My Blog"
	description := "Blog Description"
	outputPath := "rss.xml"

	_, err := GenerateRSS(RSSOptions{
		Sink:        sink,
		BaseURL:     baseURL,
		Items:       items,
		Title:       title,
		Description: description,
		OutputPath:  outputPath,
	})
	if err != nil {
		t.Fatalf("GenerateRSS failed: %v", err)
	}

	content, ok := sink.Files[outputPath]
	if !ok {
		t.Fatal("RSS file not written to sink")
	}

	rssStr := string(content)
	if !strings.Contains(rssStr, "<title>My Blog</title>") {
		t.Error("RSS missing channel title")
	}
	if !strings.Contains(rssStr, "<link>https://example.com</link>") {
		t.Error("RSS missing channel link")
	}
	if !strings.Contains(rssStr, "<title>Item 1</title>") {
		t.Error("RSS missing item title")
	}
}

func TestGenerateRSS_EmptyItems(t *testing.T) {
	sink := testutil.NewMemSink()
	_, err := GenerateRSS(RSSOptions{
		Sink:        sink,
		BaseURL:     "https://example.com",
		Items:       []models.ContentMetadata{},
		Title:       "Empty Blog",
		Description: "No items",
		OutputPath:  "rss.xml",
	})
	if err != nil {
		t.Fatalf("GenerateRSS failed with empty items: %v", err)
	}

	content, ok := sink.Files["rss.xml"]
	if !ok {
		t.Fatal("RSS file not written to sink")
	}

	if !strings.Contains(string(content), "<channel>") {
		t.Error("RSS missing channel tag")
	}
}

func TestGenerateRSS_SpecialCharacters(t *testing.T) {
	sink := testutil.NewMemSink()
	items := []models.ContentMetadata{
		{
			Title:       "Item & <More>",
			Link:        "/item?id=1&name=test",
			Description: "Special chars: \"quote\" 'apostrophe'",
			DateObj:     time.Now(),
		},
	}

	_, err := GenerateRSS(RSSOptions{
		Sink:        sink,
		BaseURL:     "https://example.com",
		Items:       items,
		Title:       "Blog & Sitemap",
		Description: "Desc <tag>",
		OutputPath:  "rss.xml",
	})
	if err != nil {
		t.Fatalf("GenerateRSS failed with special characters: %v", err)
	}

	rssStr := string(sink.Files["rss.xml"])
	// XML encoder should escape these
	if !strings.Contains(rssStr, "Item &amp; &lt;More&gt;") {
		t.Error("RSS title not escaped correctly")
	}
	if !strings.Contains(rssStr, "/item?id=1&amp;name=test") {
		t.Error("RSS link not escaped correctly")
	}
}
