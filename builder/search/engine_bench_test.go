package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

func BenchmarkExtractSnippet(b *testing.B) {
	content := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100)
	terms := []string{"fox", "dog"}
	termOffsets := map[string][]int{
		"fox": {16, 19, 60, 63},
		"dog": {40, 43, 84, 87},
	}
	// Simulate what we do for 40 results
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractSnippet(content, terms, termOffsets)
	}
}

func BenchmarkLevenshteinDistance(b *testing.B) {
	s1 := "programming"
	s2 := "programing"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = core.LevenshteinDistance(s1, s2)
	}
}

func BenchmarkPerformSearch(b *testing.B) {
	// Setup a mock index with 100 items
	items := make(map[string]models.ContentRecord)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("%d", i)
		items[id] = models.ContentRecord{
			ID:              uint64(i),
			Title:           fmt.Sprintf("Item %d", i),
			NormalizedTitle: fmt.Sprintf("item %d", i),
			Content:         "This is some content for searching. It contains the word kosh.",
		}
	}
	index := &models.SearchIndex{
		Items:      items,
		Inverted:   map[string]map[string][]uint32{"kosh": {"1": {10}}},
		ItemLens:   make(map[string]int64),
		TotalItems: 100,
		AvgDocLen:  10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PerformSearch(index, "kosh")
	}
}
