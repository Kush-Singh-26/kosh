package core

import (
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Porter Stemmer implementation for English
// Based on the Porter Stemming Algorithm: https://tartarus.org/martin/PorterStemmer/

// stemCache caches stemmed words to avoid redundant computation
// Using LRU cache to prevent unbounded memory growth
var (
	stemCache     *lru.Cache[string, string]
	stemCacheOnce sync.Once
	stemCacheErr  error
)

// InitStemCache initializes the stemmer cache.
// It is called automatically on first use, but can be called explicitly
// to handle initialization errors.
func InitStemCache() error {
	stemCacheOnce.Do(func() {
		stemCache, stemCacheErr = lru.New[string, string](10000)
	})
	return stemCacheErr
}

// StemCached returns the stemmed form of word, using a cache for efficiency
func StemCached(word string) string {
	if len(word) <= 2 {
		return word
	}

	// Ensure cache is initialized
	if err := InitStemCache(); err != nil {
		return stem(word) // Fallback to uncached if initialization failed
	}

	// Check cache first
	if cached, ok := stemCache.Get(word); ok {
		return cached
	}

	// Compute and cache
	result := stem(word)
	stemCache.Add(word, result)
	return result
}

// Stem applies the Porter stemming algorithm to a word (uncached version)
func Stem(word string) string {
	return stem(word)
}

// stem is the internal stemming implementation
func stem(word string) string {
	if len(word) <= 2 {
		return word
	}

	// Convert to rune slice for manipulation
	runes := []rune(word)

	// Step 1a: Handle plurals and past participles
	runes = step1a(runes)

	// Step 1b: Handle -eed, -ed, -ing
	runes = step1b(runes)

	// Step 1c: Handle -y
	runes = step1c(runes)

	// Step 2: Map double suffixes to single ones
	runes = step2(runes)

	// Step 3: Handle -ative, -ful, etc.
	runes = step3(runes)

	// Step 4: Handle -ance, -ence, etc.
	runes = step4(runes)

	// Step 5a: Remove final -e
	runes = step5a(runes)

	// Step 5b: Remove double consonants
	runes = step5b(runes)

	return string(runes)
}

// measure returns the number of consonant sequences (VC) in the word
func measure(runes []rune) int {
	m := 0
	i := 0
	n := len(runes)

	// Skip initial consonants
	for i < n && !isVowel(runes, i) {
		i++
	}

	for i < n {
		// Count vowels
		for i < n && isVowel(runes, i) {
			i++
		}
		if i >= n {
			break
		}
		// Count consonants
		for i < n && !isVowel(runes, i) {
			i++
		}
		m++
	}

	return m
}

// isVowel checks if the character at position i is a vowel
func isVowel(runes []rune, i int) bool {
	n := len(runes)
	if i >= n {
		return false
	}

	c := runes[i]
	if c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u' {
		return true
	}
	if c == 'y' && i > 0 && !isVowel(runes, i-1) {
		return true
	}
	return false
}

// hasVowel checks if the stem contains a vowel
func hasVowel(runes []rune) bool {
	for i := range runes {
		if isVowel(runes, i) {
			return true
		}
	}
	return false
}

// endsWithDoubleConsonant checks for double consonant ending
func endsWithDoubleConsonant(runes []rune) bool {
	n := len(runes)
	if n < 2 {
		return false
	}
	if runes[n-1] != runes[n-2] {
		return false
	}
	return !isVowel(runes, n-1)
}

// endsWithCVC checks for consonant-vowel-consonant ending
func endsWithCVC(runes []rune) bool {
	n := len(runes)
	if n < 3 {
		return false
	}

	if isVowel(runes, n-1) || !isVowel(runes, n-2) || isVowel(runes, n-3) {
		return false
	}

	// The final consonant must not be w, x, or y
	c := runes[n-1]
	return c != 'w' && c != 'x' && c != 'y'
}

func step1a(runes []rune) []rune {
	n := len(runes)
	// Compare runes directly without string conversion
	if n >= 4 && runes[n-4] == 's' && runes[n-3] == 's' && runes[n-2] == 'e' && runes[n-1] == 's' {
		return append(runes[:n-2], 's')
	}
	if n >= 3 && runes[n-3] == 'i' && runes[n-2] == 'e' && runes[n-1] == 's' {
		return append(runes[:n-2], 'i')
	}
	if n >= 2 && runes[n-2] == 's' && runes[n-1] == 's' {
		return runes
	}
	if n >= 1 && runes[n-1] == 's' {
		return runes[:n-1]
	}
	return runes
}

