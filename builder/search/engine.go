package search

import (
	"container/heap"
	"slices"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

// Constants for snippet extraction optimization.
const (
	// MaxSnippetContentLength caps content scanned for snippets.
	MaxSnippetContentLength = 2048
	// DefaultSnippetLength is the default snippet size.
	DefaultSnippetLength = 150
	// SnippetContextBefore is the prefix context size for snippets.
	SnippetContextBefore = 60
	// SnippetContextAfter is the suffix context size for snippets.
	SnippetContextAfter = 90
)

const (
	defaultScoreMapCap = 100
	defaultTopKResults = 40
)

// Result represents a search result with scoring metadata.
type Result struct {
	ID          uint32
	Title       string
	Link        string
	Description string
	Taxonomies  map[string][]string
	Snippet     string
	Score       float64
}

// PerformSearch executes a search query against the index using a structured pipeline
func PerformSearch(index *searchpkg.SearchIndex, query string) []Result {
	query = core.NormalizeNFC(query)
	query = core.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	originalQuery := query
	tagFilter, query := extractTagFilter(query)
	parsed := core.ParseQuery(query)

	ctx := &Context{
		Index:         index,
		QueryTerms:    parsed.Terms,
		Phrases:       parsed.Phrases,
		TagFilter:     tagFilter,
		OriginalQuery: originalQuery,
		TermInfos:     parsed.TermInfos,
	}

	opts := &ScoringOptions{
		TagFilter:      tagFilter,
		QueryTerms:     parsed.Terms,
		Scores:         make(map[uint32]float64, defaultScoreMapCap),
		HighlightTerms: make(map[string]bool),
		TermInfos:      parsed.TermInfos,
		K1:             index.Ranking.BM25K1,
		B:              index.Ranking.BM25B,
		Ranking:        index.Ranking,
	}

	for _, term := range opts.QueryTerms {
		opts.HighlightTerms[term] = true
	}

	pipeline := NewPipeline(
		&TitleScorer{},
		&TagScorer{},
		&BM25Scorer{},
		&PhraseScorer{},
		&ProximityScorer{},
		&RecencyScorer{},
		&BoostScorer{},
		&FilterScorer{},
	)
	pipeline.Execute(ctx, opts)

	return finalizeResults(index, opts)
}

func extractTagFilter(query string) (string, string) {
	if strings.HasPrefix(query, "tag:") {
		parts := strings.SplitN(query, " ", 2)
		tag := strings.TrimPrefix(parts[0], "tag:")
		if len(parts) > 1 {
			return tag, parts[1]
		}
		return tag, ""
	}
	return "", query
}

func finalizeResults(index *searchpkg.SearchIndex, opts *ScoringOptions) []Result {
	finalHighlightTerms := make([]string, 0, len(opts.HighlightTerms))
	for term := range opts.HighlightTerms {
		finalHighlightTerms = append(finalHighlightTerms, term)
	}

	results := convertResults(index, opts.Scores)
	results = topKResults(results, defaultTopKResults)
	enrichWithSnippets(index, results, finalHighlightTerms)

	return results
}

func convertResults(index *searchpkg.SearchIndex, scores map[uint32]float64) []Result {
	results := make([]Result, 0, len(scores))
	for id, score := range scores {
		item := index.Items[id]
		results = append(results, Result{
			ID: id, Title: item.Title, Link: item.Link,
			Description: item.Description, Taxonomies: item.Taxonomies,
			Score: score,
		})
	}
	return results
}

func topKResults(results []Result, k int) []Result {
	if len(results) > k {
		resHeap := &resultHeap{results: results[:k]}
		heap.Init(resHeap)
		for i := k; i < len(results); i++ {
			if results[i].Score > resHeap.results[0].Score {
				heap.Pop(resHeap)
				heap.Push(resHeap, results[i])
			}
		}
		results = resHeap.results
	}
	sortResults(results)
	return results[:min(len(results), k)]
}

func sortResults(results []Result) {
	slices.SortFunc(results, func(a, b Result) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})
}

func enrichWithSnippets(index *searchpkg.SearchIndex, results []Result, highlightTerms []string) {
	for i := range results {
		item := index.Items[results[i].ID]
		// JIT scanning: JIT snippet extraction from item.Content
		results[i].Snippet = ExtractSnippet(item.Content, highlightTerms)
		// Title highlighting
		results[i].Title = HighlightText(item.Title, highlightTerms)
	}
}

// resultHeap implements heap.Interface for a min-heap of search results
type resultHeap struct {
	results []Result
}

// Len implements heap.Interface.
func (heapObj resultHeap) Len() int { return len(heapObj.results) }

// Less implements heap.Interface for a min-heap by score.
func (heapObj resultHeap) Less(i, j int) bool {
	return heapObj.results[i].Score < heapObj.results[j].Score
}

// Swap implements heap.Interface.
func (heapObj resultHeap) Swap(i, j int) {
	heapObj.results[i], heapObj.results[j] = heapObj.results[j], heapObj.results[i]
}

// Push implements heap.Interface.
func (heapObj *resultHeap) Push(x any) {
	if res, ok := x.(Result); ok {
		heapObj.results = append(heapObj.results, res)
	}
}

// Pop implements heap.Interface.
func (heapObj *resultHeap) Pop() any {
	old := heapObj.results
	n := len(old)
	res := old[n-1]
	heapObj.results = old[0 : n-1]
	return res
}
