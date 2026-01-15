package search

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"my-ssg/builder/models"
)

type Result struct {
	ID          int
	Title       string
	Link        string
	Description string
	Snippet     string
	Score       float64
}

func PerformSearch(index *models.SearchIndex, query string) []Result {
	query = strings.ToLower(query)
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

	queryTerms := Tokenize(query)
	scores := make(map[int]float64)

	// BM25 Constants
	k1 := 1.2
	b := 0.75

	for _, term := range queryTerms {
		if posts, ok := index.Inverted[term]; ok {
			df := len(posts)
			idf := math.Log(1 + (float64(index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))

			for postID, freq := range posts {
				post := index.Posts[postID]

				if tagFilter != "" {
					if !HasTag(post.Tags, tagFilter) {
						continue
					}
				}

				docLen := float64(index.DocLens[postID])
				score := idf * (float64(freq) * (k1 + 1)) / (float64(freq) + k1*(1-b+b*(docLen/index.AvgDocLen)))
				scores[postID] += score
			}
		}
	}

	// Boost title and tag matches
	for i, post := range index.Posts {
		if tagFilter != "" && !HasTag(post.Tags, tagFilter) {
			continue
		}

		lowerTitle := strings.ToLower(post.Title)
		if query != "" && strings.Contains(lowerTitle, query) {
			scores[i] += 10.0
		}
		for _, tag := range post.Tags {
			if strings.ToLower(tag) == query {
				scores[i] += 5.0
			}
		}
	}

	var results []Result
	for id, score := range scores {
		post := index.Posts[id]
		results = append(results, Result{
			ID:          id,
			Title:       post.Title,
			Link:        post.Link,
			Description: post.Description,
			Snippet:     ExtractSnippet(post.Content, queryTerms),
			Score:       score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > 10 {
		results = results[:10]
	}

	return results
}

func Tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func HasTag(tags []string, target string) bool {
	for _, t := range tags {
		if strings.ToLower(t) == strings.ToLower(target) {
			return true
		}
	}
	return false
}

func ExtractSnippet(content string, terms []string) string {
	if len(terms) == 0 {
		if len(content) > 150 {
			return content[:150] + "..."
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
		if len(content) > 150 {
			return content[:150] + "..."
		}
		return content
	}

	start := firstPos - 60
	if start < 0 {
		start = 0
	}
	end := firstPos + 90
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]

	for _, term := range terms {
		re := strings.NewReplacer(term, "<b>"+term+"</b>", strings.Title(term), "<b>"+strings.Title(term)+"</b>")
		snippet = re.Replace(snippet)
	}

	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet = snippet + "..."
	}

	return snippet
}
