package search

import (
	"container/heap"
	"math"
	"math/bits"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"

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

func ExtractSnippet(content string, terms []string, termOffsets map[string][]int) string {
	if len(content) == 0 {
		return ""
	}
	if len(content) > MaxSnippetContentLength {
		content = content[:MaxSnippetContentLength]
		// Align to rune boundary to avoid invalid UTF-8
		for len(content) > 0 && !utf8.RuneStart(content[len(content)-1]) {
			content = content[:len(content)-1]
		}
		// If the last byte is the start of a multi-byte rune but we don't have the rest,
		// we should also trim it.
		r, sz := utf8.DecodeLastRuneInString(content)
		if r == utf8.RuneError && sz == 1 {
			content = content[:len(content)-1]
		}
	}

	if len(terms) == 0 {
		if len(content) > DefaultSnippetLength {
			b := utils.SharedStringBuilderPool.Get()
			defer utils.SharedStringBuilderPool.Put(b)
			escapeToBuilder(b, content[:DefaultSnippetLength])
			b.WriteString("...")
			return b.String()
		}
		b := utils.SharedStringBuilderPool.Get()
		defer utils.SharedStringBuilderPool.Put(b)
		escapeToBuilder(b, content)
		return b.String()
	}

	type match struct {
		pos  int
		term string
	}
	var matches []match

	if len(termOffsets) > 0 {
		for _, term := range terms {
			if offsets, ok := termOffsets[term]; ok {
				for i := 0; i < len(offsets); i += 2 {
					start := offsets[i]
					if start < len(content) {
						matches = append(matches, match{pos: start, term: term})
					}
				}
			}
		}
	}

	if len(matches) == 0 {
		contentLower := ToLower(content)
		for _, term := range terms {
			if len(term) < 2 {
				continue
			}
			curr := 0
			for {
				idx := strings.Index(contentLower[curr:], term)
				if idx == -1 {
					break
				}
				matches = append(matches, match{pos: curr + idx, term: term})
				curr += idx + len(term)
			}
		}
	}

	if len(matches) == 0 {
		if len(content) > DefaultSnippetLength {
			b := utils.SharedStringBuilderPool.Get()
			defer utils.SharedStringBuilderPool.Put(b)
			escapeToBuilder(b, content[:DefaultSnippetLength])
			b.WriteString("...")
			return b.String()
		}
		b := utils.SharedStringBuilderPool.Get()
		defer utils.SharedStringBuilderPool.Put(b)
		escapeToBuilder(b, content)
		return b.String()
	}

	slices.SortFunc(matches, func(a, b match) int {
		return a.pos - b.pos
	})

	termToIndex := make(map[string]int, len(terms))
	for i, t := range terms {
		termToIndex[t] = i
	}

	bestStart := matches[0].pos
	maxScore := 0
	windowSize := DefaultSnippetLength

	for i := 0; i < len(matches); i++ {
		count := 0
		var mask uint64
		for j := i; j < len(matches) && matches[j].pos < matches[i].pos+windowSize; j++ {
			count++
			if idx, ok := termToIndex[matches[j].term]; ok {
				if idx < 64 {
					mask |= (1 << uint(idx))
				}
			}
		}

		score := bits.OnesCount64(mask)*100 + count
		if score > maxScore {
			maxScore = score
			bestStart = matches[i].pos
		}
	}

	start := max(bestStart-SnippetContextBefore, 0)
	// Align to rune boundary to avoid panic
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}

	end := start + windowSize + SnippetContextBefore
	if end > len(content) {
		end = len(content)
		start = max(end-(windowSize+SnippetContextBefore), 0)
		for start > 0 && !utf8.RuneStart(content[start]) {
			start--
		}
	} else {
		// Align end to rune boundary
		for end < len(content) && !utf8.RuneStart(content[end]) {
			end++
		}
	}

	if start > 0 {
		idx := strings.Index(content[start:], " ")
		if idx != -1 && idx < 15 {
			start += idx + 1
		}
	}

	b := utils.SharedStringBuilderPool.Get()
	b.Grow(int(float64(end-start) * 1.2))

	if start > 0 {
		b.WriteString("...")
	}

	lastPos := start
	for _, m := range matches {
		if m.pos < start {
			continue
		}
		if m.pos >= end {
			break
		}
		if m.pos < lastPos {
			continue
		}

		escapeToBuilder(b, content[lastPos:m.pos])

		// Find word end (including trailing \w*)
		actualEnd := m.pos + len(m.term)
		// Match \w* behavior (unicode letter or number)
		for actualEnd < end {
			r, sz := utf8.DecodeRuneInString(content[actualEnd:])
			if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' {
				break
			}
			actualEnd += sz
		}

		b.WriteString("<b>")
		escapeToBuilder(b, content[m.pos:actualEnd])
		b.WriteString("</b>")
		lastPos = actualEnd
	}

	escapeToBuilder(b, content[lastPos:end])

	if end < len(content) {
		b.WriteString("...")
	}

	res := b.String()
	utils.SharedStringBuilderPool.Put(b)
	return res
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
