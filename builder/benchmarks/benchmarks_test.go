// Package benchmarks provides comprehensive performance tests for the SSG.
package benchmarks

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

func BenchmarkSearch(b *testing.B) {
	sizes := []int{10, 50, 100, 500}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("IndexSize-%d", size), func(b *testing.B) {
			idx := createMockSearchIndex(size)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = search.PerformSearch(idx, "test query")
			}
		})
	}
}

func BenchmarkSearchWithTagFilter(b *testing.B) {
	idx := createMockSearchIndex(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = search.PerformSearch(idx, "tag:go test query")
	}
}

func BenchmarkPhraseSearch(b *testing.B) {
	idx := createMockSearchIndex(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = search.PerformSearch(idx, `"programming optimization"`)
	}
}

func BenchmarkGetFrontmatterHash(b *testing.B) {
	metaData := map[string]any{
		"title":       "Test",
		"description": "Desc",
		"date":        "2026-02-08",
		"tags":        []string{"go", "ssg"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = hashing.GetFrontmatterHash(metaData, nil)
	}
}

func BenchmarkTokenize(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog."
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = core.TokenizeWithUnicodeInto(text, nil)
	}
}

func BenchmarkExtractSnippet(b *testing.B) {
	content := strings.Repeat("This is some text with keywords. ", 10)
	terms := []string{"keywords", "text"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = search.ExtractSnippet(content, terms)
	}
}

func createMockSearchIndex(size int) *searchpkg.SearchIndex {
	var indexedPosts []searchpkg.IndexedContent
	for i := 0; i < size; i++ {
		indexedPosts = append(indexedPosts, searchpkg.IndexedContent{
			DenseID: uint32(i),
			Record: searchpkg.ContentRecord{
				Title:       fmt.Sprintf("Item %d", i),
				Description: fmt.Sprintf("Desc %d", i),
				Link:        fmt.Sprintf("/item-%d", i),
				Taxonomies:  map[string][]string{"tags": {"go", "ssg"}},
			},
			PositionalIndex: map[string][]uint32{
				"test":         {uint32(i)},
				"query":        {uint32(i + 1)},
				"programming":  {uint32(i + 2)},
				"optimization": {uint32(i + 3)},
			},
			DocLen: 100,
		})
	}
	return index.Build(indexedPosts)
}

func createMockItems(count int) []models.ContentMetadata {
	items := make([]models.ContentMetadata, count)
	for i := range count {
		items[i] = models.ContentMetadata{
			Title:    fmt.Sprintf("Item %d", i),
			DateObj:  time.Now(),
			IsPinned: i%5 == 0,
		}
	}
	return items
}

func BenchmarkSortItems(b *testing.B) {
	items := createMockItems(100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		itemsCopy := make([]models.ContentMetadata, len(items))
		copy(itemsCopy, items)
		timeutil.SortItems(itemsCopy)
	}
}
