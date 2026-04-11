package search

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

const (
	scoreModifierBase          = 1.0
	scoreModifierPrefixMatch   = 0.9
	phraseFullQueryBoostFactor = 1.2
	exactTagBoostFactor        = 2.0
	exactTitleBoost            = 50.0
	prefixTitleBoost           = 30.0
	substringTitleBoost        = 15.0
	exactTitleWordBoost        = 20.0
	prefixTitleWordBoost       = 10.0
	descriptionContainsBoost   = 5.0
	minHighlightTermLength     = 2
	maxProximitySlop           = 15
	proximityBoost             = 3.0
	secondsPerMonth            = 2592000
	recencyWeight              = 0.5
	recencyLambda              = 0.05
	recencyBaseBoost           = 1.0
	bm25Smoothing              = 0.5
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

// Score boosts results that match the active tag filter.
func (scorer *TagScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
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

// Score applies BM25 scoring and fuzzy matching when needed.
func (scorer *BM25Scorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	for _, term := range ctx.QueryTerms {
		if posts, ok := ctx.Index.Inverted[term]; ok {
			opts.Modifier = scoreModifierBase
			scorer.applyBM25Score(ctx, posts, term, opts)
		} else {
			scorer.scoreFuzzy(ctx, term, opts)
		}
	}
}

func (scorer *BM25Scorer) applyBM25Score(ctx *SearchContext, posts map[string][]uint32, term string, opts *SearchScoringOptions) {
	docFreq := len(posts)
	invDocFreq := math.Log(1 + (float64(ctx.Index.TotalDocs)-float64(docFreq)+bm25Smoothing)/(float64(docFreq)+bm25Smoothing))
	avgDocLen := ctx.Index.AvgDocLen

	for postID, positions := range posts {
		post, ok := ctx.Index.Posts[postID]
		if !ok || (ctx.TagFilter != "" && !slices.Contains(post.NormalizedTags, ctx.TagFilter)) {
			continue
		}

		freq := len(positions)
		docLen := float64(ctx.Index.DocLens[postID])
		score := invDocFreq * (float64(freq) * (opts.K1 + 1)) / (float64(freq) + opts.K1*(1-opts.B+opts.B*(docLen/avgDocLen)))
		opts.Scores[postID] += score * opts.Modifier
	}
}

func (scorer *BM25Scorer) scoreFuzzy(ctx *SearchContext, term string, opts *SearchScoringOptions) {
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
				opts.Modifier = scoreModifierPrefixMatch
			}
			scorer.applyBM25Score(ctx, posts, candTerm, opts)
		}
	}
}

// PhraseScorer boosts scores for phrase matches
type PhraseScorer struct{}

