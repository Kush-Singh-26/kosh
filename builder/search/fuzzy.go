package search

import (
	"strings"
)

// MaxEditDistance is the maximum Levenshtein distance for fuzzy matching
const MaxEditDistance = 2

// LevenshteinDistance calculates the edit distance between two strings
func LevenshteinDistance(a, b string) int {
	aRunes := []rune(a)
	bRunes := []rune(b)

	lenA := len(aRunes)
	lenB := len(bRunes)

	// Quick exit for empty strings
	if lenA == 0 {
		return lenB
	}
	if lenB == 0 {
		return lenA
	}

	// Use single slice optimization
	// We only need to track the previous row
	prev := make([]int, lenB+1)
	curr := make([]int, lenB+1)

	// Initialize first row
	for j := 0; j <= lenB; j++ {
		prev[j] = j
	}

	for i := 1; i <= lenA; i++ {
		curr[0] = i

		for j := 1; j <= lenB; j++ {
			cost := 1
			if aRunes[i-1] == bRunes[j-1] {
				cost = 0
			}

			// Minimum of insert, delete, replace
			insert := curr[j-1] + 1
			delete := prev[j] + 1
			replace := prev[j-1] + cost

			curr[j] = min3(insert, delete, replace)
		}

		// Swap slices
		prev, curr = curr, prev
	}

	return prev[lenB]
}

// FuzzyMatch checks if two strings match within maxDist edit distance
func FuzzyMatch(term, target string, maxDist int) bool {
	// Quick length check - if length difference > maxDist, can't match
	diff := len(term) - len(target)
	if diff < 0 {
		diff = -diff
	}
	if diff > maxDist {
		return false
	}

	return LevenshteinDistance(term, target) <= maxDist
}

// FuzzyExpand generates candidate terms for fuzzy matching
// Returns terms within the inverted index that are similar to the input
func FuzzyExpand(term string, inverted map[string]map[int]int, maxDist int) []string {
	var candidates []string

	for idxTerm := range inverted {
		if FuzzyMatch(term, idxTerm, maxDist) {
			candidates = append(candidates, idxTerm)
		}
	}

	return candidates
}

// FuzzyExpandWithNgrams uses n-gram index for faster fuzzy candidate generation
func FuzzyExpandWithNgrams(term string, ngramIndex map[string][]string, maxDist int) []string {
	// Generate trigrams for the term
	trigrams := generateTrigrams(term)

	// Count how many trigrams each candidate shares
	candidateScores := make(map[string]int)
	for _, tg := range trigrams {
		if candidates, ok := ngramIndex[tg]; ok {
			for _, cand := range candidates {
				candidateScores[cand]++
			}
		}
	}

	// Filter candidates by edit distance
	var results []string
	for cand, score := range candidateScores {
		// Jaccard-like filtering: need at least some overlap
		minScore := len(trigrams) / 2
		if score >= minScore {
			if FuzzyMatch(term, cand, maxDist) {
				results = append(results, cand)
			}
		}
	}

	return results
}

// generateTrigrams creates trigram (3-character) sequences from a word
func generateTrigrams(word string) []string {
	if len(word) < 3 {
		return []string{word}
	}

	runes := []rune(word)
	n := len(runes)
	trigrams := make([]string, 0, n-2)

	for i := 0; i <= n-3; i++ {
		trigrams = append(trigrams, string(runes[i:i+3]))
	}

	return trigrams
}

// BuildNgramIndex builds a trigram index for fast fuzzy lookups
func BuildNgramIndex(inverted map[string]map[int]int) map[string][]string {
	ngramIndex := make(map[string][]string)

	for term := range inverted {
		trigrams := generateTrigrams(term)
		for _, tg := range trigrams {
			ngramIndex[tg] = append(ngramIndex[tg], term)
		}
	}

	return ngramIndex
}

// min3 returns the minimum of three integers
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// ParseQuery parses a search query into terms and phrases
// Phrases are enclosed in quotes: "machine learning"
type ParsedQuery struct {
	Terms   []string // Individual terms
	Phrases []string // Quoted phrases
	Raw     string   // Original query
}

// ParseQuery extracts terms and phrases from a query string
func ParseQuery(query string) ParsedQuery {
	result := ParsedQuery{
		Raw: query,
	}

	// Extract phrases (quoted strings)
	var phraseBuf strings.Builder
	inPhrase := false

	for _, r := range query {
		if r == '"' {
			if inPhrase {
				// End phrase
				phrase := strings.TrimSpace(phraseBuf.String())
				if phrase != "" {
					result.Phrases = append(result.Phrases, strings.ToLower(phrase))
				}
				phraseBuf.Reset()
			}
			inPhrase = !inPhrase
		} else if inPhrase {
			phraseBuf.WriteRune(r)
		}
	}

	// Remove quoted phrases from query to extract individual terms
	cleaned := query
	for _, phrase := range result.Phrases {
		cleaned = strings.ReplaceAll(cleaned, `"`+phrase+`"`, " ")
	}

	// Tokenize remaining terms
	result.Terms = DefaultAnalyzer.Analyze(cleaned)

	return result
}
