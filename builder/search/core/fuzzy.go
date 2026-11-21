package core

import (
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	intSlicePoolCap      = 32
	runeSlicePoolCap     = 32
	maxPooledSliceCap    = 256
	minPrefixMatchLength = 1
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

// FuzzyExpand generates candidate terms for fuzzy matching from a flat Lexicon.
func FuzzyExpand(term string, lexicon []string, maxDist int) []string {
	var candidates []string
	termLen := utf8.RuneCountInString(term)

	for _, idxTerm := range lexicon {
		idxLen := utf8.RuneCountInString(idxTerm)

		// Length-based filtering: skip impossible matches early
		if idxLen < termLen-maxDist || idxLen > termLen+maxDist {
			// Special case: if we want prefix matches as well, we shouldn't continue here
			// but usually PerformSearch calls PrefixExpand separately or expects it here.
			// Let's keep it focused on edit distance.
			if termLen >= minPrefixMatchLength && strings.HasPrefix(idxTerm, term) {
				candidates = append(candidates, idxTerm)
				continue
			}
			continue
		}

		if FuzzyMatch(term, idxTerm, maxDist) {
			candidates = append(candidates, idxTerm)
		}
	}

	return candidates
}

// PrefixExpand finds all terms in the sorted lexicon that start with query.
func PrefixExpand(query string, lexicon []string) []string {
	if query == "" {
		return nil
	}
	idx := sort.SearchStrings(lexicon, query)
	var results []string
	for i := idx; i < len(lexicon); i++ {
		if !strings.HasPrefix(lexicon[i], query) {
			break
		}
		results = append(results, lexicon[i])
	}
	return results
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
