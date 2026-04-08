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
	OriginalQuery string
	TermInfos     []core.QueryTerm
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
		if !ok || (ctx.TagFilter != "" && !slices.Contains(post.NormalizedTags, ctx.TagFilter)) {
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
		for id := range ctx.Index.Posts {
			if !checkPhraseUnified(ctx.Index, id, phraseTerms) {
				continue
			}
			opts.Scores[id] += ScorePhraseMatch
			for _, pt := range phraseTerms {
				opts.HighlightTerms[pt] = true
			}
		}
	}
}

// TitleScorer boosts scores based on title and description matches
type TitleScorer struct{}

func (s *TitleScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if ctx.OriginalQuery == "" {
		return
	}

	query := ctx.OriginalQuery
	for id, post := range ctx.Index.Posts {
		title := post.NormalizedTitle
		score := 0.0

		// 1. Exact Title Match (Highest priority)
		if title == query {
			score += 50.0
		} else if strings.HasPrefix(title, query) {
			// 2. Prefix Title Match
			score += 30.0
		} else if strings.Contains(title, query) {
			// 3. Substring Title Match
			score += 15.0
		}

		// 4. Word-level matching within title
		titleWords := strings.Fields(title)
		for _, word := range titleWords {
			if word == query {
				score += 20.0
			} else if strings.HasPrefix(word, query) {
				score += 10.0
			}
		}

		// 5. Description matching (Lower priority)
		desc := core.ToLower(post.Description)
		if strings.Contains(desc, query) {
			score += 5.0
		}

		if score > 0 {
			opts.Scores[id] += score
			// Add terms to highlight
			for _, word := range strings.Fields(query) {
				if len(word) > 2 {
					opts.HighlightTerms[word] = true
				}
			}
		}
	}
}

// BoostScorer boosts scores based on exact tag matches
type BoostScorer struct{}

func (s *BoostScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if ctx.OriginalQuery == "" && ctx.TagFilter == "" {
		return
	}

	for id := range opts.Scores {
		post, ok := ctx.Index.Posts[id]
		if !ok {
			continue
		}

		// Exact tag matches
		for _, tag := range post.NormalizedTags {
			if tag == ctx.OriginalQuery || tag == ctx.TagFilter {
				opts.Scores[id] += ScoreTagMatch * 2.0
				opts.HighlightTerms[tag] = true
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

// FilterScorer enforces required and excluded terms
type FilterScorer struct{}

func (s *FilterScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if len(opts.TermInfos) == 0 {
		return
	}

	for id := range opts.Scores {
		for _, info := range opts.TermInfos {
			if info.Required {
				if posts, ok := ctx.Index.Inverted[info.Term]; !ok {
					delete(opts.Scores, id)
					break
				} else if _, found := posts[id]; !found {
					delete(opts.Scores, id)
					break
				}
			}
			if info.Excluded {
				if posts, ok := ctx.Index.Inverted[info.Term]; ok {
					if _, found := posts[id]; found {
						delete(opts.Scores, id)
						break
					}
				}
			}
		}
	}
}

// ProximityScorer rewards documents where query terms appear close to each other
type ProximityScorer struct{}

func (s *ProximityScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if len(ctx.QueryTerms) < 2 {
		return
	}

	for id := range opts.Scores {
		score := s.calculateProximityScore(ctx, id)
		if score > 0 {
			opts.Scores[id] += score
		}
	}
}

func (s *ProximityScorer) calculateProximityScore(ctx *SearchContext, id string) float64 {
	// Find all positions for each term in this document
	termPositions := make([][]int, 0, len(ctx.QueryTerms))
	for _, term := range ctx.QueryTerms {
		if posts, ok := ctx.Index.Inverted[term]; ok {
			if posData, found := posts[id]; found {
				termPositions = append(termPositions, models.DecodePositions(posData))
			}
		}
	}

	if len(termPositions) < 2 {
		return 0
	}

	// Slop window size
	const maxSlop = 15
	const proximityBoost = 3.0

	// Simple heuristic: check for any two terms within maxSlop
	boost := 0.0
	for i := 0; i < len(termPositions); i++ {
		for j := i + 1; j < len(termPositions); j++ {
			p1, p2 := 0, 0
			list1, list2 := termPositions[i], termPositions[j]
			for p1 < len(list1) && p2 < len(list2) {
				diff := list1[p1] - list2[p2]
				if diff < 0 {
					diff = -diff
				}
				if diff <= maxSlop {
					boost += proximityBoost / float64(diff+1)
					p1++
					p2++
				} else if list1[p1] < list2[p2] {
					p1++
				} else {
					p2++
				}
			}
		}
	}

	return boost
}

// RecencyScorer boosts scores for newer documents using an exponential decay function
type RecencyScorer struct{}

func (s *RecencyScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if ctx.Index.TotalDocs == 0 {
		return
	}

	nowUnix := core.NowUnix()

	for id, score := range opts.Scores {
		post, ok := ctx.Index.Posts[id]
		if !ok || post.Date == 0 {
			continue
		}

		// Calculate age in months
		const secondsInMonth = 2592000
		ageMonths := float64(nowUnix-post.Date) / float64(secondsInMonth)
		if ageMonths < 0 {
			ageMonths = 0
		}

		// Apply exponential decay: boost = 1.0 + weight * exp(-lambda * age)
		const weight = 0.5
		const lambda = 0.05 // Halves boost roughly every 14 months
		boost := 1.0 + weight*math.Exp(-lambda*ageMonths)

		opts.Scores[id] = score * boost
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
