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
	exactTitleBoost            = 100.0
	prefixTitleBoost           = 70.0
	substringTitleBoost        = 30.0
	exactTitleWordBoost        = 50.0
	prefixTitleWordBoost       = 25.0
	tier1ExactFloor            = 10000.0 // Exact Title/Tag match
	tier2PrefixFloor           = 5000.0  // Prefix Title match
	tier3WordFloor             = 1000.0  // Individual word match in title
	descriptionContainsBoost   = 5.0
	minHighlightTermLength     = 1
	maxProximitySlop           = 15
	proximityBoost             = 3.0
	secondsPerMonth            = 2592000
	recencyWeight              = 0.5
	recencyLambda              = 0.05
	recencyBaseBoost           = 1.0
	bm25Smoothing              = 0.5
)

// Context holds the context for a search execution.
type Context struct {
	Index         *models.SearchIndex
	QueryTerms    []string
	Phrases       [][]string
	TagFilter     string
	OriginalQuery string
	TermInfos     []core.QueryTerm
}

// Scorer defines the interface for different scoring strategies
type Scorer interface {
	Score(ctx *Context, opts *ScoringOptions)
}

// TagScorer boosts scores based on tag matches
type TagScorer struct{}

// Score boosts results that match the active tag filter.
func (scorer *TagScorer) Score(ctx *Context, opts *ScoringOptions) {
	if ctx.TagFilter != "" && len(ctx.QueryTerms) == 0 {
		opts.HighlightTerms[ctx.TagFilter] = true
		for id, item := range ctx.Index.Items {
			match := false
			for _, terms := range item.NormalizedTaxs {
				if slices.Contains(terms, ctx.TagFilter) {
					match = true
					break
				}
			}
			if match {
				opts.Scores[id] += opts.Ranking.TagBoost
			}
		}
	}
}

// BM25Scorer calculates base scores using BM25 and fuzzy matching
type BM25Scorer struct{}

// Score applies BM25 scoring and fuzzy matching when needed.
func (scorer *BM25Scorer) Score(ctx *Context, opts *ScoringOptions) {
	for _, term := range ctx.QueryTerms {
		if posts, ok := ctx.Index.Inverted[term]; ok {
			opts.Modifier = scoreModifierBase
			scorer.applyBM25Score(ctx, posts, term, opts)
		} else {
			scorer.scoreFuzzy(ctx, term, opts)
		}
	}
}

