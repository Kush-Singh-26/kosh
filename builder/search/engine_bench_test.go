package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
)


func BenchmarkExtractSnippet(b *testing.B) {
	content := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10)
	terms := []string{"fox", "dog"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractSnippet(content, terms)
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
	var indexedPosts []searchpkg.IndexedContent
	for i := 0; i < 100; i++ {
		indexedPosts = append(indexedPosts, searchpkg.IndexedContent{
			DenseID: uint32(i),
			Record: searchpkg.ContentRecord{
				Title: fmt.Sprintf("Item %d", i),
				Content: "This is some content kosh.",
			},
			PositionalIndex: map[string][]uint32{"kosh": {4}},
		})
	}
	idx := index.Build(indexedPosts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PerformSearch(idx, "kosh")
	}
}
