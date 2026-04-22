package models

import (
	"bytes"
	"testing"

	"github.com/tinylib/msgp/msgp"
)

func TestTOCEntry_Msgp(t *testing.T) {
	v := TOCEntry{
		ID:    "test-id",
		Text:  "Test Text",
		Level: 2,
	}

	var buf bytes.Buffer
	if err := msgp.Encode(&buf, &v); err != nil {
		t.Fatal(err)
	}

	var v2 TOCEntry
	if err := msgp.Decode(&buf, &v2); err != nil {
		t.Fatal(err)
	}

	if v.ID != v2.ID || v.Text != v2.Text || v.Level != v2.Level {
		t.Errorf("TOCEntry roundtrip failed: got %+v, want %+v", v2, v)
	}
}

func TestContentRecord_Msgp(t *testing.T) {
	v := ContentRecord{
		Title:           "Title",
		Link:            "/link",
		Description:     "Desc",
		Taxonomies:      map[string][]string{"tags": {"a", "b"}},
		NormalizedTaxs:  map[string][]string{"tags": {"a", "b"}},
		Content:         "Content",
	}

	var buf bytes.Buffer
	if err := msgp.Encode(&buf, &v); err != nil {
		t.Fatal(err)
	}

	var v2 ContentRecord
	if err := msgp.Decode(&buf, &v2); err != nil {
		t.Fatal(err)
	}

	if v.Title != v2.Title || v.Content != v2.Content {
		t.Errorf("ContentRecord roundtrip failed")
	}
}

func TestSearchIndex_Msgp(t *testing.T) {
	v := SearchIndex{
		SchemaVersion: CurrentSchemaVersion,
		Items:         []ContentRecord{{Title: "P1"}},
		ItemLens:      []int32{10},
		AvgDocLen:     10.5,
		TotalItems:    1,
		Terms:         []string{"word"},
		PostingOffsets: []uint32{0, 1},
		DocIDs:         []uint32{0},
		DocPosOffsets:  []uint32{0, 2},
		PosOffsets:     []uint32{0, 2},
		Positions:      []uint32{1, 2},
	}

	var buf bytes.Buffer
	if err := msgp.Encode(&buf, &v); err != nil {
		t.Fatal(err)
	}

	var v2 SearchIndex
	if err := msgp.Decode(&buf, &v2); err != nil {
		t.Fatal(err)
	}

	if v.SchemaVersion != v2.SchemaVersion || v.TotalItems != v2.TotalItems {
		t.Errorf("SearchIndex roundtrip failed")
	}
}
