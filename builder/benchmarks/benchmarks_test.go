// Package benchmarks provides comprehensive performance tests for the SSG.
// Run with: go test -bench=. -benchmem ./builder/benchmarks/
package benchmarks

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

// BenchmarkSearch performs search with various index sizes
func BenchmarkSearch(b *testing.B) {
	sizes := []int{10, 50, 100, 500}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("IndexSize-%d", size), func(b *testing.B) {
			index := createMockSearchIndex(size)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = search.PerformSearch(index, "test query")
			}
		})
	}
}

// BenchmarkSearchWithTagFilter tests search with tag filtering
func BenchmarkSearchWithTagFilter(b *testing.B) {
	index := createMockSearchIndex(100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.PerformSearch(index, "tag:go test query")
	}
}

// BenchmarkPhraseSearch tests phrase matching performance
func BenchmarkPhraseSearch(b *testing.B) {
	index := createMockSearchIndex(100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.PerformSearch(index, `"programming optimization"`)
	}
}

// BenchmarkGetFrontmatterHash tests hash computation
func BenchmarkGetFrontmatterHash(b *testing.B) {
	metaData := map[string]any{
		"title":       "Test Post Title",
		"description": "This is a test description for benchmarking hash computation performance",
		"date":        "2026-02-08",
		"tags":        []string{"go", "ssg", "performance", "benchmark"},
		"pinned":      true,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = hashing.GetFrontmatterHash(metaData, nil)
	}
}

// BenchmarkSortItems tests item sorting performance
func BenchmarkSortItems(b *testing.B) {
	sizes := []int{10, 50, 100, 500, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			items := createMockItems(size)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create a copy to avoid sorting already sorted slice
				itemsCopy := make([]models.ContentMetadata, len(items))
				copy(itemsCopy, items)
				timeutil.SortItems(itemsCopy)
			}
		})
	}
}

// BenchmarkTokenize tests text tokenization
func BenchmarkTokenize(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog. This is a test of the tokenization performance with various words and numbers like 123 and 456."

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = core.TokenizeWithUnicodeInto(text, nil)
	}
}

// BenchmarkExtractSnippet tests snippet extraction
func BenchmarkExtractSnippet(b *testing.B) {
	content := strings.Repeat("This is some text with keywords. ", 100)
	terms := []string{"keywords", "text"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = search.ExtractSnippet(content, terms, nil)
	}
}

func BenchmarkExtractSnippet_WithOffsets(b *testing.B) {
	content := strings.Repeat("This is some text with keywords. ", 100)
	terms := []string{"keywords", "text"}
	offsets := map[string][]int{
		"keywords": {23, 31},
		"text":     {13, 17},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = search.ExtractSnippet(content, terms, offsets)
	}
}

// BenchmarkStemCached tests cached stemming performance
func BenchmarkStemCached(b *testing.B) {
	words := []string{
		"running", "jumped", "quickly", "beautifully", "programming",
		"transformer", "optimization", "performance", "architecture", "implementation",
		"documentation", "configuration", "initialization", "synchronization", "communication",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, word := range words {
			_ = core.StemCached(word)
		}
	}
}

// BenchmarkStemUncached tests uncached stemming for comparison
func BenchmarkStemUncached(b *testing.B) {
	words := []string{
		"running", "jumped", "quickly", "beautifully", "programming",
		"transformer", "optimization", "performance", "architecture", "implementation",
		"documentation", "configuration", "initialization", "synchronization", "communication",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, word := range words {
			_ = core.Stem(word)
		}
	}
}

// BenchmarkStemCachedRepeated tests caching benefit on repeated words
func BenchmarkStemCachedRepeated(b *testing.B) {
	// Same word repeated many times (common in real content)
	word := "programming"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for range 100 {
			_ = core.StemCached(word)
		}
	}
}

// BenchmarkStemUncachedRepeated tests uncached repeated stems for comparison
func BenchmarkStemUncachedRepeated(b *testing.B) {
	word := "programming"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for range 100 {
			_ = core.Stem(word)
		}
	}
}

