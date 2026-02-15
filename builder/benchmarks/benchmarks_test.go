// Package benchmarks provides comprehensive performance tests for the SSG.
// Run with: go test -bench=. -benchmem ./builder/benchmarks/
package benchmarks

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// BenchmarkSearch performs search with various index sizes
func BenchmarkSearch(b *testing.B) {
	sizes := []int{10, 50, 100, 500}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("IndexSize-%d", size), func(b *testing.B) {
			index := createMockSearchIndex(size)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = search.PerformSearch(index, "test query", "")
			}
		})
	}
}

// BenchmarkSearchWithTagFilter tests search with tag filtering
func BenchmarkSearchWithTagFilter(b *testing.B) {
	index := createMockSearchIndex(100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.PerformSearch(index, "tag:go test query", "")
	}
}

// BenchmarkGetFrontmatterHash tests hash computation
func BenchmarkGetFrontmatterHash(b *testing.B) {
	metaData := map[string]interface{}{
		"title":       "Test Post Title",
		"description": "This is a test description for benchmarking hash computation performance",
		"date":        "2026-02-08",
		"tags":        []string{"go", "ssg", "performance", "benchmark"},
		"pinned":      true,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = utils.GetFrontmatterHash(metaData)
	}
}

// BenchmarkSortPosts tests post sorting performance
func BenchmarkSortPosts(b *testing.B) {
	sizes := []int{10, 50, 100, 500, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			posts := createMockPosts(size)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create a copy to avoid sorting already sorted slice
				postsCopy := make([]models.PostMetadata, len(posts))
				copy(postsCopy, posts)
				utils.SortPosts(postsCopy)
			}
		})
	}
}

// BenchmarkTokenize tests text tokenization
func BenchmarkTokenize(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog. This is a test of the tokenization performance with various words and numbers like 123 and 456."

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.Tokenize(text)
	}
}

// BenchmarkExtractSnippet tests snippet extraction
func BenchmarkExtractSnippet(b *testing.B) {
	content := `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`
	terms := []string{"consequat", "exercitation", "aliqua"}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.ExtractSnippet(content, terms)
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
			_ = search.StemCached(word)
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
			_ = search.Stem(word)
		}
	}
}

// BenchmarkStemCachedRepeated tests caching benefit on repeated words
func BenchmarkStemCachedRepeated(b *testing.B) {
	// Same word repeated many times (common in real content)
	word := "programming"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			_ = search.StemCached(word)
		}
	}
}

// BenchmarkStemUncachedRepeated tests uncached repeated stems for comparison
func BenchmarkStemUncachedRepeated(b *testing.B) {
	word := "programming"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			_ = search.Stem(word)
		}
	}
}

// BenchmarkFuzzyExpand tests fuzzy matching with full scan
func BenchmarkFuzzyExpand(b *testing.B) {
	index := createMockSearchIndex(500)
	term := "progamming" // typo

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.FuzzyExpand(term, index.Inverted, 2)
	}
}

// BenchmarkFuzzyExpandWithNgrams tests fuzzy matching with ngram index
func BenchmarkFuzzyExpandWithNgrams(b *testing.B) {
	index := createMockSearchIndexWithNgrams(500)
	term := "progamming" // typo

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.FuzzyExpandWithNgrams(term, index.NgramIndex, 2)
	}
}

// BenchmarkBuildNgramIndex tests ngram index construction
func BenchmarkBuildNgramIndex(b *testing.B) {
	index := createMockSearchIndex(500)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = search.BuildNgramIndex(index.Inverted)
	}
}

// BenchmarkAnalyze tests text analysis with stemming
func BenchmarkAnalyze(b *testing.B) {
	analyzer := search.NewAnalyzer(true, true)
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = analyzer.Analyze(text)
	}
}

// BenchmarkAnalyzeNoStemming tests text analysis without stemming
func BenchmarkAnalyzeNoStemming(b *testing.B) {
	analyzer := search.NewAnalyzer(true, false)
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
			_ = search.LevenshteinDistance(tt.a, tt.b)
		}
	}
}

// Helper functions

func createMockSearchIndex(size int) *models.SearchIndex {
	index := &models.SearchIndex{
		Posts:     make([]models.PostRecord, size),
		Inverted:  make(map[string]map[int]int),
		DocLens:   make(map[int]int),
		TotalDocs: size,
	}

	totalLen := 0
	for i := 0; i < size; i++ {
		index.Posts[i] = models.PostRecord{
			ID:          i,
			Title:       fmt.Sprintf("Post %d", i),
			Link:        fmt.Sprintf("/posts/post-%d", i),
			Description: fmt.Sprintf("Description for post %d", i),
			Tags:        []string{"go", "ssg", "web"},
			Content:     fmt.Sprintf("Content for post %d with some test words", i),
		}

		// Add some inverted index entries
		words := []string{"test", "content", "post", "go", "ssg", "programming", "optimization", "performance"}
		for j, word := range words {
			if _, ok := index.Inverted[word]; !ok {
				index.Inverted[word] = make(map[int]int)
			}
			index.Inverted[word][i] = j + 1
		}

		index.DocLens[i] = 100 + i
		totalLen += index.DocLens[i]
	}

	if size > 0 {
		index.AvgDocLen = float64(totalLen) / float64(size)
	}

	return index
}

func createMockSearchIndexWithNgrams(size int) *models.SearchIndex {
	index := createMockSearchIndex(size)
	index.NgramIndex = search.BuildNgramIndex(index.Inverted)
	return index
}

func createMockPosts(count int) []models.PostMetadata {
	posts := make([]models.PostMetadata, count)
	for i := 0; i < count; i++ {
		posts[i] = models.PostMetadata{
			Title:   fmt.Sprintf("Post %d", i),
			DateObj: time.Now().Add(-time.Duration(i) * time.Hour),
			Pinned:  i%5 == 0,
		}
	}
	return posts
}