func step1b(runes []rune) []rune {
	n := len(runes)

	// -eed (compare runes directly)
	if n >= 4 && runes[n-4] == 'e' && runes[n-3] == 'e' && runes[n-2] == 'd' {
		stem := runes[:n-3]
		if measure(stem) > 0 {
			return append(stem, 'e', 'e')
		}
		return runes
	}

	// -ed (compare runes directly)
	if n >= 3 && runes[n-3] == 'e' && runes[n-2] == 'd' {
		stem := runes[:n-2]
		if hasVowel(stem) {
			runes = stem
			return step1bHelper(runes)
		}
		return runes
	}

	// -ing (compare runes directly)
	if n >= 4 && runes[n-4] == 'i' && runes[n-3] == 'n' && runes[n-2] == 'g' {
		stem := runes[:n-3]
		if hasVowel(stem) {
			runes = stem
			return step1bHelper(runes)
		}
		return runes
	}

	return runes
}

// hasSuffix checks if the rune slice ends with the given string suffix
func hasSuffix(runes []rune, suffix string) bool {
	n := len(runes)
	sLen := len(suffix)
	if n < sLen {
		return false
	}
	// Fast ASCII path: all Porter stemmer suffixes are ASCII, so rune index == byte index
	for i := range sLen {
		if runes[n-sLen+i] != rune(suffix[i]) {
			return false
		}
	}
	return true
}

func step1bHelper(runes []rune) []rune {
	n := len(runes)

	// -at, -bl, -iz -> add 'e'
	if n >= 2 {
		if (runes[n-2] == 'a' && runes[n-1] == 't') ||
			(runes[n-2] == 'b' && runes[n-1] == 'l') ||
			(runes[n-2] == 'i' && runes[n-1] == 'z') {
			return append(runes, 'e')
		}
	}

	// Double consonant -> single
	if endsWithDoubleConsonant(runes) && runes[len(runes)-1] != 'l' && runes[len(runes)-1] != 's' && runes[len(runes)-1] != 'z' {
		return runes[:len(runes)-1]
	}

	// m=1 and ends with CVC -> add 'e'
	if measure(runes) == 1 && endsWithCVC(runes) {
		return append(runes, 'e')
	}

	return runes
}

func step1c(runes []rune) []rune {
	n := len(runes)
	if n >= 1 && runes[n-1] == 'y' {
		stem := runes[:n-1]
		if hasVowel(stem) {
			return append(stem, 'i')
		}
	}
	return runes
}

func step2(runes []rune) []rune {
	n := len(runes)

	suffixes := []struct {
		suffix      string
		replacement string
	}{
		{"ational", "ate"}, {"tional", "tion"}, {"enci", "ence"}, {"anci", "ance"},
		{"izer", "ize"}, {"abli", "able"}, {"alli", "al"}, {"entli", "ent"},
		{"eli", "e"}, {"ousli", "ous"}, {"ization", "ize"}, {"ation", "ate"},
		{"ator", "ate"}, {"alism", "al"}, {"iveness", "ive"}, {"fulness", "ful"},
		{"ousness", "ous"}, {"aliti", "al"}, {"iviti", "ive"}, {"biliti", "ble"},
	}

	for _, s := range suffixes {
		if hasSuffix(runes, s.suffix) {
			stem := runes[:n-len(s.suffix)]
			if measure(stem) > 0 {
				return append(stem, []rune(s.replacement)...)
			}
			return runes
		}
	}

	return runes
}

func step3(runes []rune) []rune {
	n := len(runes)

	suffixes := []struct {
		suffix      string
		replacement string
	}{
		{"icate", "ic"}, {"ative", ""}, {"alize", "al"}, {"iciti", "ic"},
		{"ical", "ic"}, {"ful", ""}, {"ness", ""},
	}

	for _, s := range suffixes {
		if hasSuffix(runes, s.suffix) {
			stem := runes[:n-len(s.suffix)]
			if measure(stem) > 0 {
				return append(stem, []rune(s.replacement)...)
			}
			return runes
		}
	}

	return runes
}

func step4(runes []rune) []rune {
	n := len(runes)

	suffixes := []string{
		"al", "ance", "ence", "er", "ic", "able", "ible", "ant", "ement",
		"ment", "ent", "ion", "ou", "ism", "ate", "iti", "ous", "ive", "ize",
	}

	for _, suffix := range suffixes {
		if hasSuffix(runes, suffix) {
			slen := len(suffix)
			stem := runes[:n-slen]
			m := measure(stem)

			// Special case for -ion: stem must end with s or t
			if suffix == "ion" {
				if len(stem) > 0 && (stem[len(stem)-1] == 's' || stem[len(stem)-1] == 't') && m > 1 {
					return stem
				}
			} else if m > 1 {
				return stem
			}
			return runes
		}
	}

	return runes
}

func step5a(runes []rune) []rune {
	n := len(runes)
	if n >= 1 && runes[n-1] == 'e' {
		stem := runes[:n-1]
		m := measure(stem)

		if m > 1 {
			return stem
		}
		if m == 1 && !endsWithCVC(stem) {
			return stem
		}
	}
	return runes
}

func step5b(runes []rune) []rune {
	n := len(runes)
	if n >= 2 && runes[n-1] == 'l' && runes[n-2] == 'l' && measure(runes) > 1 {
		return runes[:n-1]
	}
	return runes
}