// BenchmarkFuzzyExpand tests fuzzy matching with full scan
func BenchmarkFuzzyExpand(b *testing.B) {
	index := createMockSearchIndex(500)
	term := "progamming" // typo

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = core.FuzzyExpand(term, index.Inverted, 2)
	}
}

// BenchmarkFuzzyExpandWithNgrams tests fuzzy matching with ngram index
func BenchmarkFuzzyExpandWithNgrams(b *testing.B) {
	index := createMockSearchIndexWithNgrams(500)
	term := "progamming" // typo

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = core.FuzzyExpandWithNgrams(term, index.NgramIndex, 2)
	}
}

// BenchmarkBuildNgramIndex tests ngram index construction
func BenchmarkBuildNgramIndex(b *testing.B) {
	index := createMockSearchIndex(500)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = core.BuildNgramIndex(index.Inverted)
	}
}

// BenchmarkAnalyze tests text analysis with stemming
func BenchmarkAnalyze(b *testing.B) {
	analyzer := core.NewAnalyzer(true, true)
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = analyzer.Analyze(text)
	}
}

// BenchmarkAnalyzeNoStemming tests text analysis without stemming
func BenchmarkAnalyzeNoStemming(b *testing.B) {
	analyzer := core.NewAnalyzer(true, false)
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = analyzer.Analyze(text)
	}
}

// BenchmarkLevenshteinDistance tests edit distance calculation
func BenchmarkLevenshteinDistance(b *testing.B) {
	tests := []struct {
		a, b string
	}{
		{"programming", "progamming"},          // 1 edit
		{"optimization", "optimisation"},       // 2 edits
		{"implementation", "imlementation"},    // 1 edit
		{"synchronization", "synchronisation"}, // 2 edits
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, tt := range tests {
			_ = core.LevenshteinDistance(tt.a, tt.b)
		}
	}
}

// Helper functions

func createMockSearchIndex(size int) *models.SearchIndex {
	index := &models.SearchIndex{
		Items:      make(map[string]models.ContentRecord, size),
		Inverted:   make(map[string]map[string][]uint32),
		ItemLens:   make(map[string]int64),
		TotalItems: int64(size),
	}

	totalLen := 0
	for i := range size {
		idStr := strconv.Itoa(i)
		index.Items[idStr] = models.ContentRecord{
			ID:             uint64(i),
			Title:          fmt.Sprintf("Item %d", i),
			Link:           fmt.Sprintf("/items/item-%d", i),
			Description:    fmt.Sprintf("Description for item %d", i),
			Taxonomies:     map[string][]string{"tags": {"go", "ssg", "web"}},
			NormalizedTaxs: map[string][]string{"tags": {"go", "ssg", "web"}},
		}

		// Add some inverted index entries
		words := []string{"test", "content", "item", "go", "ssg", "programming", "optimization", "performance"}
		for j, word := range words {
			if _, ok := index.Inverted[word]; !ok {
				index.Inverted[word] = make(map[string][]uint32)
			}
			index.Inverted[word][idStr] = []uint32{uint32(j)}
		}

		index.ItemLens[idStr] = int64(100 + i)
		totalLen += int(index.ItemLens[idStr])
	}

	if size > 0 {
		index.AvgDocLen = float64(totalLen) / float64(size)
	}

	return index
}

func createMockSearchIndexWithNgrams(size int) *models.SearchIndex {
	index := createMockSearchIndex(size)
	index.NgramIndex = core.BuildNgramIndex(index.Inverted)
	return index
}

func createMockItems(count int) []models.ContentMetadata {
	items := make([]models.ContentMetadata, count)
	for i := range count {
		items[i] = models.ContentMetadata{
			Title:    fmt.Sprintf("Item %d", i),
			DateObj:  time.Now().Add(-time.Duration(i) * time.Hour),
			IsPinned: i%5 == 0,
		}
	}
	return items
}
