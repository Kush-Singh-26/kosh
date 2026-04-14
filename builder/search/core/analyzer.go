package core

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Kush-Singh-26/kosh/builder/pools"
)

const (
	tokenPoolCap           = 512
	minTokenLength         = 2
	estUniqueDivisor       = 4
	minEstUnique           = 4
	positionsCap           = 1
	offsetsCap             = 2
	localStemCacheDivisor  = 4
	asciiWordTableSize     = 256
	estimatedTokensDivisor = 5
	minEstimatedTokens     = 8
	asciiBoundary          = 0x80
)

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "for": true, "if": true, "in": true,
	"into": true, "is": true, "it": true, "no": true, "not": true, "of": true,
	"on": true, "or": true, "such": true, "that": true, "the": true, "their": true,
	"then": true, "there": true, "these": true, "they": true, "this": true,
	"to": true, "was": true, "will": true, "with": true, "have": true, "has": true,
	"had": true, "been": true, "being": true, "from": true, "were": true,
	"what": true, "when": true, "where": true, "which": true, "who": true,
	"whom": true, "why": true, "how": true, "all": true, "each": true,
	"every": true, "both": true, "few": true, "more": true, "most": true,
	"other": true, "some": true, "any": true, "only": true, "own": true,
	"same": true, "so": true, "than": true, "too": true, "very": true,
	"can": true, "just": true, "should": true, "now": true, "also": true,
	"its": true, "about": true, "after": true, "before": true, "above": true,
	"below": true, "between": true, "under": true, "again": true, "further": true,
	"once": true, "here": true, "during": true, "out": true, "up": true,
	"down": true, "off": true, "over": true, "through": true, "because": true,
	"while": true, "until": true, "am": true, "i": true, "me": true, "my": true,
	"myself": true, "we": true, "our": true, "ours": true, "ourselves": true,
	"you": true, "your": true, "yours": true, "yourself": true, "yourselves": true,
	"he": true, "him": true, "his": true, "himself": true, "she": true,
	"her": true, "hers": true, "herself": true, "itself": true, "them": true,
	"themselves": true, "those": true,
	// Additional common stop words
	"do": true, "does": true, "did": true, "would": true, "could": true,
	"may": true, "might": true, "must": true, "shall": true, "need": true,
	"dare": true, "ought": true, "used": true, "nor": true,
}

// Analyzer provides text analysis for search indexing.
type Analyzer struct {
	useStopWords bool
	useStemming  bool
}

// NewAnalyzer creates an Analyzer with configurable stop-word and stemming behavior.
func NewAnalyzer(useStopWords, useStemming bool) *Analyzer {
	return &Analyzer{
		useStopWords: useStopWords,
		useStemming:  useStemming,
	}
}

// DefaultAnalyzer is the default analyzer with stop words and stemming enabled.
var DefaultAnalyzer = NewAnalyzer(true, true)

// Analyze processes text and returns normalized tokens
func (analyzer *Analyzer) Analyze(text string) []string {
	tokens, _ := analyzer.AnalyzeWithMapping(text)
	return tokens
}

// AnalyzeWithMapping processes text and returns tokens plus a word->stem mapping
func (analyzer *Analyzer) AnalyzeWithMapping(text string) ([]string, map[string]string) {
	tokens, mapping, _, _ := analyzer.AnalyzeWithPositions(text)
	return tokens, mapping
}

var (
	// tokenPool stores *[]Token buffers for tokenization.
	tokenPool = sync.Pool{
		New: func() any {
			tokens := make([]Token, 0, tokenPoolCap)
			return &tokens
		},
	}
)

// AnalyzeWithPositions processes text and returns tokens, mapping, and positional data including offsets
func (analyzer *Analyzer) AnalyzeWithPositions(text string) ([]string, map[string]string, map[string][]int, map[string][]int) {
	tokensPtr := tokenPool.Get().(*[]Token)
	tokens := TokenizeWithUnicodeInto(text, (*tokensPtr)[:0])
	defer func() {
		*tokensPtr = tokens
		tokenPool.Put(tokensPtr)
	}()

	if len(tokens) == 0 {
		return nil, nil, nil, nil
	}

	estUnique := max(len(tokens)/estUniqueDivisor, minEstUnique)
	result := make([]string, 0, len(tokens))
	mapping := make(map[string]string, estUnique)
	positions := make(map[string][]int, estUnique)
	offsets := make(map[string][]int, estUnique)
	localStemCache := make(map[string]string, estUnique/localStemCacheDivisor)

	bufPtr := pools.SharedByteSlicePool.Get()
	defer pools.SharedByteSlicePool.Put(bufPtr)

	tokenIndex := 0
	for _, token := range tokens {
		var original string
		if isLowerASCII(token.Value) {
			// Avoid cloning if it's a stop word
			if analyzer.useStopWords && stopWords[token.Value] {
				tokenIndex++
				continue
			}
			original = strings.Clone(token.Value)
		} else {
			lowered, hasUnicode := toLowerASCII(token.Value, *bufPtr)
			if hasUnicode {
				// strings.ToLower handles Unicode and returns a new allocation
				original = strings.ToLower(token.Value)
			} else {
				// Go compiler optimizes stopWords[string(lowered)] here
				if analyzer.useStopWords && stopWords[string(lowered)] {
					tokenIndex++
					continue
				}
				original = string(lowered)
			}
		}

		if len(original) < minTokenLength {
			tokenIndex++
			continue
		}
		if analyzer.useStopWords && stopWords[original] {
			tokenIndex++
			continue
		}

		stem := original
		if analyzer.useStemming {
			if cached, ok := localStemCache[original]; ok {
				stem = cached
			} else {
				// Stem already returns a new string allocation (string(runes))
				stem = Stem(original)
				localStemCache[original] = stem
			}
		}

		if stem != "" {
			result = append(result, stem)
			if positions[stem] == nil {
				positions[stem] = make([]int, 0, positionsCap)
				offsets[stem] = make([]int, 0, offsetsCap)
			}
			positions[stem] = append(positions[stem], tokenIndex)
			offsets[stem] = append(offsets[stem], token.Start, token.End)
			if analyzer.useStemming {
				mapping[original] = stem
			}
		}
		tokenIndex++
	}
	return result, mapping, positions, offsets
}

