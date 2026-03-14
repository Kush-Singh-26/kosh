package search

import (
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
	// Apply NFC normalization and Unicode-aware lowercasing
	query = NormalizeNFC(query)
	query = ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	// Capture original query for fallback and title boosting
	originalQuery := query

	// Parse query for tag filter
	tagFilter := ""
	if strings.HasPrefix(query, "tag:") {
		parts := strings.SplitN(query, " ", 2)
		tagFilter = strings.TrimPrefix(parts[0], "tag:")
		if len(parts) > 1 {
			query = parts[1]
		} else {
			query = ""
		}
	}

	// Parse query for phrases and terms
	parsed := ParseQuery(query)
	queryTerms := parsed.Terms

	// Use dynamic capacity based on actual index size
	maxResults := len(index.Posts)
	if maxResults > 400 {
		maxResults = 400
	}
	scores := make(map[string]float64, maxResults/4)

	k1 := 1.2
	b := 0.75

	// Initialize postCache with reasonable capacity
	postCache := make(map[string]models.PostRecord, maxResults/4)

	// Collect terms that actually matched in the index for high-fidelity highlighting
	highlightTerms := make(map[string]bool)
	for _, t := range queryTerms {
		highlightTerms[t] = true
	}

	if tagFilter != "" && len(queryTerms) == 0 {
		highlightTerms[tagFilter] = true
		for id, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}
			if HasTagNormalized(post.NormalizedTags, tagFilter) {
				scores[id] += ScoreTagMatch
			}
		}
	}

	// 1.5 Process individual terms with BM25 (Exact + Fuzzy + Prefix)
	for _, term := range queryTerms {
		if posts, ok := index.Inverted[term]; ok {
			// Exact match
			df := len(posts)
			idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))
			avgDocLen := index.AvgDocLen

			for postID, positions := range posts {
				post, cached := postCache[postID]
				if !cached {
					p, ok := index.Posts[postID]
					if !ok {
						continue
					}
					post = p
					postCache[postID] = post
				}

				if versionFilter != "all" && post.Version != versionFilter {
					continue
				}

				if tagFilter != "" && !HasTagNormalized(post.NormalizedTags, tagFilter) {
					continue
				}

				freq := len(positions)
				docLen := float64(index.DocLens[postID])
				score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/avgDocLen)))
				scores[postID] += score
			}
		} else {
			// Fallback: Try prefix and fuzzy matching if exact term not found
			var candidates []string
			if index.NgramIndex != nil {
				candidates = FuzzyExpandWithNgrams(term, index.NgramIndex, MaxEditDistance)
			} else {
				candidates = FuzzyExpand(term, index.Inverted, MaxEditDistance)
			}

			for _, candTerm := range candidates {
				highlightTerms[candTerm] = true // Match found, highlight it in the snippet

				if posts, ok := index.Inverted[candTerm]; ok {
					df := len(posts)
					idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))
					avgDocLen := index.AvgDocLen

					for postID, positions := range posts {
						post, cached := postCache[postID]
						if !cached {
							p, ok := index.Posts[postID]
							if !ok {
								continue
							}
							post = p
							postCache[postID] = post
						}

						if versionFilter != "all" && post.Version != versionFilter {
							continue
						}

						if tagFilter != "" && !HasTagNormalized(post.NormalizedTags, tagFilter) {
							continue
						}

						freq := len(positions)
						docLen := float64(index.DocLens[postID])
						score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/avgDocLen)))

						// Prefix matches (distance usually high) get less penalty than pure fuzzy
						modifier := ScoreFuzzyModifier
						if strings.HasPrefix(candTerm, term) {
							modifier = 0.9
						}
						scores[postID] += score * modifier
					}
				}
			}
		}
	}

	// 1.6 Implicit Phrase Boost
	if len(queryTerms) > 1 {
		for id := range scores {
			if checkPhraseUnified(index, id, queryTerms) {
				scores[id] += ScorePhraseMatch * 1.2
			}
		}
	}

	// 2. Process phrase matches
	for _, phraseTerms := range parsed.Phrases {
		for id, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}

			if checkPhraseUnified(index, id, phraseTerms) {
				scores[id] += ScorePhraseMatch
				for _, pt := range phraseTerms {
					highlightTerms[pt] = true
				}
			}
		}
	}

	// 3. Fallback for Empty Query Terms
	if len(scores) == 0 && originalQuery != "" {
		for id, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}

			match := false
			if strings.Contains(post.NormalizedTitle, originalQuery) {
				scores[id] += ScoreTitleMatch
				match = true
			}
			if strings.Contains(ToLower(post.Description), originalQuery) {
				scores[id] += 1.0
				match = true
			}

			if match {
				for word := range strings.FieldsSeq(originalQuery) {
					if len(word) > 2 {
						highlightTerms[word] = true
					}
				}
			}
		}
	}

	// 4. Boost title and tag matches
	for id := range scores {
		post, ok := index.Posts[id]
		if !ok {
			continue
		}

		if originalQuery != "" && strings.Contains(post.NormalizedTitle, originalQuery) {
			scores[id] += ScoreTitleMatch
		}

		for _, tag := range post.NormalizedTags {
			if tag == originalQuery || tag == tagFilter {
				scores[id] += ScoreTagMatch
			}
		}
	}

	finalHighlightTerms := make([]string, 0, len(highlightTerms))
	for t := range highlightTerms {
		finalHighlightTerms = append(finalHighlightTerms, t)
	}

	results := make([]Result, 0, len(scores))
	for id, score := range scores {
		post := index.Posts[id]
		title := post.Title
		if versionFilter == "all" && post.Version != "" {
			title = "[" + post.Version + "] " + title
		}

		idNum, _ := strconv.ParseUint(id, 10, 64)
		results = append(results, Result{
			ID:          idNum,
			Title:       title,
			Link:        post.Link,
			Description: post.Description,
			Version:     post.Version,
			Score:       score,
		})
	}

	slices.SortFunc(results, func(a, b Result) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})

	if len(results) > 40 {
		results = results[:40]
	}

	for i := range results {
		idNum := results[i].ID
		id := strconv.FormatUint(idNum, 10)
		post, ok := index.Posts[id]
		if !ok {
			continue
		}

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

func HasTagNormalized(normalizedTags []string, target string) bool {
	return slices.Contains(normalizedTags, target)
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
	// Quick path if no characters need escaping
	if !strings.ContainsAny(s, `'"&<>`) {
		sb.WriteString(s)
		return
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		case '&':
			sb.WriteString("&amp;")
		case '"':
			sb.WriteString("&#34;")
		case '\'':
			sb.WriteString("&#39;")
		default:
			sb.WriteByte(c)
		}
	}
}
