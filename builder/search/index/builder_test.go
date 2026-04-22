package index

import (
	"slices"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
)


func TestBuildEmpty(t *testing.T) {
	idx := Build(nil)
	if idx == nil {
		t.Fatal("Build(nil) returned nil")
	}
	if idx.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", idx.TotalItems)
	}
	if idx.SchemaVersion != searchpkg.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", idx.SchemaVersion, searchpkg.CurrentSchemaVersion)
	}
}

func TestBuildSingleItem(t *testing.T) {
	items := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			Record:  searchpkg.ContentRecord{Title: "Go Tutorial", Link: "/go"},
			DocLen:  3,
			PositionalIndex: map[string][]uint32{
				"go":       {0},
				"tutorial": {1},
				"learn":    {2},
			},
		},
	}
	idx := Build(items)

	if idx.TotalItems != 1 {
		t.Errorf("TotalItems = %d, want 1", idx.TotalItems)
	}
	if len(idx.Terms) == 0 {
		t.Error("Lexicon is empty")
	}
	if idx.LookupTerm("go") < 0 {
		t.Error("Term 'go' not found in lexicon")
	}
	if idx.AvgDocLen != 3 {
		t.Errorf("AvgDocLen = %f, want 3", idx.AvgDocLen)
	}
}

func TestBuildMultipleItems(t *testing.T) {
	items := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			Record:  searchpkg.ContentRecord{Title: "Go 1"},
			DocLen:  2,
			PositionalIndex: map[string][]uint32{"go": {0}, "one": {1}},
		},
		{
			DenseID: 1,
			Record:  searchpkg.ContentRecord{Title: "Go 2"},
			DocLen:  2,
			PositionalIndex: map[string][]uint32{"go": {0}, "two": {1}},
		},
	}
	idx := Build(items)

	if idx.TotalItems != 2 {
		t.Errorf("TotalItems = %d, want 2", idx.TotalItems)
	}

	termIdx := idx.LookupTerm("go")
	if termIdx < 0 {
		t.Fatal("Term 'go' not found")
	}

	docIDs, _ := idx.GetPostings(termIdx)
	if len(docIDs) != 2 {
		t.Errorf("Term 'go' postings len = %d, want 2", len(docIDs))
	}
}

func TestBuildStemMapMerging(t *testing.T) {
	items := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			StemMap: map[string]string{"running": "run"},
		},
		{
			DenseID: 1,
			StemMap: map[string]string{"runs": "run"},
		},
	}
	idx := Build(items)

	origins, ok := idx.StemMap["run"]
	if !ok {
		t.Fatal("Stem 'run' not found")
	}
	if len(origins) != 2 {
		t.Errorf("Stem 'run' has %d origins, want 2", len(origins))
	}
	if !slices.Contains(origins, "running") || !slices.Contains(origins, "runs") {
		t.Error("Missing origins in stem map")
	}
}
