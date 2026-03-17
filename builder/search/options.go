package search

// SearchScoringOptions defines parameters for the BM25 scoring engine
type SearchScoringOptions struct {
	TagFilter      string
	VersionFilter  string
	QueryTerms     []string
	HighlightTerms map[string]bool
	Scores         map[string]float64
	K1             float64
	B              float64
	Modifier       float64
}

// SearchScoringResult holds the output of a scoring pass
type SearchScoringResult struct {
	Scores         map[string]float64
	HighlightTerms []string
}
