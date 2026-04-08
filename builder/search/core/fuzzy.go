package core

import (
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// MaxEditDistance is the maximum Levenshtein distance for fuzzy matching
const MaxEditDistance = 2

var intSlicePool = sync.Pool{
	New: func() any {
		s := make([]int, 0, 32)
		return &s
	},
}

func getIntSlice(size int) *[]int {
	p := intSlicePool.Get().(*[]int)
	if cap(*p) < size {
		*p = make([]int, size)
	} else {
		*p = (*p)[:size]
	}
	return p
}

func putIntSlice(p *[]int) {
	if cap(*p) > 256 {
		return
	}
	intSlicePool.Put(p)
}

var runeSlicePool = sync.Pool{
	New: func() any {
		s := make([]rune, 0, 32)
		return &s
	},
}

func getRuneSlice(size int) *[]rune {
	p := runeSlicePool.Get().(*[]rune)
	if cap(*p) < size {
		*p = make([]rune, size)
	} else {
		*p = (*p)[:size]
	}
	return p
}

func putRuneSlice(p *[]rune) {
	if cap(*p) > 256 {
		return
	}
	runeSlicePool.Put(p)
}

func stringToRunesPool(s string, p *[]rune) {
	buf := (*p)[:0]
	for _, r := range s {
		buf = append(buf, r)
	}
	*p = buf
}

// LevenshteinDistance calculates the edit distance between two strings
func LevenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}

	// We still need the rune count for the slices
	lenA := utf8.RuneCountInString(a)
	lenB := utf8.RuneCountInString(b)

	// Quick exit for empty strings
	if lenA == 0 {
		return lenB
	}
	if lenB == 0 {
		return lenA
	}

	aRunesPtr := getRuneSlice(lenA)
	bRunesPtr := getRuneSlice(lenB)
	stringToRunesPool(a, aRunesPtr)
	stringToRunesPool(b, bRunesPtr)

	aRunes := *aRunesPtr
	bRunes := *bRunesPtr

	defer func() {
		putRuneSlice(aRunesPtr)
		putRuneSlice(bRunesPtr)
	}()

	// Use single slice optimization
	// We only need to track the previous row
	prevPtr := getIntSlice(lenB + 1)
	currPtr := getIntSlice(lenB + 1)
	defer func() {
		putIntSlice(prevPtr)
		putIntSlice(currPtr)
	}()

	prev := *prevPtr
	curr := *currPtr
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
func FuzzyExpand(term string, inverted map[string]map[string][]uint32, maxDist int) []string {
	var candidates []string
	termLen := len([]rune(term))

	for idxTerm := range inverted {
		// Length-based filtering: skip impossible matches early
		idxLen := len([]rune(idxTerm))

		// 1. Live Prefix Matching (crucial for live search bars)
		// If user typed "prog" and index has "program", match it immediately
		if termLen >= 3 && strings.HasPrefix(idxTerm, term) {
			candidates = append(candidates, idxTerm)
			continue
		}

		// 2. Levenshtein Distance (Fuzzy match for typos)
		if idxLen < termLen-maxDist || idxLen > termLen+maxDist {
			continue
		}
		if FuzzyMatch(term, idxTerm, maxDist) {
			candidates = append(candidates, idxTerm)
		}
	}

	return candidates
}

// FuzzyExpandWithNgrams uses n-gram index for faster fuzzy candidate generation
func FuzzyExpandWithNgrams(term string, ngramIndex map[string][]string, maxDist int) []string {
	// Generate trigrams for the term
	trigrams := GenerateTrigrams(term)

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
		// Optimized: Set minimum score to 1 to avoid matching every term on short queries
		// A 3-character word generates exactly 1 trigram, so len/2 = 0 would match everything
		minScore := max(1, len(trigrams)/2)
		if score >= minScore {
			if FuzzyMatch(term, cand, maxDist) {
				results = append(results, cand)
			}
		}
	}

	return results
}

// GenerateTrigrams creates trigram (3-character) sequences from a word
// Uses a byte-slice approach for ASCII strings to reduce allocations
func GenerateTrigrams(word string) []string {
	n := len(word)

	// Fast path for ASCII strings (common case)
	if n < 3 {
		return []string{word}
	}

	// Check if ASCII
	isASCII := true
	for i := 0; i < n; i++ {
		if word[i] >= 128 {
			isASCII = false
			break
		}
	}

	if isASCII {
		// Fast path: use byte slices for ASCII
		trigrams := make([]string, 0, n-2)
		for i := 0; i <= n-3; i++ {
			trigrams = append(trigrams, word[i:i+3])
		}
		return trigrams
	}

	// Slow path: use runes for Unicode
	runes := []rune(word)
	n = len(runes)
	if n < 3 {
		return []string{word}
	}

	trigrams := make([]string, 0, n-2)
	for i := 0; i <= n-3; i++ {
		trigrams = append(trigrams, string(runes[i:i+3]))
	}
	return trigrams
}

// MaxNgramPostings is the threshold for n-gram pruning.
// Trigrams that map to more than this many terms are dropped.
const MaxNgramPostings = 50