func (scorer *BM25Scorer) applyBM25Score(ctx *Context, items map[string][]uint32, _ string, opts *ScoringOptions) {
	docFreq := len(items)
	invDocFreq := math.Log(1 + (float64(ctx.Index.TotalItems)-float64(docFreq)+bm25Smoothing)/(float64(docFreq)+bm25Smoothing))
	avgDocLen := ctx.Index.AvgDocLen

	for ContentID, positions := range items {
		item, ok := ctx.Index.Items[ContentID]
		if !ok {
			continue
		}
		if ctx.TagFilter != "" {
			match := false
			for _, terms := range item.NormalizedTaxs {
				if slices.Contains(terms, ctx.TagFilter) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		freq := len(positions)
		docLen := float64(ctx.Index.ItemLens[ContentID])
		score := invDocFreq * (float64(freq) * (opts.K1 + 1)) / (float64(freq) + opts.K1*(1-opts.B+opts.B*(docLen/avgDocLen)))
		opts.Scores[ContentID] += score * opts.Modifier
	}
}

func (scorer *BM25Scorer) scoreFuzzy(ctx *Context, term string, opts *ScoringOptions) {
	var candidates []string
	// Trigram indices (size 3) cannot reliably match 2-character queries like "ke".
	// For queries < 3 chars, we must bypass the index and use a full inverted scan (FuzzyExpand).
	if ctx.Index.NgramIndex != nil && len([]rune(term)) >= 3 {
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
func (scorer *PhraseScorer) Score(ctx *Context, opts *ScoringOptions) {
	if len(ctx.QueryTerms) > 1 {
		for id := range opts.Scores {
			if checkPhraseUnified(ctx.Index, id, ctx.QueryTerms) {
				opts.Scores[id] += ScorePhraseMatch * phraseFullQueryBoostFactor
			}
		}
	}

	for _, phraseTerms := range ctx.Phrases {
		for id := range ctx.Index.Items {
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
func (scorer *TitleScorer) Score(ctx *Context, opts *ScoringOptions) {
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
					titleMatches[id] += opts.Ranking.TitleBoost * 0.5 // Boost for term presence in title
				}
				opts.HighlightTerms[term] = true
			}
		}
	}

	for id := range ctx.Index.Items {
		item := ctx.Index.Items[id]
		idNum, _ := strconv.ParseUint(id, 10, 64)
		switch {
		case item.NormalizedTitle == queryLower:
			titleMatches[idNum] += opts.Ranking.TitleBoost*2.0 + tier1ExactFloor
		case strings.HasPrefix(item.NormalizedTitle, queryLower):
			titleMatches[idNum] += opts.Ranking.TitleBoost*0.8 + tier2PrefixFloor
		case strings.Contains(item.NormalizedTitle, queryLower):
			titleMatches[idNum] += opts.Ranking.TitleBoost*0.3 + tier3WordFloor
		}

		description := core.ToLower(item.Description)
		if strings.Contains(description, queryLower) {
			idNum, _ := strconv.ParseUint(id, 10, 64)
			titleMatches[idNum] += opts.Ranking.DescriptionBoost
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
func (scorer *BoostScorer) Score(ctx *Context, opts *ScoringOptions) {
	if ctx.OriginalQuery == "" && ctx.TagFilter == "" {
		return
	}

	for id := range opts.Scores {
		item, ok := ctx.Index.Items[id]
		if !ok {
			continue
		}

		// Exact taxonomy term matches
		for _, terms := range item.NormalizedTaxs {
			for _, term := range terms {
				if term == ctx.OriginalQuery || term == ctx.TagFilter {
					opts.Scores[id] += opts.Ranking.TagBoost * exactTagBoostFactor
					opts.HighlightTerms[term] = true
				}
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
func (pipeline *Pipeline) Execute(ctx *Context, opts *ScoringOptions) {
	for _, scorer := range pipeline.scorers {
		scorer.Score(ctx, opts)
	}
}

// FilterScorer enforces required and excluded terms
type FilterScorer struct{}

// Score filters results based on required and excluded query terms.
func (scorer *FilterScorer) Score(ctx *Context, opts *ScoringOptions) {
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
func (scorer *ProximityScorer) Score(ctx *Context, opts *ScoringOptions) {
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

func (scorer *ProximityScorer) calculateProximityScore(ctx *Context, id string) float64 {
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
				switch {
				case diff <= maxProximitySlop:
					boost += proximityBoost / float64(diff+1)
					pos1++
					pos2++
				case list1[pos1] < list2[pos2]:
					pos1++
				default:
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
func (scorer *RecencyScorer) Score(ctx *Context, opts *ScoringOptions) {
	if ctx.Index.TotalItems == 0 {
		return
	}

	nowUnix := core.NowUnix()

	for id, score := range opts.Scores {
		item, ok := ctx.Index.Items[id]
		if !ok || item.Date == 0 {
			continue
		}

		// Calculate age in months
		ageMonths := float64(nowUnix-item.Date) / float64(secondsPerMonth)
		if ageMonths < 0 {
			ageMonths = 0
		}

		// Apply exponential decay: boost = 1.0 + weight * exp(-lambda * age)
		boost := recencyBaseBoost + recencyWeight*math.Exp(-recencyLambda*ageMonths)

		opts.Scores[id] = score * boost
	}
}

func checkPhraseUnified(index *models.SearchIndex, contentID string, phraseTerms []string) bool {
	if len(phraseTerms) == 0 {
		return false
	}

	if len(phraseTerms) == 1 {
		return checkSingleTermInPost(index, contentID, phraseTerms[0])
	}

	posList, ok := getInitialPositions(index, contentID, phraseTerms[0])
	if !ok {
		return false
	}

	for i := 1; i < len(phraseTerms); i++ {
		nextPositions, ok := getDecodedPositions(index, contentID, phraseTerms[i])
		if !ok {
			return false
		}

		posList = intersectPositions(posList, nextPositions)
		if len(posList) == 0 {
			return false
		}
	}

	return true
}

func checkSingleTermInPost(index *models.SearchIndex, contentID, term string) bool {
	if postMap, ok := index.Inverted[term]; ok {
		_, found := postMap[contentID]
		return found
	}
	return false
}

func getInitialPositions(index *models.SearchIndex, contentID, term string) ([]int, bool) {
	candidates, ok := getDecodedPositions(index, contentID, term)
	if !ok {
		return nil, false
	}
	posList := make([]int, len(candidates))
	copy(posList, candidates)
	return posList, true
}

func getDecodedPositions(index *models.SearchIndex, contentID, term string) ([]int, bool) {
	postMap, ok := index.Inverted[term]
	if !ok {
		return nil, false
	}
	candidates, ok := postMap[contentID]
	if !ok {
		return nil, false
	}
	return models.DecodePositions(candidates), true
}

func intersectPositions(posList, nextDecoded []int) []int {
	var newCandidates []int
	idx1, idx2 := 0, 0
	for idx1 < len(posList) && idx2 < len(nextDecoded) {
		switch {
		case nextDecoded[idx2] == posList[idx1]+1:
			newCandidates = append(newCandidates, nextDecoded[idx2])
			idx1++
			idx2++
		case nextDecoded[idx2] < posList[idx1]+1:
			idx2++
		default:
			idx1++
		}
	}
	return newCandidates
}
