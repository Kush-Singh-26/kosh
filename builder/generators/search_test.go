package generators

import (
	"bytes"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/andybalholm/brotli"
	"github.com/tinylib/msgp/msgp"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestGenerateSearchIndex(t *testing.T) {
	sink := testutil.NewMemSink()

	indexedItems := []models.IndexedContent{
		{
			DenseID: 0,
			Record: models.ContentRecord{
				Title:   "Item 1",
				Link:    "/item1.html",
				Content: "Body of item 1",
			},
			DocLen: 4,
			PositionalIndex: map[string][]uint32{
				"body": {0},
				"item": {2},
			},
			StemMap: map[string]string{
				"body": "body",
				"item": "item",
			},
		},
	}

	resultPath, size, err := GenerateSearchIndex(sink, indexedItems, models.SearchRankingConfig{})
	if err != nil {
		t.Fatalf("GenerateSearchIndex failed: %v", err)
	}

	if size == 0 {
		t.Error("Expected non-zero size")
	}

	expectedPath := "search.bin"
	if resultPath != expectedPath {
		t.Errorf("Expected result path %s, got %s", expectedPath, resultPath)
	}

	content, ok := sink.Files[expectedPath]
	if !ok {
		t.Fatalf("Search index file not found in sink at %s", expectedPath)
	}

	// Verify decoding
	var index models.SearchIndex
	br := brotli.NewReader(bytes.NewReader(content))
	mr := msgp.NewReader(br)
	if err := index.DecodeMsg(mr); err != nil {
		t.Fatalf("Failed to decode search index: %v", err)
	}

	if index.SchemaVersion != models.CurrentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", models.CurrentSchemaVersion, index.SchemaVersion)
	}

	if index.TotalItems != 1 {
		t.Errorf("Expected 1 item, got %d", index.TotalItems)
	}

	// Verify item record
	if len(index.Items) != 1 {
		t.Fatal("Expected 1 item record in index")
	}
	item := index.Items[0]
	if item.Title != "Item 1" {
		t.Errorf("Expected item title Item 1, got %s", item.Title)
	}

	// Verify CSR Lexicon
	foundBody := false
	for _, term := range index.Terms {
		if term == "body" {
			foundBody = true
			break
		}
	}
	if !foundBody {
		t.Error("Lexicon entry for 'body' missing")
	}

	// Verify CSR Posting Table
	if len(index.DocIDs) == 0 {
		t.Error("Expected DocIDs postings to be generated")
	}
}

func TestGenerateSearchIndex_Empty(t *testing.T) {
	sink := testutil.NewMemSink()
	_, _, err := GenerateSearchIndex(sink, []models.IndexedContent{}, models.SearchRankingConfig{})
	if err != nil {
		t.Fatalf("GenerateSearchIndex failed with empty items: %v", err)
	}

	content := sink.Files["search.bin"]
	if len(content) == 0 {
		t.Fatal("Expected search.bin to be written even for empty input")
	}

	var index models.SearchIndex
	br := brotli.NewReader(bytes.NewReader(content))
	mr := msgp.NewReader(br)
	if err := index.DecodeMsg(mr); err != nil {
		t.Fatalf("Failed to decode empty search index: %v", err)
	}

	if index.TotalItems != 0 {
		t.Errorf("Expected 0 total items, got %d", index.TotalItems)
	}
}

func TestGenerateSearchIndex_Nil(t *testing.T) {
	sink := testutil.NewMemSink()
	_, _, err := GenerateSearchIndex(sink, nil, models.SearchRankingConfig{})
	if err != nil {
		t.Fatalf("GenerateSearchIndex failed with nil items: %v", err)
	}

	if _, ok := sink.Files["search.bin"]; !ok {
		t.Error("Expected search.bin to be written for nil input")
	}
}