// isLowerASCII returns true if the string is already lowercase ASCII.
func isLowerASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		char := text[i]
		if char&0x80 != 0 || (char >= 'A' && char <= 'Z') {
			return false
		}
	}
	return true
}

// toLowerASCII attempts to lowercase a string into buf.
// Returns (result, hasUnicode)
func toLowerASCII(text string, buf []byte) ([]byte, bool) {
	buf = buf[:0]
	for i := 0; i < len(text); i++ {
		char := text[i]
		if char&0x80 != 0 {
			return nil, true
		}
		if char >= 'A' && char <= 'Z' {
			buf = append(buf, char|0x20)
		} else {
			buf = append(buf, char)
		}
	}
	return buf, false
}

// AnalyzeWithOriginals returns both stemmed and original forms
// This enables fuzzy matching on original forms while using stemmed forms for indexing
func (analyzer *Analyzer) AnalyzeWithOriginals(text string) ([]string, []string) {
	tokensPtr := tokenPool.Get().(*[]Token)
	tokens := TokenizeWithUnicodeInto(text, (*tokensPtr)[:0])
	defer func() {
		*tokensPtr = tokens
		tokenPool.Put(tokensPtr)
	}()

	bufPtr := pools.SharedByteSlicePool.Get()
	defer pools.SharedByteSlicePool.Put(bufPtr)

	var stemmed []string
	var originals []string
	for _, token := range tokens {
		var original string
		if isLowerASCII(token.Value) {
			original = token.Value
		} else {
			lowered, hasUnicode := toLowerASCII(token.Value, *bufPtr)
			if hasUnicode {
				original = strings.ToLower(token.Value)
			} else {
				original = string(lowered)
			}
		}

		if len(original) < minTokenLength {
			continue
		}
		if analyzer.useStopWords && stopWords[original] {
			continue
		}

		originals = append(originals, original)

		if analyzer.useStemming {
			stemmed = append(stemmed, StemCached(original))
		} else {
			stemmed = append(stemmed, original)
		}
	}
	return stemmed, originals
}

// Token represents a word with its byte offsets in the original text
type Token struct {
	Value string
	Start int
	End   int
}

var isWordPartASCII [asciiWordTableSize]bool

// init populates the ASCII word-part lookup table. This is a standard Go pattern
// for compile-time constant data — the table is read-only and shared across all
// tokenization calls. No external state is inspected; no side effects beyond
// populating an internal lookup array.
func init() {
	for i := range asciiWordTableSize {
		r := rune(i)
		isWordPartASCII[i] = (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
}

// TokenizeWithUnicode splits text into tokens with Unicode support and returns offsets
func TokenizeWithUnicode(text string) []Token {
	return TokenizeWithUnicodeInto(text, nil)
}

// TokenizeWithUnicodeInto splits text into tokens with Unicode support and appends them to dst
func TokenizeWithUnicodeInto(text string, dst []Token) []Token {
	if len(text) == 0 {
		return dst
	}

	if dst == nil {
		estimatedTokens := max(len(text)/estimatedTokensDivisor, minEstimatedTokens)
		dst = make([]Token, 0, estimatedTokens)
	}

	start := -1
	for i := 0; i < len(text); {
		char := text[i]
		var isWordPart bool
		var size int
		if char < asciiBoundary {
			isWordPart = isWordPartASCII[char]
			size = 1
		} else {
			charRune, sz := utf8.DecodeRuneInString(text[i:])
			isWordPart = unicode.IsLetter(charRune) || unicode.IsNumber(charRune)
			size = sz
		}

		if isWordPart {
			if start == -1 {
				start = i
			}
		} else {
			if start != -1 {
				dst = append(dst, Token{
					Value: text[start:i],
					Start: start,
					End:   i,
				})
				start = -1
			}
		}
		i += size
	}

	if start != -1 {
		dst = append(dst, Token{
			Value: text[start:],
			Start: start,
			End:   len(text),
		})
	}

	return dst
}

// IsStopWord checks if a word is a stop word
func IsStopWord(word string) bool {
	if isLowerASCII(word) {
		return stopWords[word]
	}
	return stopWords[strings.ToLower(word)]
}

// TruncateToLength truncates string text to maxLen and aligns to rune boundaries
func TruncateToLength(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	text = text[:maxLen]
	// Align to rune boundary to avoid invalid UTF-8
	for len(text) > 0 && !utf8.RuneStart(text[len(text)-1]) {
		text = text[:len(text)-1]
	}
	// If the last byte is the start of a multi-byte rune but we don't have the rest,
	// we should also trim it.
	char, size := utf8.DecodeLastRuneInString(text)
	if char == utf8.RuneError && size == 1 {
		text = text[:len(text)-1]
	}
	return text
}
