package search

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// English stop words - common words that don't contribute to search relevance
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

// Analyzer provides text analysis for search indexing
type Analyzer struct {
	useStopWords bool
	useStemming  bool
}

// NewAnalyzer creates a new analyzer with specified options
func NewAnalyzer(useStopWords, useStemming bool) *Analyzer {
	return &Analyzer{
		useStopWords: useStopWords,
		useStemming:  useStemming,
	}
}

// DefaultAnalyzer is the default analyzer with stemming and stop words enabled
var DefaultAnalyzer = NewAnalyzer(true, true)

// Analyze processes text and returns normalized tokens
func (a *Analyzer) Analyze(text string) []string {
	tokens, _ := a.AnalyzeWithMapping(text)
	return tokens
}

// AnalyzeWithMapping processes text and returns tokens plus a word->stem mapping
func (a *Analyzer) AnalyzeWithMapping(text string) ([]string, map[string]string) {
	tokens, mapping, _, _ := a.AnalyzeWithPositions(text)
	return tokens, mapping
}

// AnalyzeWithPositions processes text and returns tokens, mapping, and positional data including offsets
func (a *Analyzer) AnalyzeWithPositions(text string) ([]string, map[string]string, map[string][]int, map[string][]int) {
	tokens := TokenizeWithUnicode(text)
	if len(tokens) == 0 {
		return nil, nil, nil, nil
	}

	estUnique := max(len(tokens)/2, 4)
	result := make([]string, 0, len(tokens))
	mapping := make(map[string]string, estUnique)
	positions := make(map[string][]int, estUnique)
	offsets := make(map[string][]int, estUnique)

	bufPtr := utils.SharedByteSlicePool.Get()
	defer utils.SharedByteSlicePool.Put(bufPtr)

	idx := 0
	for _, token := range tokens {
		var orig string
		if isLowerASCII(token.Value) {
			orig = token.Value
		} else {
			lowered, hasUnicode := toLowerASCII(token.Value, *bufPtr)
			if hasUnicode {
				orig = strings.ToLower(token.Value)
			} else {
				// We use string(lowered) here for map lookup.
				// Go compiler optimizes stopWords[string(lowered)] to avoid allocation.
				if a.useStopWords && stopWords[string(lowered)] {
					idx++
					continue
				}
				orig = string(lowered)
			}
		}

		if len(orig) < 2 {
			idx++
			continue
		}
		if a.useStopWords && stopWords[orig] {
			idx++
			continue
		}

		stem := orig
		if a.useStemming {
			stem = StemCached(orig)
		}

		if stem != "" {
			result = append(result, stem)
			if positions[stem] == nil {
				positions[stem] = make([]int, 0, 1)
				offsets[stem] = make([]int, 0, 2)
			}
			positions[stem] = append(positions[stem], idx)
			offsets[stem] = append(offsets[stem], token.Start, token.End)
			if a.useStemming {
				mapping[orig] = stem
			}
		}
		idx++
	}
	return result, mapping, positions, offsets
}

// isLowerASCII returns true if the string is already lowercase ASCII.
func isLowerASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c&0x80 != 0 || (c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// toLowerASCII attempts to lowercase a string into buf.
// Returns (result, hasUnicode)
func toLowerASCII(s string, buf []byte) ([]byte, bool) {
	buf = buf[:0]
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c&0x80 != 0 {
			return nil, true
		}
		if c >= 'A' && c <= 'Z' {
			buf = append(buf, c|0x20)
		} else {
			buf = append(buf, c)
		}
	}
	return buf, false
}

// AnalyzeWithOriginals returns both stemmed and original forms
// This enables fuzzy matching on original forms while using stemmed forms for indexing
func (a *Analyzer) AnalyzeWithOriginals(text string) (stemmed []string, originals []string) {
	tokens := TokenizeWithUnicode(text)

	bufPtr := utils.SharedByteSlicePool.Get()
	defer utils.SharedByteSlicePool.Put(bufPtr)

	for _, token := range tokens {
		var orig string
		if isLowerASCII(token.Value) {
			orig = token.Value
		} else {
			lowered, hasUnicode := toLowerASCII(token.Value, *bufPtr)
			if hasUnicode {
				orig = strings.ToLower(token.Value)
			} else {
				orig = string(lowered)
			}
		}

		if len(orig) < 2 {
			continue
		}
		if a.useStopWords && stopWords[orig] {
			continue
		}

		originals = append(originals, orig)

		if a.useStemming {
			stemmed = append(stemmed, StemCached(orig))
		} else {
			stemmed = append(stemmed, orig)
		}
	}
	return stemmed, originals
}

// Tokenize splits text into tokens (deprecated: use TokenizeWithUnicode)
func Tokenize(text string) []string {
	tokens := TokenizeWithUnicode(text)
	res := make([]string, len(tokens))
	for i, t := range tokens {
		res[i] = t.Value
	}
	return res
}

// Token represents a word with its byte offsets in the original text
type Token struct {
	Value string
	Start int
	End   int
}

var isWordPartASCII [256]bool

func init() {
	for i := range 256 {
		r := rune(i)
		isWordPartASCII[i] = (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
}

// TokenizeWithUnicode splits text into tokens with Unicode support and returns offsets
func TokenizeWithUnicode(text string) []Token {
	if len(text) == 0 {
		return nil
	}

	estimatedTokens := max(len(text)/5, 8)
	tokens := make([]Token, 0, estimatedTokens)

	start := -1
	for i := 0; i < len(text); {
		c := text[i]
		var isWordPart bool
		var size int
		if c < 0x80 {
			isWordPart = isWordPartASCII[c]
			size = 1
		} else {
			r, sz := utf8.DecodeRuneInString(text[i:])
			isWordPart = unicode.IsLetter(r) || unicode.IsNumber(r)
			size = sz
		}

		if isWordPart {
			if start == -1 {
				start = i
			}
		} else {
			if start != -1 {
				tokens = append(tokens, Token{
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
		tokens = append(tokens, Token{
			Value: text[start:],
			Start: start,
			End:   len(text),
		})
	}

	return tokens
}

// IsStopWord checks if a word is a stop word
func IsStopWord(word string) bool {
	if isLowerASCII(word) {
		return stopWords[word]
	}
	return stopWords[strings.ToLower(word)]
}
