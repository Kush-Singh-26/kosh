package core

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Kush-Singh-26/kosh/builder/async"
)

const (
	intSlicePoolCap      = 32
	runeSlicePoolCap     = 32
	maxPooledSliceCap    = 256
	minPrefixMatchLength = 3
	trigramSize          = 3
	maxNgramWorkers      = 8
	minCandidateScore    = 1
)

// MaxEditDistance is the maximum Levenshtein distance for fuzzy matching
const MaxEditDistance = 2

// intSlicePool stores *[]int buffers for Levenshtein computation.
var intSlicePool = sync.Pool{
	New: func() any {
		slice := make([]int, 0, intSlicePoolCap)
		return &slice
	},
}

func getIntSlice(size int) *[]int {
	ptr := intSlicePool.Get().(*[]int)
	if cap(*ptr) < size {
		*ptr = make([]int, size)
	} else {
		*ptr = (*ptr)[:size]
	}
	return ptr
}

func putIntSlice(ptr *[]int) {
	if cap(*ptr) > maxPooledSliceCap {
		return
	}
	intSlicePool.Put(ptr)
}

// runeSlicePool stores *[]rune buffers for Levenshtein computation.
var runeSlicePool = sync.Pool{
	New: func() any {
		runes := make([]rune, 0, runeSlicePoolCap)
		return &runes
	},
}

func getRuneSlice(size int) *[]rune {
	ptr := runeSlicePool.Get().(*[]rune)
	if cap(*ptr) < size {
		*ptr = make([]rune, size)
	} else {
		*ptr = (*ptr)[:size]
	}
	return ptr
}

func putRuneSlice(ptr *[]rune) {
	if cap(*ptr) > maxPooledSliceCap {
		return
	}
	runeSlicePool.Put(ptr)
}

func stringToRunesPool(text string, target *[]rune) {
	buf := (*target)[:0]
	for _, char := range text {
		buf = append(buf, char)
	}
	*target = buf
}

// LevenshteinDistance calculates the edit distance between two strings
func LevenshteinDistance(str1, str2 string) int {
	if str1 == str2 {
		return 0
	}

	// We still need the rune count for the slices
	len1 := utf8.RuneCountInString(str1)
	len2 := utf8.RuneCountInString(str2)

	// Quick exit for empty strings
	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	str1RunesPtr := getRuneSlice(len1)
	str2RunesPtr := getRuneSlice(len2)
	stringToRunesPool(str1, str1RunesPtr)
	stringToRunesPool(str2, str2RunesPtr)

	str1Runes := *str1RunesPtr
	str2Runes := *str2RunesPtr

	defer func() {
		putRuneSlice(str1RunesPtr)
		putRuneSlice(str2RunesPtr)
	}()

	// Use single slice optimization
	// We only need to track the previous row
	prevPtr := getIntSlice(len2 + 1)
	currPtr := getIntSlice(len2 + 1)
	defer func() {
		putIntSlice(prevPtr)
		putIntSlice(currPtr)
	}()

	prev := *prevPtr
	curr := *currPtr
	for j := 0; j <= len2; j++ {
		prev[j] = j
	}

	for i := 1; i <= len1; i++ {
		curr[0] = i

		for j := 1; j <= len2; j++ {
			cost := 1
			if str1Runes[i-1] == str2Runes[j-1] {
				cost = 0
			}

			// Minimum of insert, delete, replace
			insert := curr[j-1] + 1
			del := prev[j] + 1
			replace := prev[j-1] + cost

			curr[j] = min3(insert, del, replace)
		}

		// Swap slices
		prev, curr = curr, prev
	}

	return prev[len2]
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
		if termLen >= minPrefixMatchLength && strings.HasPrefix(idxTerm, term) {
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
	for _, trigram := range trigrams {
		if candidates, ok := ngramIndex[trigram]; ok {
			for _, candidate := range candidates {
				candidateScores[candidate]++
			}
		}
	}

	// Filter candidates by edit distance
	var results []string
	for candidate, score := range candidateScores {
		// Jaccard-like filtering: need at least some overlap
		// Optimized: Set minimum score to 1 to avoid matching every term on short queries
		// A 3-character word generates exactly 1 trigram, so len/2 = 0 would match everything
		minScore := max(minCandidateScore, len(trigrams)/2)
		if score >= minScore {
			if FuzzyMatch(term, candidate, maxDist) {
				results = append(results, candidate)
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
	if n < trigramSize {
		return []string{word}
	}

	// Check if ASCII
	isASCII := true
	for i := 0; i < n; i++ {
		if word[i] >= asciiBoundary {
			isASCII = false
			break
		}
	}

	if isASCII {
		// Fast path: use byte slices for ASCII
		trigrams := make([]string, 0, n-(trigramSize-1))
		for i := 0; i <= n-trigramSize; i++ {
			trigrams = append(trigrams, word[i:i+trigramSize])
		}
		return trigrams
	}

	// Slow path: use runes for Unicode
	runes := []rune(word)
	n = len(runes)
	if n < trigramSize {
		return []string{word}
	}

	trigrams := make([]string, 0, n-(trigramSize-1))
	for i := 0; i <= n-trigramSize; i++ {
		trigrams = append(trigrams, string(runes[i:i+trigramSize]))
	}
	return trigrams
}

// MaxNgramPostings is the threshold for n-gram pruning.
// Trigrams that map to more than this many terms are dropped.
const MaxNgramPostings = 50

// BuildNgramIndex builds a trigram index for fast fuzzy lookups with pruning
func BuildNgramIndex(inverted map[string]map[string][]uint32) map[string][]string {
	numWorkers := min(runtime.NumCPU(), maxNgramWorkers)

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
		workerID := i
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       context.Background(),
			Logger:    slog.Default(),
			Operation: "ngram build",
			Fn: func() error {
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
				return nil
			},
			Cleanup: wg.Done,
		})
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
