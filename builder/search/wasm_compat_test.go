package search

import (
	"reflect"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
)

func TestSearchIndex_RoundTrip(t *testing.T) {
	indexedPosts := []models.IndexedContent{
		{
			DenseID: 0,
			Record: models.ContentRecord{
				Title:       "Test Item",
				Link:        "/items/test.html",
				Description: "A test item",
				Taxonomies:  map[string][]string{"tags": {"test", "demo"}},
				Content:     "This is the full content of the test item.",
			},
			WordFreqs:       map[string]int{"test": 1},
			DocLen:          10,
			PositionalIndex: map[string][]uint32{"test": {0}},
		},
	}

	original := index.Build(indexedPosts)

	// Encode
	encoded, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to encode: %v", err)
	}

	// Decode
	var decoded models.SearchIndex
	if _, err := decoded.UnmarshalMsg(encoded); err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify
	if decoded.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", decoded.SchemaVersion, original.SchemaVersion)
	}

	if len(decoded.Items) != len(original.Items) {
		t.Fatalf("Items length mismatch: got %d, want %d", len(decoded.Items), len(original.Items))
	}

	if decoded.Items[0].Content != original.Items[0].Content {
		t.Errorf("Content mismatch: got %q, want %q", decoded.Items[0].Content, original.Items[0].Content)
	}

	if !reflect.DeepEqual(decoded.Terms, original.Terms) {
		t.Error("Lexicon mismatch")
	}

	if !reflect.DeepEqual(decoded.PostingOffsets, original.PostingOffsets) {
		t.Error("PostingOffsets mismatch")
	}
}