// BuildNgramIndex builds a trigram index for fast fuzzy lookups with pruning
func BuildNgramIndex(inverted map[string]map[string][]uint32) map[string][]string {
	numWorkers := min(runtime.NumCPU(), 8)

	terms := make([]string, 0, len(inverted))
	for term := range inverted {
		terms = append(terms, term)
	}

	totalTerms := len(terms)
	chunkSize := (totalTerms + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	results := make([]map[string][]string, numWorkers)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			localNgram := make(map[string][]string)
			start := workerID * chunkSize
			end := min(start+chunkSize, totalTerms)

			for j := start; j < end; j++ {
				term := terms[j]
				trigrams := GenerateTrigrams(term)
				for _, tg := range trigrams {
					localNgram[tg] = append(localNgram[tg], term)
				}
			}
			results[workerID] = localNgram
		}(i)
	}
	wg.Wait()

	// Merge results with pruning
	ngramIndex := make(map[string][]string)
	for _, r := range results {
		for tg, termList := range r {
			ngramIndex[tg] = append(ngramIndex[tg], termList...)
		}
	}

	// Prune common trigrams that point to too many terms
	for tg, termList := range ngramIndex {
		if len(termList) > MaxNgramPostings {
			delete(ngramIndex, tg)
		}
	}

	return ngramIndex
}

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

// QueryTerm describes a query token with operators.
type QueryTerm struct {
	Term     string
	Required bool
	Excluded bool
}

// ParsedQuery holds normalized query terms and phrases.
type ParsedQuery struct {
	Terms     []string
	Phrases   [][]string
	Raw       string
	Required  []string
	Excluded  []string
	TermInfos []QueryTerm
}

// ParseQuery parses a search query into terms and phrases.
func ParseQuery(query string) ParsedQuery {
	result := ParsedQuery{
		Raw: query,
	}

	normalized := NormalizeNFC(query)
	normalized = ToLower(strings.TrimSpace(normalized))
	if normalized == "" {
		return result
	}

	var phraseBuf strings.Builder
	inPhrase := false
	var rawPhrases []string

	for _, r := range normalized {
		if r == '"' {
			if inPhrase {
				phrase := strings.TrimSpace(phraseBuf.String())
				if phrase != "" {
					rawPhrases = append(rawPhrases, phrase)
					tokens := DefaultAnalyzer.Analyze(phrase)
					if len(tokens) > 0 {
						result.Phrases = append(result.Phrases, tokens)
					}
				}
				phraseBuf.Reset()
			}
			inPhrase = !inPhrase
		} else if inPhrase {
			phraseBuf.WriteRune(r)
		}
	}

	cleaned := normalized
	for _, phrase := range rawPhrases {
		cleaned = strings.ReplaceAll(cleaned, `"`+phrase+`"`, " ")
	}

	result.Terms, result.TermInfos = parseOperators(cleaned)

	return result
}

func parseOperators(text string) ([]string, []QueryTerm) {
	var terms []string
	var termInfos []QueryTerm
	var current strings.Builder
	inOperator := false

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				term, op := extractOperator(current.String())
				switch op {
				case 1:
					result := processTerm(term)
					if result != "" {
						result = StemCached(result)
						terms = append(terms, result)
						termInfos = append(termInfos, QueryTerm{Term: result, Required: true})
					}
				case 2:
					result := processTerm(term)
					if result != "" {
						result = StemCached(result)
						terms = append(terms, result)
						termInfos = append(termInfos, QueryTerm{Term: result, Excluded: true})
					}
				default:
					result := processTerm(term)
					if result != "" {
						result = StemCached(result)
						terms = append(terms, result)
						termInfos = append(termInfos, QueryTerm{Term: result})
					}
				}
				current.Reset()
				inOperator = false
			}
			continue
		}

		if r == '+' || r == '-' {
			if current.Len() > 0 && !inOperator {
				term, op := extractOperator(current.String())
				if op == 0 {
					result := processTerm(term)
					if result != "" {
						result = StemCached(result)
						terms = append(terms, result)
						termInfos = append(termInfos, QueryTerm{Term: result})
					}
				}
				current.Reset()
			}
			inOperator = true
			current.WriteRune(r)
			continue
		}

		inOperator = false
		current.WriteRune(r)
	}

	if current.Len() > 0 {
		term, op := extractOperator(current.String())
		switch op {
		case 1:
			result := processTerm(term)
			if result != "" {
				result = StemCached(result)
				terms = append(terms, result)
				termInfos = append(termInfos, QueryTerm{Term: result, Required: true})
			}
		case 2:
			result := processTerm(term)
			if result != "" {
				result = StemCached(result)
				terms = append(terms, result)
				termInfos = append(termInfos, QueryTerm{Term: result, Excluded: true})
			}
		default:
			result := processTerm(term)
			if result != "" {
				result = StemCached(result)
				terms = append(terms, result)
				termInfos = append(termInfos, QueryTerm{Term: result})
			}
		}
	}

	return terms, termInfos
}

func extractOperator(term string) (string, int) {
	if len(term) > 0 {
		if term[0] == '+' {
			return strings.TrimPrefix(term, "+"), 1
		}
		if term[0] == '-' {
			return strings.TrimPrefix(term, "-"), 2
		}
	}
	return term, 0
}

func processTerm(term string) string {
	term = strings.TrimSpace(term)
	if len(term) < 2 {
		return ""
	}
	if IsStopWord(term) {
		return ""
	}
	return term
}
