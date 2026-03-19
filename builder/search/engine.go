package search

import (
	"container/heap"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
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
	Version     string
	Score       float64
}

// PerformSearch executes a search query against the index with fuzzy, prefix and phrase support
func PerformSearch(index *models.SearchIndex, query string, versionFilter string) []Result {
	query = NormalizeNFC(query)
	query = ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	originalQuery := query
	tagFilter, query := extractTagFilter(query)
	parsed := ParseQuery(query)

	opts := SearchScoringOptions{
		TagFilter:      tagFilter,
		VersionFilter:  versionFilter,
		QueryTerms:     parsed.Terms,
		Scores:         make(map[string]float64, 100),
		HighlightTerms: make(map[string]bool),
		K1:             1.2,
		B:              0.75,
	}

	for _, t := range opts.QueryTerms {
		opts.HighlightTerms[t] = true
	}

	scoreTagOnly(index, &opts)
	scoreBM25(index, &opts)
	applyPhraseBoosts(index, parsed.Phrases, &opts)
	applyFallbackScoring(index, originalQuery, &opts)
	applyTitleAndTagBoost(index, originalQuery, &opts)

	results := finalizeResults(index, &opts)
	return results
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

func scoreTagOnly(index *models.SearchIndex, opts *SearchScoringOptions) {
	if opts.TagFilter != "" && len(opts.QueryTerms) == 0 {
		opts.HighlightTerms[opts.TagFilter] = true
		for id, post := range index.Posts {
			if opts.VersionFilter != "all" && post.Version != opts.VersionFilter {
				continue
			}
			if slices.Contains(post.NormalizedTags, opts.TagFilter) {
				opts.Scores[id] += ScoreTagMatch
			}
		}
	}
}

func scoreBM25(index *models.SearchIndex, opts *SearchScoringOptions) {
	for _, term := range opts.QueryTerms {
		if posts, ok := index.Inverted[term]; ok {
			opts.Modifier = 1.0
			applyBM25Score(index, posts, term, opts)
		} else {
			scoreFuzzy(index, term, opts)
		}
	}
}

func applyBM25Score(index *models.SearchIndex, posts map[string][]int, term string, opts *SearchScoringOptions) {
	df := len(posts)
	idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))
	avgDocLen := index.AvgDocLen

	for postID, positions := range posts {
		post, ok := index.Posts[postID]
		if !ok || (opts.VersionFilter != "all" && post.Version != opts.VersionFilter) || (opts.TagFilter != "" && !slices.Contains(post.NormalizedTags, opts.TagFilter)) {
			continue
		}

		freq := len(positions)
		docLen := float64(index.DocLens[postID])
		score := idf * (float64(freq) * (opts.K1 + 1)) / (float64(freq) + opts.K1*(1-opts.B+opts.B*(docLen/avgDocLen)))
		opts.Scores[postID] += score * opts.Modifier
	}
}

func scoreFuzzy(index *models.SearchIndex, term string, opts *SearchScoringOptions) {
	var candidates []string
	if index.NgramIndex != nil {
		candidates = FuzzyExpandWithNgrams(term, index.NgramIndex, MaxEditDistance)
	} else {
		candidates = FuzzyExpand(term, index.Inverted, MaxEditDistance)
	}

	for _, candTerm := range candidates {
		opts.HighlightTerms[candTerm] = true
		if posts, ok := index.Inverted[candTerm]; ok {
			opts.Modifier = ScoreFuzzyModifier
			if strings.HasPrefix(candTerm, term) {
				opts.Modifier = 0.9
			}
			applyBM25Score(index, posts, candTerm, opts)
		}
	}
}

func applyPhraseBoosts(index *models.SearchIndex, phrases [][]string, opts *SearchScoringOptions) {
	if len(opts.QueryTerms) > 1 {
		for id := range opts.Scores {
			if checkPhraseUnified(index, id, opts.QueryTerms) {
				opts.Scores[id] += ScorePhraseMatch * 1.2
			}
		}
	}

	for _, phraseTerms := range phrases {
		for id, post := range index.Posts {
			if (opts.VersionFilter != "all" && post.Version != opts.VersionFilter) || !checkPhraseUnified(index, id, phraseTerms) {
				continue
			}
			opts.Scores[id] += ScorePhraseMatch
			for _, pt := range phraseTerms {
				opts.HighlightTerms[pt] = true
			}
		}
	}
}

