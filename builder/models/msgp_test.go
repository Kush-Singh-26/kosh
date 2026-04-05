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

func TestPostRecord_Msgp(t *testing.T) {
	v := PostRecord{
		ID:              123,
		Title:           "Title",
		NormalizedTitle: "title",
		Link:            "/link",
		Description:     "Desc",
		Tags:            []string{"a", "b"},
		NormalizedTags:  []string{"a", "b"},
		Content:         "Content",
	}

	var buf bytes.Buffer
	if err := msgp.Encode(&buf, &v); err != nil {
		t.Fatal(err)
	}

	var v2 PostRecord
	if err := msgp.Decode(&buf, &v2); err != nil {
		t.Fatal(err)
	}

	if v.ID != v2.ID || v.Title != v2.Title || v.Content != v2.Content {
		t.Errorf("PostRecord roundtrip failed")
	}
}

func TestSearchIndex_Msgp(t *testing.T) {
	v := SearchIndex{
		SchemaVersion: CurrentSchemaVersion,
		Posts:         map[string]PostRecord{"1": {ID: 1, Title: "P1"}},
		DocLens:       map[string]int64{"1": 10},
		AvgDocLen:     10.5,
		TotalDocs:     1,
		Inverted:      map[string]map[string][]uint32{"word": {"1": {1, 2}}},
		Offsets:       map[string]map[string][]uint32{"word": {"1": {0, 4}}},
	}

	var buf bytes.Buffer
	if err := msgp.Encode(&buf, &v); err != nil {
		t.Fatal(err)
	}

	var v2 SearchIndex
	if err := msgp.Decode(&buf, &v2); err != nil {
		t.Fatal(err)
	}

	if v.SchemaVersion != v2.SchemaVersion || v.TotalDocs != v2.TotalDocs {
		t.Errorf("SearchIndex roundtrip failed")
	}
}
