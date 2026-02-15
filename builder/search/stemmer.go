package search

// Porter Stemmer implementation for English
// Based on the Porter Stemming Algorithm: https://tartarus.org/martin/PorterStemmer/

// Stem applies the Porter stemming algorithm to a word
func Stem(word string) string {
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
	for i := 0; i < len(runes); i++ {
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

// replaceSuffix replaces a suffix if the stem is long enough
func replaceSuffix(runes []rune, suffix, replacement string, minLength int) []rune {
	if len(runes) < len(suffix) {
		return runes
	}

	// Check if it ends with suffix
	suffixRunes := []rune(suffix)
	n := len(runes)
	match := true
	for i := 0; i < len(suffixRunes); i++ {
		if runes[n-len(suffixRunes)+i] != suffixRunes[i] {
			match = false
			break
		}
	}

	if !match {
		return runes
	}

	stem := runes[:n-len(suffixRunes)]
	if measure(stem) > minLength {
		return append(stem, []rune(replacement)...)
	}
	return runes
}

func step1a(runes []rune) []rune {
	n := len(runes)
	if n >= 4 && string(runes[n-4:]) == "sses" {
		return append(runes[:n-2], 's')
	}
	if n >= 3 && string(runes[n-3:]) == "ies" {
		return append(runes[:n-2], 'i')
	}
	if n >= 2 && string(runes[n-2:]) == "ss" {
		return runes
	}
	if n >= 1 && runes[n-1] == 's' {
		return runes[:n-1]
	}
	return runes
}

func step1b(runes []rune) []rune {
	n := len(runes)

	// -eed
	if n >= 4 && string(runes[n-4:]) == "eed" {
		stem := runes[:n-3]
		if measure(stem) > 0 {
			return append(stem, 'e', 'e')
		}
		return runes
	}

	// -ed
	if n >= 3 && string(runes[n-3:]) == "ed" {
		stem := runes[:n-2]
		if hasVowel(stem) {
			runes = stem
			return step1bHelper(runes)
		}
		return runes
	}

	// -ing
	if n >= 4 && string(runes[n-4:]) == "ing" {
		stem := runes[:n-3]
		if hasVowel(stem) {
			runes = stem
			return step1bHelper(runes)
		}
		return runes
	}

	return runes
}

func step1bHelper(runes []rune) []rune {
	n := len(runes)

	// -at, -bl, -iz -> add 'e'
	if n >= 2 {
		suffix := string(runes[n-2:])
		if suffix == "at" || suffix == "bl" || suffix == "iz" {
			return append(runes, 'e')
		}
	}

	// Double consonant -> single
	if endsWithDoubleConsonant(runes) && !(runes[len(runes)-1] == 'l' || runes[len(runes)-1] == 's' || runes[len(runes)-1] == 'z') {
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
		if n >= len(s.suffix) && string(runes[n-len(s.suffix):]) == s.suffix {
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
		if n >= len(s.suffix) && string(runes[n-len(s.suffix):]) == s.suffix {
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
		slen := len(suffix)
		if n >= slen && string(runes[n-slen:]) == suffix {
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
