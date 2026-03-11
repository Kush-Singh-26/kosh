package search

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// replacerCache caches regex patterns for snippet highlighting (thread-safe LRU)
var replacerCache *lru.Cache[string, *regexp.Regexp]

func init() {
	var err error
	replacerCache, err = lru.New[string, *regexp.Regexp](500)
	if err != nil {
		panic(err) // Should never happen with valid size
	}
}

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
	ID          int
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

	maxResults := min(len(index.Posts), 100)
	scores := make(map[int]float64, maxResults)

	k1 := 1.2
	b := 0.75

	postCache := make(map[int]*models.PostRecord, maxResults)

	// Collect terms that actually matched in the index for high-fidelity highlighting
	highlightTerms := make(map[string]bool)
	for _, t := range queryTerms {
		highlightTerms[t] = true
	}

	if tagFilter != "" && len(queryTerms) == 0 {
		highlightTerms[tagFilter] = true
		for i, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}
			if HasTagNormalized(post.NormalizedTags, tagFilter) {
				scores[i] += ScoreTagMatch
			}
		}
	}

	// 1.5 Process individual terms with BM25 (Exact + Fuzzy + Prefix)
	for _, term := range queryTerms {
		if posts, ok := index.Inverted[term]; ok {
			// Exact match
			df := len(posts)
			idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))

			for postIDStr, positions := range posts {
				// Search index posts are array indexed, but keys in DocLens/Inverted are strings for msgp stability
				// Convert once per doc match
				postID := 0
				for j := 0; j < len(postIDStr); j++ {
					postID = postID*10 + int(postIDStr[j]-'0')
				}

				post, cached := postCache[postID]
				if !cached {
					post = &index.Posts[postID]
					postCache[postID] = post
				}

				if versionFilter != "all" && post.Version != versionFilter {
					continue
				}

				if tagFilter != "" && !HasTagNormalized(post.NormalizedTags, tagFilter) {
					continue
				}

				freq := len(positions)
				docLen := float64(index.DocLens[postIDStr])
				score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/index.AvgDocLen)))
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

					for postIDStr, positions := range posts {
						postID := 0
						for j := 0; j < len(postIDStr); j++ {
							postID = postID*10 + int(postIDStr[j]-'0')
						}

						post, cached := postCache[postID]
						if !cached {
							post = &index.Posts[postID]
							postCache[postID] = post
						}

						if versionFilter != "all" && post.Version != versionFilter {
							continue
						}

						if tagFilter != "" && !HasTagNormalized(post.NormalizedTags, tagFilter) {
							continue
						}

						freq := len(positions)
						docLen := float64(index.DocLens[postIDStr])
						score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/index.AvgDocLen)))

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
		for i, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}

			if checkPhraseUnified(index, i, phraseTerms) {
				scores[i] += ScorePhraseMatch
				for _, pt := range phraseTerms {
					highlightTerms[pt] = true
				}
			}
		}
	}

	// 3. Fallback for Empty Query Terms
	if len(scores) == 0 && originalQuery != "" {
		for i, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}

			match := false
			if strings.Contains(post.NormalizedTitle, originalQuery) {
				scores[i] += ScoreTitleMatch
				match = true
			}
			if strings.Contains(ToLower(post.Description), originalQuery) {
				scores[i] += 1.0
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
		post := &index.Posts[id]

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

		results = append(results, Result{
			ID:          id,
			Title:       title,
			Link:        post.Link,
			Description: post.Description,
			Version:     post.Version,
			Score:       score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 10 {
		results = results[:10]
	}

	for i := range results {
		post := &index.Posts[results[i].ID]
		idStr := ""
		id := results[i].ID
		if id == 0 {
			idStr = "0"
		} else {
			// Fast itoa
			var b [20]byte
			bp := len(b) - 1
			for id > 0 {
				b[bp] = byte(id%10) + '0'
				bp--
				id /= 10
			}
			idStr = string(b[bp+1:])
		}

		termOffsets := make(map[string][]int)
		for _, term := range finalHighlightTerms {
			if docMap, ok := index.Offsets[term]; ok {
				if offsets, found := docMap[idStr]; found {
					termOffsets[term] = offsets
				}
			}
		}

		results[i].Snippet = ExtractSnippet(post.Content, finalHighlightTerms, termOffsets)
	}

	return results
}

func checkPhraseUnified(index *models.SearchIndex, postID int, phraseTerms []string) bool {
	if len(phraseTerms) == 0 {
		return false
	}

	idStr := ""
	tempID := postID
	if tempID == 0 {
		idStr = "0"
	} else {
		var b [20]byte
		bp := len(b) - 1
		for tempID > 0 {
			b[bp] = byte(tempID%10) + '0'
			bp--
			tempID /= 10
		}
		idStr = string(b[bp+1:])
	}

	if len(phraseTerms) == 1 {
		if postMap, ok := index.Inverted[phraseTerms[0]]; ok {
			_, found := postMap[idStr]
			return found
		}
		return false
	}

	postMap, ok := index.Inverted[phraseTerms[0]]
	if !ok {
		return false
	}
	candidates, ok := postMap[idStr]
	if !ok {
		return false
	}

	for i := 1; i < len(phraseTerms); i++ {
		nextWord := phraseTerms[i]
		nextPostMap, ok := index.Inverted[nextWord]
		if !ok {
			return false
		}
		nextPositions, ok := nextPostMap[idStr]
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

func getHighlightRegex(terms []string) *regexp.Regexp {
	if len(terms) == 0 {
		return nil
	}

	sortedTerms := make([]string, len(terms))
	copy(sortedTerms, terms)
	sort.Strings(sortedTerms)

	key := strings.Join(sortedTerms, "|")
	if r, ok := replacerCache.Get(key); ok {
		return r
	}

	escaped := make([]string, 0, len(sortedTerms))
	for _, term := range sortedTerms {
		if len(term) < 2 {
			continue
		}
		escaped = append(escaped, regexp.QuoteMeta(term))
	}

	if len(escaped) == 0 {
		return nil
	}

	pattern := "(?i)\\b((" + strings.Join(escaped, "|") + ")\\w*)\\b"
	r, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}

	replacerCache.Add(key, r)
	return r
}

func ExtractSnippet(content string, terms []string, termOffsets map[string][]int) string {
	if len(content) == 0 {
		return ""
	}
	if len(content) > MaxSnippetContentLength {
		content = content[:MaxSnippetContentLength]
	}

	if len(terms) == 0 {
		if len(content) > DefaultSnippetLength {
			return content[:DefaultSnippetLength] + "..."
		}
		return content
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
			return content[:DefaultSnippetLength] + "..."
		}
		return content
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].pos < matches[j].pos
	})

	bestStart := matches[0].pos
	maxScore := 0

	windowSize := DefaultSnippetLength
	for i := 0; i < len(matches); i++ {
		count := 0
		uniqueTerms := make(map[string]bool)
		for j := i; j < len(matches) && matches[j].pos < matches[i].pos+windowSize; j++ {
			count++
			uniqueTerms[matches[j].term] = true
		}

		score := len(uniqueTerms)*100 + count
		if score > maxScore {
			maxScore = score
			bestStart = matches[i].pos
		}
	}

	start := max(bestStart-SnippetContextBefore, 0)
	end := start + windowSize + SnippetContextBefore
	if end > len(content) {
		end = len(content)
		start = max(end-(windowSize+SnippetContextBefore), 0)
	}

	if start > 0 {
		idx := strings.Index(content[start:], " ")
		if idx != -1 && idx < 15 {
			start += idx + 1
		}
	}

	snippet := content[start:end]

	re := getHighlightRegex(terms)
	if re != nil {
		snippet = re.ReplaceAllString(snippet, "<b>$1</b>")
	}

	var b strings.Builder
	b.Grow(len(snippet) + 10)
	if start > 0 {
		b.WriteString("...")
	}
	b.WriteString(snippet)
	if end < len(content) {
		b.WriteString("...")
	}

	return b.String()
}
