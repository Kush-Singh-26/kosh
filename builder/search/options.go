package search

import (
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

// ScoringOptions defines parameters for the BM25 scoring engine.
type ScoringOptions struct {
	TagFilter      string
	QueryTerms     []string
	HighlightTerms map[string]bool
	Scores         map[uint32]float64
	TermInfos      []core.QueryTerm
	K1             float64
	B              float64
	Modifier       float64
	Ranking        models.SearchRankingConfig
}

// ScoringResult holds the output of a scoring pass.
type ScoringResult struct {
	Scores         map[uint32]float64
	HighlightTerms []string
}
