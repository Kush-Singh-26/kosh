package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
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
		_ = LevenshteinDistance(s1, s2)
	}
}

func BenchmarkPerformSearch(b *testing.B) {
	// Setup a mock index with 100 posts
	posts := make(map[string]models.PostRecord)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("%d", i)
		posts[id] = models.PostRecord{
			ID:              uint64(i),
			Title:           fmt.Sprintf("Post %d", i),
			NormalizedTitle: fmt.Sprintf("post %d", i),
			Content:         "This is some content for searching. It contains the word kosh.",
		}
	}
	index := &models.SearchIndex{
		Posts:     posts,
		Inverted:  map[string]map[string][]int{"kosh": {"1": {10}}},
		DocLens:   make(map[string]int64),
		TotalDocs: 100,
		AvgDocLen: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PerformSearch(index, "kosh", "all")
	}
}
