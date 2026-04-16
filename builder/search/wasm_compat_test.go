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
		Items: map[string]models.ContentRecord{
			"0": {
				ID:              0,
				Title:           "Test Item",
				NormalizedTitle: "test item",
				Link:            "/items/test.html",
				Description:     "A test item",
				Taxonomies:      map[string][]string{"tags": {"test", "demo"}},
				NormalizedTaxs:  map[string][]string{"tags": {"test", "demo"}},
				Content:         "This is the full content of the test item for snippet extraction.",
			},
		},
		ItemLens:   map[string]int64{"0": 12},
		AvgDocLen:  12.0,
		TotalItems: 1,
		StemMap:    map[string][]string{"test": {"tests", "testing"}},
		Inverted: map[string]map[string][]uint32{
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

	if len(decoded.Items) != len(original.Items) {
		t.Fatalf("Items length mismatch: got %d, want %d", len(decoded.Items), len(original.Items))
	}

	if decoded.Items["0"].Content != original.Items["0"].Content {
		t.Errorf("Content field mismatch: got %q, want %q", decoded.Items["0"].Content, original.Items["0"].Content)
	}

	if !reflect.DeepEqual(decoded.Inverted, original.Inverted) {
		t.Errorf("Inverted index mismatch: got %v, want %v", decoded.Inverted, original.Inverted)
	}

	if !reflect.DeepEqual(decoded.ItemLens, original.ItemLens) {
		t.Errorf("ItemLens mismatch: got %v, want %v", decoded.ItemLens, original.ItemLens)
	}
}