func applyFallbackScoring(index *models.SearchIndex, originalQuery string, opts *SearchScoringOptions) {
	if len(opts.Scores) > 0 || originalQuery == "" {
		return
	}

	for id, post := range index.Posts {
		if opts.VersionFilter != "all" && post.Version != opts.VersionFilter {
			continue
		}

		match := false
		if strings.Contains(post.NormalizedTitle, originalQuery) {
			opts.Scores[id] += ScoreTitleMatch
			match = true
		}
		if strings.Contains(ToLower(post.Description), originalQuery) {
			opts.Scores[id] += 1.0
			match = true
		}

		if match {
			for word := range strings.FieldsSeq(originalQuery) {
				if len(word) > 2 {
					opts.HighlightTerms[word] = true
				}
			}
		}
	}
}

func applyTitleAndTagBoost(index *models.SearchIndex, originalQuery string, opts *SearchScoringOptions) {
	for id := range opts.Scores {
		post, ok := index.Posts[id]
		if !ok {
			continue
		}

		if originalQuery != "" && strings.Contains(post.NormalizedTitle, originalQuery) {
			opts.Scores[id] += ScoreTitleMatch
		}

		for _, tag := range post.NormalizedTags {
			if tag == originalQuery || tag == opts.TagFilter {
				opts.Scores[id] += ScoreTagMatch
			}
		}
	}
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
		if opts.VersionFilter == "all" && post.Version != "" {
			title = "[" + post.Version + "] " + title
		}

		idNum, _ := strconv.ParseUint(id, 10, 64)
		results = append(results, Result{
			ID: idNum, Title: title, Link: post.Link,
			Description: post.Description, Version: post.Version, Score: score,
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
					termOffsets[term] = offsets
				}
			}
		}
		results[i].Snippet = ExtractSnippet(post.Content, finalHighlightTerms, termOffsets)
	}

	return results
}

func checkPhraseUnified(index *models.SearchIndex, postID string, phraseTerms []string) bool {
	if len(phraseTerms) == 0 {
		return false
	}

	if len(phraseTerms) == 1 {
		if postMap, ok := index.Inverted[phraseTerms[0]]; ok {
			_, found := postMap[postID]
			return found
		}
		return false
	}

	postMap, ok := index.Inverted[phraseTerms[0]]
	if !ok {
		return false
	}
	candidates, ok := postMap[postID]
	if !ok {
		return false
	}

	for i := 1; i < len(phraseTerms); i++ {
		nextWord := phraseTerms[i]
		nextPostMap, ok := index.Inverted[nextWord]
		if !ok {
			return false
		}
		nextPositions, ok := nextPostMap[postID]
		if !ok {
			return false
		}

		var newCandidates []int
		p1, p2 := 0, 0
		for p1 < len(candidates) && p2 < len(nextPositions) {
			if nextPositions[p2] == candidates[p1]+1 {
				newCandidates = append(newCandidates, nextPositions[p2])
				p1++
				p2++
			} else if nextPositions[p2] < candidates[p1]+1 {
				p2++
			} else {
				p1++
			}
		}

		if len(newCandidates) == 0 {
			return false
		}
		candidates = newCandidates
	}

	return true
}

// ExtractSnippet extracts a search snippet from content, highlighting matching terms.
// This is a refactored version using helper functions for better maintainability.
func ExtractSnippet(content string, terms []string, termOffsets map[string][]int) string {
	if len(content) == 0 {
		return ""
	}

	// Truncate content if too long
	content = truncateContent(content)

	// Handle case with no search terms
	if len(terms) == 0 {
		return buildSimpleSnippet(content)
	}

	// Find all term matches
	matches := findMatches(content, terms, termOffsets)

	// If no matches found, return simple snippet
	if len(matches) == 0 {
		return buildSimpleSnippet(content)
	}

	// Sort matches by position
	slices.SortFunc(matches, func(a, b snippetMatch) int {
		return a.pos - b.pos
	})

	// Create term to index mapping for scoring
	termToIndex := make(map[string]int, len(terms))
	for i, t := range terms {
		termToIndex[t] = i
	}

	// Find the best window position for the snippet
	start, end := findBestSnippetWindow(matches, content, termToIndex)

	// Build the final snippet with highlighted matches
	return buildSnippetText(content, matches, start, end, true)
}

func escapeToBuilder(sb *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '&':
			sb.WriteString("&amp;")
		case '\'':
			sb.WriteString("&#39;")
		case '"':
			sb.WriteString("&#34;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteByte(c)
		}
	}
}

// resultHeap implements heap.Interface for a min-heap of search results
type resultHeap struct {
	results []Result
}

func (h resultHeap) Len() int           { return len(h.results) }
func (h resultHeap) Less(i, j int) bool { return h.results[i].Score < h.results[j].Score }
func (h resultHeap) Swap(i, j int)      { h.results[i], h.results[j] = h.results[j], h.results[i] }

func (h *resultHeap) Push(x any) {
	h.results = append(h.results, x.(Result))
}

func (h *resultHeap) Pop() any {
	old := h.results
	n := len(old)
	x := old[n-1]
	h.results = old[0 : n-1]
	return x
}
