package search

import (
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
)


func buildRankingIndex() *searchpkg.SearchIndex {
	indexedPosts := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			Record: searchpkg.ContentRecord{
				Title:       "Go Programming Guide",
				Description: "Learn Go programming basics",
				NormalizedTaxs: map[string][]string{"tags": {"go", "programming"}},
				Content:      "Go is a programming language created at Google.",
				Date:         1713686400,
			},
			PositionalIndex: map[string][]uint32{"go": {0}, "program": {1}, "guide": {3}},
			DocLen:          10,
		},
		{
			DenseID: 1,
			Record: searchpkg.ContentRecord{
				Title:       "Rust Programming Tutorial",
				Description: "Learn Rust programming from scratch",
				NormalizedTaxs: map[string][]string{"tags": {"rust", "programming"}},
				Content:      "Rust is a systems programming language.",
				Date:         1713600000,
			},
			PositionalIndex: map[string][]uint32{"rust": {0}, "program": {1}, "tutorial": {2}},
			DocLen:          10,
		},
	}

	idx := index.Build(indexedPosts)
	idx.Ranking = searchpkg.SearchRankingConfig{
		TitleBoost:       50.0,
		TagBoost:         5.0,
		DescriptionBoost: 5.0,
		BM25K1:           1.2,
		BM25B:            0.75,
	}
	return idx
}

func TestRanking_TitleBoost(t *testing.T) {
	idx := buildRankingIndex()
	results := PerformSearch(idx, "go")
	if len(results) < 1 {
		t.Fatalf("Expected results, got 0")
	}
	if !strings.Contains(strings.ToLower(results[0].Title), "go") {
		t.Error("Title boost failed")
	}
}

func TestRanking_TagBoost(t *testing.T) {
	idx := buildRankingIndex()
	results := PerformSearch(idx, "programming")
	if len(results) < 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
}

func TestRanking_PhraseMatch(t *testing.T) {
	indexedPosts := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			Record: searchpkg.ContentRecord{Title: "Neural Network Basics", Content: "A neural network."},
			PositionalIndex: map[string][]uint32{"neural": {0}, "network": {1}},
			DocLen: 10,
		},
		{
			DenseID: 1,
			Record: searchpkg.ContentRecord{Title: "Network Security", Content: "Networks are for security."},
			PositionalIndex: map[string][]uint32{"network": {0}},
			DocLen: 10,
		},
	}
	idx := index.Build(indexedPosts)
	results := PerformSearch(idx, "neural network")
	if len(results) < 1 || results[0].ID != 0 {
		t.Errorf("Phrase match ranking failed, got %v", results)
	}
}

func TestRanking_ScoreOrdering(t *testing.T) {
	idx := buildRankingIndex()
	results := PerformSearch(idx, "programming")
	for i := 0; i < len(results)-1; i++ {
		if results[i].Score < results[i+1].Score {
			t.Errorf("Decending sort order failed at index %d", i)
		}
	}
}
