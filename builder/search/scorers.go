package search

import (
	"math"
	"slices"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

// SearchContext holds the context for a search execution
type SearchContext struct {
	Index         *models.SearchIndex
	QueryTerms    []string
	Phrases       [][]string
	TagFilter     string
	VersionFilter string
	OriginalQuery string
}

// Scorer defines the interface for different scoring strategies
type Scorer interface {
	Score(ctx *SearchContext, opts *SearchScoringOptions)
}

// TagScorer boosts scores based on tag matches
type TagScorer struct{}

func (s *TagScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if ctx.TagFilter != "" && len(ctx.QueryTerms) == 0 {
		opts.HighlightTerms[ctx.TagFilter] = true
		for id, post := range ctx.Index.Posts {
			if ctx.VersionFilter != "all" && post.Version != ctx.VersionFilter {
				continue
			}
			if slices.Contains(post.NormalizedTags, ctx.TagFilter) {
				opts.Scores[id] += ScoreTagMatch
			}
		}
	}
}

// BM25Scorer calculates base scores using BM25 and fuzzy matching
type BM25Scorer struct{}

func (s *BM25Scorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	for _, term := range ctx.QueryTerms {
		if posts, ok := ctx.Index.Inverted[term]; ok {
			opts.Modifier = 1.0
			s.applyBM25Score(ctx, posts, term, opts)
		} else {
			s.scoreFuzzy(ctx, term, opts)
		}
	}
}

func (s *BM25Scorer) applyBM25Score(ctx *SearchContext, posts map[string][]uint32, term string, opts *SearchScoringOptions) {
	df := len(posts)
	idf := math.Log(1 + (float64(ctx.Index.TotalDocs)-float64(df)+0.5)/(float64(df)+0.5))
	avgDocLen := ctx.Index.AvgDocLen

	for postID, positions := range posts {
		post, ok := ctx.Index.Posts[postID]
		if !ok || (ctx.VersionFilter != "all" && post.Version != ctx.VersionFilter) || (ctx.TagFilter != "" && !slices.Contains(post.NormalizedTags, ctx.TagFilter)) {
			continue
		}

		freq := len(positions)
		docLen := float64(ctx.Index.DocLens[postID])
		score := idf * (float64(freq) * (opts.K1 + 1)) / (float64(freq) + opts.K1*(1-opts.B+opts.B*(docLen/avgDocLen)))
		opts.Scores[postID] += score * opts.Modifier
	}
}

func (s *BM25Scorer) scoreFuzzy(ctx *SearchContext, term string, opts *SearchScoringOptions) {
	var candidates []string
	if ctx.Index.NgramIndex != nil {
		candidates = core.FuzzyExpandWithNgrams(term, ctx.Index.NgramIndex, core.MaxEditDistance)
	} else {
		candidates = core.FuzzyExpand(term, ctx.Index.Inverted, core.MaxEditDistance)
	}

	for _, candTerm := range candidates {
		opts.HighlightTerms[candTerm] = true
		if posts, ok := ctx.Index.Inverted[candTerm]; ok {
			opts.Modifier = ScoreFuzzyModifier
			if strings.HasPrefix(candTerm, term) {
				opts.Modifier = 0.9
			}
			s.applyBM25Score(ctx, posts, candTerm, opts)
		}
	}
}

// PhraseScorer boosts scores for phrase matches
type PhraseScorer struct{}

func (s *PhraseScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if len(ctx.QueryTerms) > 1 {
		for id := range opts.Scores {
			if checkPhraseUnified(ctx.Index, id, ctx.QueryTerms) {
				opts.Scores[id] += ScorePhraseMatch * 1.2
			}
		}
	}

	for _, phraseTerms := range ctx.Phrases {
		for id, post := range ctx.Index.Posts {
			if (ctx.VersionFilter != "all" && post.Version != ctx.VersionFilter) || !checkPhraseUnified(ctx.Index, id, phraseTerms) {
				continue
			}
			opts.Scores[id] += ScorePhraseMatch
			for _, pt := range phraseTerms {
				opts.HighlightTerms[pt] = true
			}
		}
	}
}

// FallbackScorer provides a safety net for empty query arrays
type FallbackScorer struct{}

func (s *FallbackScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if len(opts.Scores) > 0 || ctx.OriginalQuery == "" {
		return
	}

	for id, post := range ctx.Index.Posts {
		if ctx.VersionFilter != "all" && post.Version != ctx.VersionFilter {
			continue
		}

		match := false
		if strings.Contains(post.NormalizedTitle, ctx.OriginalQuery) {
			opts.Scores[id] += ScoreTitleMatch
			match = true
		}
		if strings.Contains(core.ToLower(post.Description), ctx.OriginalQuery) {
			opts.Scores[id] += 1.0
			match = true
		}

		if match {
			for word := range strings.FieldsSeq(ctx.OriginalQuery) {
				if len(word) > 2 {
					opts.HighlightTerms[word] = true
				}
			}
		}
	}
}

// BoostScorer boosts scores based on title and exact tag matches
type BoostScorer struct{}

func (s *BoostScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	for id := range opts.Scores {
		post, ok := ctx.Index.Posts[id]
		if !ok {
			continue
		}

		if ctx.OriginalQuery != "" && strings.Contains(post.NormalizedTitle, ctx.OriginalQuery) {
			opts.Scores[id] += ScoreTitleMatch
		}

		for _, tag := range post.NormalizedTags {
			if tag == ctx.OriginalQuery || tag == ctx.TagFilter {
				opts.Scores[id] += ScoreTagMatch
			}
		}
	}
}

// Pipeline orchestrates multiple scorers
type Pipeline struct {
	scorers []Scorer
}

func NewPipeline(scorers ...Scorer) *Pipeline {
	return &Pipeline{scorers: scorers}
}

func (p *Pipeline) Execute(ctx *SearchContext, opts *SearchScoringOptions) {
	for _, scorer := range p.scorers {
		scorer.Score(ctx, opts)
	}
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

	decoded := models.DecodePositions(candidates)
	posList := make([]int, len(decoded))
	copy(posList, decoded)

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

		nextDecoded := models.DecodePositions(nextPositions)
		var newCandidates []int
		p1, p2 := 0, 0
		for p1 < len(posList) && p2 < len(nextDecoded) {
			if nextDecoded[p2] == posList[p1]+1 {
				newCandidates = append(newCandidates, nextDecoded[p2])
				p1++
				p2++
			} else if nextDecoded[p2] < posList[p1]+1 {
				p2++
			} else {
				p1++
			}
		}

		if len(newCandidates) == 0 {
			return false
		}
		posList = newCandidates
	}

	return true
}
