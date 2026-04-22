package search

import (
	"math"
	"slices"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

const (
	scoreModifierBase          = 1.0
	scoreModifierPrefixMatch   = 0.9
	phraseFullQueryBoostFactor = 1.2
	exactTagBoostFactor        = 2.0
	tier1ExactFloor            = 10000.0 // Exact Title/Tag match
	tier2PrefixFloor           = 5000.0  // Prefix Title match
	tier3WordFloor             = 1000.0  // Individual word match in title
	minHighlightTermLength     = 1
	maxProximitySlop           = 15
	proximityBoost             = 3.0
	secondsPerMonth            = 2592000
	recencyWeight              = 0.5
	recencyLambda              = 0.05
	recencyBaseBoost           = 1.0
	bm25Smoothing              = 0.5
	// ScoreFuzzyModifier is the multiplier applied to fuzzy search matches.
	ScoreFuzzyModifier = 0.7
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
	if ctx.TagFilter == "" {
		return
	}

	tagLower := strings.ToLower(ctx.TagFilter)
	for id := uint32(0); id < uint32(len(ctx.Index.Items)); id++ {
		item := ctx.Index.Items[id]
		match := false
		for _, terms := range item.NormalizedTaxs {
			if slices.Contains(terms, tagLower) {
				match = true
				break
			}
		}
		if match {
			opts.Scores[id] += opts.Ranking.TagBoost
		}
	}
}

// BM25Scorer calculates base scores using BM25 and fuzzy matching
type BM25Scorer struct{}

// Score applies BM25 scoring and fuzzy matching when needed.
func (scorer *BM25Scorer) Score(ctx *Context, opts *ScoringOptions) {
	for _, term := range ctx.QueryTerms {
		termIdx := ctx.Index.LookupTerm(term)
		if termIdx >= 0 {
			opts.Modifier = scoreModifierBase
			scorer.applyBM25Score(ctx, termIdx, opts)
		} else {
			scorer.scoreFuzzy(ctx, term, opts)
		}
	}
}

func (scorer *BM25Scorer) applyBM25Score(ctx *Context, termIdx int, opts *ScoringOptions) {
	start := ctx.Index.PostingOffsets[termIdx]
	end := ctx.Index.PostingOffsets[termIdx+1]

	if start == end {
		return
	}

	docIDs := ctx.Index.DocIDs[start:end]
	absDocIDs := models.DecodeDocIDs(docIDs)
	docFreq := len(absDocIDs)

	invDocFreq := math.Log(1 + (float64(ctx.Index.TotalItems)-float64(docFreq)+bm25Smoothing)/(float64(docFreq)+bm25Smoothing))
	avgDocLen := ctx.Index.AvgDocLen

	for i, docID := range absDocIDs {
		// Filter by tag if active
		if ctx.TagFilter != "" {
			tagLower := strings.ToLower(ctx.TagFilter)
			item := ctx.Index.Items[docID]
			match := false
			for _, terms := range item.NormalizedTaxs {
				if slices.Contains(terms, tagLower) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		postingIdx := int(start) + i
		freq := int(ctx.Index.DocPosOffsets[postingIdx+1] - ctx.Index.DocPosOffsets[postingIdx])
		docLen := float64(ctx.Index.ItemLens[docID])

		score := invDocFreq * (float64(freq) * (opts.K1 + 1)) / (float64(freq) + opts.K1*(1-opts.B+opts.B*(docLen/avgDocLen)))
		opts.Scores[docID] += score * opts.Modifier
	}
}

func (scorer *BM25Scorer) scoreFuzzy(ctx *Context, term string, opts *ScoringOptions) {
	// Use Lexicon-based expansion
	candidates := core.FuzzyExpand(term, ctx.Index.Terms, core.MaxEditDistance)

	// Add prefix expansion as well for live search
	prefixes := core.PrefixExpand(term, ctx.Index.Terms)
	candidates = append(candidates, prefixes...)
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)

	for _, candTerm := range candidates {
		termIdx := ctx.Index.LookupTerm(candTerm)
		if termIdx < 0 {
			continue
		}

		opts.Modifier = ScoreFuzzyModifier
		if strings.HasPrefix(candTerm, term) {
			opts.Modifier = scoreModifierPrefixMatch
		}

		scorer.applyBM25Score(ctx, termIdx, opts)
	}
}

// PhraseScorer boosts scores for phrase matches
type PhraseScorer struct{}

// Score boosts scores for phrase matches in the document.
func (scorer *PhraseScorer) Score(ctx *Context, opts *ScoringOptions) {
	// Full query phrase match
	if len(ctx.QueryTerms) > 1 {
		for id := range opts.Scores {
			if checkPhraseUnified(ctx.Index, id, ctx.QueryTerms) {
				opts.Scores[id] += ScorePhraseMatch * phraseFullQueryBoostFactor
			}
		}
	}

	// Explicit phrases "quoted like this"
	for _, phraseTerms := range ctx.Phrases {
		for id := uint32(0); id < uint32(len(ctx.Index.Items)); id++ {
			if checkPhraseUnified(ctx.Index, id, phraseTerms) {
				opts.Scores[id] += ScorePhraseMatch
				for _, pt := range phraseTerms {
					opts.HighlightTerms[pt] = true
				}
			}
		}
	}
}

// TitleScorer boosts scores based on title and description matches
type TitleScorer struct{}

// Score boosts results that match terms in the title or description.
func (scorer *TitleScorer) Score(ctx *Context, opts *ScoringOptions) {
	if ctx.OriginalQuery == "" {
		return
	}

	queryLower := strings.ToLower(ctx.OriginalQuery)

	// 1. Direct title postings for fast title scoring
	for _, term := range ctx.QueryTerms {
		termIdx := ctx.Index.LookupTerm(term)
		if termIdx >= 0 {
			tIDs := ctx.Index.GetTitlePostings(termIdx)
			if len(tIDs) > 0 {
				absTIDs := models.DecodeDocIDs(tIDs)
				for _, tid := range absTIDs {
					opts.Scores[tid] += opts.Ranking.TitleBoost * 0.5
				}
			}
		}
	}

	// 2. Exact/Prefix matches on full title
	for id := uint32(0); id < uint32(len(ctx.Index.Items)); id++ {
		item := ctx.Index.Items[id]
		titleLower := strings.ToLower(item.Title)

		switch {
		case titleLower == queryLower:
			opts.Scores[id] += opts.Ranking.TitleBoost*2.0 + tier1ExactFloor
		case strings.HasPrefix(titleLower, queryLower):
			opts.Scores[id] += opts.Ranking.TitleBoost*0.8 + tier2PrefixFloor
		case strings.Contains(titleLower, queryLower):
			opts.Scores[id] += opts.Ranking.TitleBoost*0.3 + tier3WordFloor
		}

		descLower := strings.ToLower(item.Description)
		if strings.Contains(descLower, queryLower) {
			opts.Scores[id] += opts.Ranking.DescriptionBoost
		}
	}
}

// BoostScorer boosts scores based on exact tag matches
type BoostScorer struct{}

// Score boosts results that match the query or tag filter exactly.
func (scorer *BoostScorer) Score(ctx *Context, opts *ScoringOptions) {
	if ctx.OriginalQuery == "" && ctx.TagFilter == "" {
		return
	}

	queryLower := strings.ToLower(ctx.OriginalQuery)
	tagLower := strings.ToLower(ctx.TagFilter)

	for id := range opts.Scores {
		item := ctx.Index.Items[id]
		for _, terms := range item.NormalizedTaxs {
			for _, term := range terms {
				if term == queryLower || term == tagLower {
					opts.Scores[id] += opts.Ranking.TagBoost * exactTagBoostFactor
					opts.HighlightTerms[term] = true
				}
			}
		}
	}
}

// ScorePhraseMatch is the base boost for phrase matches.
const ScorePhraseMatch = 15.0

// RecencyScorer boosts scores for newer documents
type RecencyScorer struct{}

// Score boosts scores for newer documents based on their publication date.
func (scorer *RecencyScorer) Score(ctx *Context, opts *ScoringOptions) {
	nowUnix := core.NowUnix()
	for id, score := range opts.Scores {
		item := ctx.Index.Items[id]
		if item.Date == 0 {
			continue
		}

		ageMonths := float64(nowUnix-item.Date) / float64(secondsPerMonth)
		if ageMonths < 0 {
			ageMonths = 0
		}

		boost := recencyBaseBoost + recencyWeight*math.Exp(-recencyLambda*ageMonths)
		opts.Scores[id] = score * boost
	}
}

// FilterScorer enforces required and excluded terms
type FilterScorer struct{}

// Score removes documents that do not contain required terms or do contain excluded terms.
func (scorer *FilterScorer) Score(ctx *Context, opts *ScoringOptions) {
	for id := range opts.Scores {
		for _, info := range opts.TermInfos {
			termIdx := ctx.Index.LookupTerm(info.Term)
			hasTerm := false
			if termIdx >= 0 {
				docIDs, _ := ctx.Index.GetPostings(termIdx)
				absIDs := models.DecodeDocIDs(docIDs)
				hasTerm = slices.Contains(absIDs, id)
			}

			if info.Required && !hasTerm {
				delete(opts.Scores, id)
				break
			}
			if info.Excluded && hasTerm {
				delete(opts.Scores, id)
				break
			}
		}
	}
}

// ProximityScorer rewards documents where query terms appear close to each other
type ProximityScorer struct{}

// Score rewards documents where query terms appear in close proximity.
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

func (scorer *ProximityScorer) calculateProximityScore(ctx *Context, id uint32) float64 {
	termPositions := make([][]int, 0, len(ctx.QueryTerms))
	for _, term := range ctx.QueryTerms {
		termIdx := ctx.Index.LookupTerm(term)
		if termIdx < 0 {
			continue
		}

		docIDs, _ := ctx.Index.GetPostings(termIdx)
		absIDs := models.DecodeDocIDs(docIDs)
		idx := -1
		for i, aid := range absIDs {
			if aid == id {
				idx = i
				break
			}
		}
		if idx >= 0 {
			startIdx := ctx.Index.PostingOffsets[termIdx]
			postingIdx := int(startIdx) + idx
			pStart := ctx.Index.DocPosOffsets[postingIdx]
			pEnd := ctx.Index.DocPosOffsets[postingIdx+1]
			posDeltas := ctx.Index.Positions[pStart:pEnd]
			termPositions = append(termPositions, models.DecodePositions(posDeltas))
		}
	}

	if len(termPositions) < 2 {
		return 0
	}

	var boost float64
	for i := 0; i < len(termPositions); i++ {
		for j := i + 1; j < len(termPositions); j++ {
			p1, p2 := 0, 0
			l1, l2 := termPositions[i], termPositions[j]
			for p1 < len(l1) && p2 < len(l2) {
				diff := l1[p1] - l2[p2]
				if diff < 0 {
					diff = -diff
				}
				switch {
				case diff <= maxProximitySlop:
					boost += proximityBoost / float64(diff+1)
					p1++
					p2++
				case l1[p1] < l2[p2]:
					p1++
				default:
					p2++
				}
			}
		}
	}
	return boost
}

// Pipeline orchestrates multiple scorers
type Pipeline struct {
	scorers []Scorer
}

// NewPipeline creates a new scoring pipeline with the provided scorers.
func NewPipeline(scorers ...Scorer) *Pipeline {
	return &Pipeline{scorers: scorers}
}

// Execute runs all scorers in the pipeline.
func (pipeline *Pipeline) Execute(ctx *Context, opts *ScoringOptions) {
	for _, scorer := range pipeline.scorers {
		scorer.Score(ctx, opts)
	}
}

// Helpers for phrase matching
func checkPhraseUnified(index *models.SearchIndex, docID uint32, phraseTerms []string) bool {
	if len(phraseTerms) == 0 {
		return false
	}

	positions, ok := getDocPositions(index, docID, phraseTerms[0])
	if !ok {
		return false
	}

	for i := 1; i < len(phraseTerms); i++ {
		nextPos, ok := getDocPositions(index, docID, phraseTerms[i])
		if !ok {
			return false
		}
		positions = intersectPositions(positions, nextPos)
		if len(positions) == 0 {
			return false
		}
	}
	return true
}

func getDocPositions(index *models.SearchIndex, docID uint32, term string) ([]int, bool) {
	termIdx := index.LookupTerm(term)
	if termIdx < 0 {
		return nil, false
	}

	docIDs, _ := index.GetPostings(termIdx)
	absIDs := models.DecodeDocIDs(docIDs)
	for i, aid := range absIDs {
		if aid == docID {
			startIdx := index.PostingOffsets[termIdx]
			postingIdx := int(startIdx) + i
			pStart := index.DocPosOffsets[postingIdx]
			pEnd := index.DocPosOffsets[postingIdx+1]
			return models.DecodePositions(index.Positions[pStart:pEnd]), true
		}
	}
	return nil, false
}

func intersectPositions(posList, next []int) []int {
	var res []int
	i, j := 0, 0
	for i < len(posList) && j < len(next) {
		switch {
		case next[j] == posList[i]+1:
			res = append(res, next[j])
			i++
			j++
		case next[j] < posList[i]+1:
			j++
		default:
			i++
		}
	}
	return res
}