// Score boosts results that match phrases or full query terms.
func (scorer *PhraseScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if len(ctx.QueryTerms) > 1 {
		for id := range opts.Scores {
			if checkPhraseUnified(ctx.Index, id, ctx.QueryTerms) {
				opts.Scores[id] += ScorePhraseMatch * phraseFullQueryBoostFactor
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

// Score boosts results that match title or description text.
func (scorer *TitleScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if ctx.OriginalQuery == "" {
		return
	}

	query := ctx.OriginalQuery
	queryLower := core.ToLower(query)

	titleMatches := make(map[uint64]float64)

	if ctx.Index.TitleInverted != nil {
		for _, term := range ctx.QueryTerms {
			if ids, ok := ctx.Index.TitleInverted[term]; ok {
				for _, id := range ids {
					titleMatches[id] += exactTitleWordBoost
				}
				opts.HighlightTerms[term] = true
			}
		}
	}

	for id := range ctx.Index.Posts {
		post := ctx.Index.Posts[id]
		if post.NormalizedTitle == queryLower {
			idNum, _ := strconv.ParseUint(id, 10, 64)
			titleMatches[idNum] += exactTitleBoost
		} else if strings.HasPrefix(post.NormalizedTitle, queryLower) {
			idNum, _ := strconv.ParseUint(id, 10, 64)
			titleMatches[idNum] += prefixTitleBoost
		} else if strings.Contains(post.NormalizedTitle, queryLower) {
			idNum, _ := strconv.ParseUint(id, 10, 64)
			titleMatches[idNum] += substringTitleBoost
		}

		description := core.ToLower(post.Description)
		if strings.Contains(description, queryLower) {
			idNum, _ := strconv.ParseUint(id, 10, 64)
			titleMatches[idNum] += descriptionContainsBoost
		}
	}

	for idNum, score := range titleMatches {
		if score > 0 {
			idStr := strconv.FormatUint(idNum, 10)
			opts.Scores[idStr] += score
		}
	}

	for _, word := range strings.Fields(query) {
		if len(word) > minHighlightTermLength {
			opts.HighlightTerms[word] = true
		}
	}
}

// BoostScorer boosts scores based on exact tag matches
type BoostScorer struct{}

// Score boosts results that match exact tags.
func (scorer *BoostScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
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
				opts.Scores[id] += ScoreTagMatch * exactTagBoostFactor
				opts.HighlightTerms[tag] = true
			}
		}
	}
}

// Pipeline orchestrates multiple scorers
type Pipeline struct {
	scorers []Scorer
}

// NewPipeline constructs a scoring pipeline from scorers.
func NewPipeline(scorers ...Scorer) *Pipeline {
	return &Pipeline{scorers: scorers}
}

// Execute runs all scorers in order.
func (pipeline *Pipeline) Execute(ctx *SearchContext, opts *SearchScoringOptions) {
	for _, scorer := range pipeline.scorers {
		scorer.Score(ctx, opts)
	}
}

// FilterScorer enforces required and excluded terms
type FilterScorer struct{}

// Score filters results based on required and excluded query terms.
func (scorer *FilterScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
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

// Score boosts results where terms appear close together.
func (scorer *ProximityScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
	if len(ctx.QueryTerms) < 2 {
		return
	}

	for id := range opts.Scores {
		score := scorer.calculateProximityScore(ctx, id)
		if score > 0 {
			opts.Scores[id] += score
		}
	}
}

func (scorer *ProximityScorer) calculateProximityScore(ctx *SearchContext, id string) float64 {
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
	// Simple heuristic: check for any two terms within maxSlop
	var boost float64
	for i := 0; i < len(termPositions); i++ {
		for j := i + 1; j < len(termPositions); j++ {
			pos1, pos2 := 0, 0
			list1, list2 := termPositions[i], termPositions[j]
			for pos1 < len(list1) && pos2 < len(list2) {
				diff := list1[pos1] - list2[pos2]
				if diff < 0 {
					diff = -diff
				}
				if diff <= maxProximitySlop {
					boost += proximityBoost / float64(diff+1)
					pos1++
					pos2++
				} else if list1[pos1] < list2[pos2] {
					pos1++
				} else {
					pos2++
				}
			}
		}
	}

	return boost
}

// RecencyScorer boosts scores for newer documents using an exponential decay function
type RecencyScorer struct{}

// Score boosts newer results using an exponential decay.
func (scorer *RecencyScorer) Score(ctx *SearchContext, opts *SearchScoringOptions) {
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
		ageMonths := float64(nowUnix-post.Date) / float64(secondsPerMonth)
		if ageMonths < 0 {
			ageMonths = 0
		}

		// Apply exponential decay: boost = 1.0 + weight * exp(-lambda * age)
		boost := recencyBaseBoost + recencyWeight*math.Exp(-recencyLambda*ageMonths)

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
		idx1, idx2 := 0, 0
		for idx1 < len(posList) && idx2 < len(nextDecoded) {
			if nextDecoded[idx2] == posList[idx1]+1 {
				newCandidates = append(newCandidates, nextDecoded[idx2])
				idx1++
				idx2++
			} else if nextDecoded[idx2] < posList[idx1]+1 {
				idx2++
			} else {
				idx1++
			}
		}

		if len(newCandidates) == 0 {
			return false
		}
		posList = newCandidates
	}

	return true
}
