package search

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// titleCaser is cached at package level to avoid recreation on every snippet extraction
var titleCaser = cases.Title(language.English)

// replacerCache caches string replacers for snippet highlighting
var (
	replacerCache   = make(map[string]*strings.Replacer)
	replacerCacheMu sync.RWMutex
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
	ID          int
	Title       string
	Link        string
	Description string
	Snippet     string
	Version     string
	Score       float64
}

// PerformSearch executes a search query against the index with fuzzy and phrase support
func PerformSearch(index *models.SearchIndex, query string, versionFilter string) []Result {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

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

	maxResults := len(index.Posts)
	if maxResults > 100 {
		maxResults = 100
	}
	scores := make(map[int]float64, maxResults)

	k1 := 1.2
	b := 0.75

	postCache := make(map[int]*models.PostRecord, maxResults)
	matchedTerms := make(map[string]bool)

	// Process individual terms with BM25
	for _, term := range queryTerms {
		if posts, ok := index.Inverted[term]; ok {
			df := len(posts)
			idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))

			for postID, freq := range posts {
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

				docLen := float64(index.DocLens[postID])
				score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/index.AvgDocLen)))
				scores[postID] += score
				matchedTerms[term] = true
			}
		} else {
			// Try fuzzy matching if exact term not found
			fuzzyCandidates := FuzzyExpand(term, index.Inverted, MaxEditDistance)
			for _, fuzzyTerm := range fuzzyCandidates {
				if posts, ok := index.Inverted[fuzzyTerm]; ok {
					df := len(posts)
					idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))

					for postID, freq := range posts {
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

						docLen := float64(index.DocLens[postID])
						score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/index.AvgDocLen)))
						// Reduce score for fuzzy matches
						scores[postID] += score * ScoreFuzzyModifier
					}
				}
			}
		}
	}

	// Process phrase matches (higher score)
	for _, phrase := range parsed.Phrases {
		for i, post := range index.Posts {
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}

			// Check if phrase appears in title (highest score)
			if strings.Contains(post.NormalizedTitle, phrase) {
				scores[i] += ScorePhraseMatch * 2
				continue
			}

			// Check if phrase appears in content
			if strings.Contains(strings.ToLower(post.Content), phrase) {
				scores[i] += ScorePhraseMatch
			}
		}
	}

	// Handle tag-only queries
	if len(queryTerms) == 0 && len(parsed.Phrases) == 0 && tagFilter != "" {
		for i := range index.Posts {
			post := &index.Posts[i]
			if versionFilter != "all" && post.Version != versionFilter {
				continue
			}
			if HasTagNormalized(post.NormalizedTags, tagFilter) {
				scores[i] = 1.0
			}
		}
	}

	// Boost title and tag matches
	originalQuery := strings.ToLower(query)
	for id := range scores {
		post := &index.Posts[id]

		// Title match boost
		if originalQuery != "" && strings.Contains(post.NormalizedTitle, originalQuery) {
			scores[id] += ScoreTitleMatch
		}

		// Tag match boost
		for _, tag := range post.NormalizedTags {
			if tag == originalQuery || tag == tagFilter {
				scores[id] += ScoreTagMatch
			}
		}
	}

	// Build results
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
			Snippet:     ExtractSnippet(post.Content, queryTerms),
			Version:     post.Version,
			Score:       score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit results
	if len(results) > 10 {
		results = results[:10]
	}

	return results
}

// Tokenize splits text into tokens (legacy function for compatibility)
func Tokenize(text string) []string {
	if len(text) == 0 {
		return nil
	}
	estimatedWords := max(8, len(text)/5)
	words := make([]string, 0, estimatedWords)
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		words = append(words, word)
	}
	return words
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// HasTagNormalized checks tags against a pre-normalized target using exact match
func HasTagNormalized(normalizedTags []string, target string) bool {
	for _, t := range normalizedTags {
		if t == target {
			return true
		}
	}
	return false
}

// getReplacer returns a cached strings.Replacer for the given term
func getReplacer(term string) *strings.Replacer {
	replacerCacheMu.RLock()
	if r, ok := replacerCache[term]; ok {
		replacerCacheMu.RUnlock()
		return r
	}
	replacerCacheMu.RUnlock()

	r := strings.NewReplacer(
		term, "<b>"+term+"</b>",
		titleCaser.String(term), "<b>"+titleCaser.String(term)+"</b>",
	)

	replacerCacheMu.Lock()
	replacerCache[term] = r
	replacerCacheMu.Unlock()
	return r
}

func ExtractSnippet(content string, terms []string) string {
	if len(content) > MaxSnippetContentLength {
		content = content[:MaxSnippetContentLength]
	}

	if len(terms) == 0 {
		if len(content) > DefaultSnippetLength {
			return content[:DefaultSnippetLength] + "..."
		}
		return content
	}

	contentLower := strings.ToLower(content)
	firstPos := -1
	for _, term := range terms {
		pos := strings.Index(contentLower, term)
		if pos != -1 && (firstPos == -1 || pos < firstPos) {
			firstPos = pos
		}
	}

	if firstPos == -1 {
		if len(content) > DefaultSnippetLength {
			return content[:DefaultSnippetLength] + "..."
		}
		return content
	}

	start := firstPos - SnippetContextBefore
	if start < 0 {
		start = 0
	}
	end := firstPos + SnippetContextAfter
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]

	for _, term := range terms {
		re := getReplacer(term)
		snippet = re.Replace(snippet)
	}

	var b strings.Builder
	b.Grow(len(snippet) + 6)
	if start > 0 {
		b.WriteString("...")
	}
	b.WriteString(snippet)
	if end < len(content) {
		b.WriteString("...")
	}

	return b.String()
}
