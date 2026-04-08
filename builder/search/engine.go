package search

import (
	"container/heap"
	"slices"
	"strconv"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

// Constants for snippet extraction optimization
const (
	MaxSnippetContentLength = 10000
	DefaultSnippetLength    = 150
	SnippetContextBefore    = 60
	SnippetContextAfter     = 90
)

// Scoring weights for different match types
const (
	ScorePhraseMatch   = 15.0
	ScoreTitleMatch    = 10.0
	ScoreTagMatch      = 5.0
	ScoreFuzzyModifier = 0.7
)

type Result struct {
	ID          uint64
	Title       string
	Link        string
	Description string
	Snippet     string
	Score       float64
}

// PerformSearch executes a search query against the index using a structured pipeline
func PerformSearch(index *models.SearchIndex, query string) []Result {
	query = core.NormalizeNFC(query)
	query = core.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	originalQuery := query
	tagFilter, query := extractTagFilter(query)
	parsed := core.ParseQuery(query)

	ctx := &SearchContext{
		Index:         index,
		QueryTerms:    parsed.Terms,
		Phrases:       parsed.Phrases,
		TagFilter:     tagFilter,
		OriginalQuery: originalQuery,
		TermInfos:     parsed.TermInfos,
	}

	opts := &SearchScoringOptions{
		TagFilter:      tagFilter,
		QueryTerms:     parsed.Terms,
		Scores:         make(map[string]float64, 100),
		HighlightTerms: make(map[string]bool),
		TermInfos:      parsed.TermInfos,
		K1:             1.2,
		B:              0.75,
	}

	for _, t := range opts.QueryTerms {
		opts.HighlightTerms[t] = true
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

func finalizeResults(index *models.SearchIndex, opts *SearchScoringOptions) []Result {
	finalHighlightTerms := make([]string, 0, len(opts.HighlightTerms))
	for t := range opts.HighlightTerms {
		finalHighlightTerms = append(finalHighlightTerms, t)
	}

	results := make([]Result, 0, len(opts.Scores))
	for id, score := range opts.Scores {
		post := index.Posts[id]
		title := post.Title

		idNum, _ := strconv.ParseUint(id, 10, 64)
		results = append(results, Result{
			ID: idNum, Title: title, Link: post.Link,
			Description: post.Description, Score: score,
		})
	}

	const topK = 40
	if len(results) > topK {
		h := &resultHeap{results: results[:topK]}
		heap.Init(h)
		for i := topK; i < len(results); i++ {
			if results[i].Score > h.results[0].Score {
				heap.Pop(h)
				heap.Push(h, results[i])
			}
		}
		slices.SortFunc(h.results, func(a, b Result) int {
			if a.Score > b.Score {
				return -1
			}
			if a.Score < b.Score {
				return 1
			}
			return 0
		})
		results = h.results
	} else {
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

	results = results[:min(len(results), topK)]

	for i := range results {
		id := strconv.FormatUint(results[i].ID, 10)
		post := index.Posts[id]
		termOffsets := make(map[string][]int)
		for _, term := range finalHighlightTerms {
			if docMap, ok := index.Offsets[term]; ok {
				if offsets, found := docMap[id]; found {
					termOffsets[term] = models.DecodeOffsets(offsets)
				}
			}
		}
		results[i].Snippet = ExtractSnippet(post.Content, finalHighlightTerms, termOffsets)
	}

	return results
}

// resultHeap implements heap.Interface for a min-heap of search results
type resultHeap struct {
	results []Result
}

func (h resultHeap) Len() int           { return len(h.results) }
func (h resultHeap) Less(i, j int) bool { return h.results[i].Score < h.results[j].Score }
func (h resultHeap) Swap(i, j int)      { h.results[i], h.results[j] = h.results[j], h.results[i] }

func (h *resultHeap) Push(x any) {
	if r, ok := x.(Result); ok {
		h.results = append(h.results, r)
	}
}

func (h *resultHeap) Pop() any {
	old := h.results
	n := len(old)
	x := old[n-1]
	h.results = old[0 : n-1]
	return x
}
