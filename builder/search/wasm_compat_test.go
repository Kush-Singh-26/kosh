package search

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestSearchIndex_RoundTrip(t *testing.T) {
	original := &models.SearchIndex{
		SchemaVersion: models.CurrentSchemaVersion,
		Posts: []models.PostRecord{
			{
				ID:              0,
				Title:           "Test Post",
				NormalizedTitle: "test post",
				Link:            "/posts/test.html",
				Description:     "A test post",
				Tags:            []string{"test", "demo"},
				NormalizedTags:  []string{"test", "demo"},
				Content:         "This is the full content of the test post for snippet extraction.",
				Version:         "v1.0",
			},
		},
		DocLens:   map[string]int64{"0": 12},
		AvgDocLen: 12.0,
		TotalDocs: 1,
		StemMap:   map[string][]string{"test": {"tests", "testing"}},
		Inverted: map[string]map[string][]int{
			"test": {"0": {0, 5, 10}},
		},
	}

	// Encode
	var buf bytes.Buffer
	encoded, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to encode SearchIndex: %v", err)
	}
	buf.Write(encoded)

	// Decode
	var decoded models.SearchIndex
	if _, err := decoded.UnmarshalMsg(buf.Bytes()); err != nil {
		t.Fatalf("Failed to decode SearchIndex: %v", err)
	}

	// Verify
	if decoded.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: got %d, want %d", decoded.SchemaVersion, original.SchemaVersion)
	}

	if len(decoded.Posts) != len(original.Posts) {
		t.Fatalf("Posts length mismatch: got %d, want %d", len(decoded.Posts), len(original.Posts))
	}

	if decoded.Posts[0].Content != original.Posts[0].Content {
		t.Errorf("Content field mismatch: got %q, want %q", decoded.Posts[0].Content, original.Posts[0].Content)
	}

	if !reflect.DeepEqual(decoded.Inverted, original.Inverted) {
		t.Errorf("Inverted index mismatch: got %v, want %v", decoded.Inverted, original.Inverted)
	}

	if !reflect.DeepEqual(decoded.DocLens, original.DocLens) {
		t.Errorf("DocLens mismatch: got %v, want %v", decoded.DocLens, original.DocLens)
	}
}
