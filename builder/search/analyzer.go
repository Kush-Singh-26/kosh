package search

import (
	"strings"
	"unicode"
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
	tokens, mapping, _ := a.AnalyzeWithPositions(text)
	return tokens, mapping
}

// AnalyzeWithPositions processes text and returns tokens, mapping, and positional data
func (a *Analyzer) AnalyzeWithPositions(text string) ([]string, map[string]string, map[string][]int) {
	tokens := TokenizeWithUnicode(text)
	result := make([]string, 0, len(tokens))
	mapping := make(map[string]string)
	positions := make(map[string][]int)

	idx := 0
	for _, token := range tokens {
		orig := strings.ToLower(token)
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
			if stem != "" {
				mapping[orig] = stem
			}
		}

		if stem != "" {
			result = append(result, stem)
			positions[stem] = append(positions[stem], idx)
		}
		idx++
	}
	return result, mapping, positions
}

// AnalyzeWithOriginals returns both stemmed and original forms
// This enables fuzzy matching on original forms while using stemmed forms for indexing
func (a *Analyzer) AnalyzeWithOriginals(text string) (stemmed []string, originals []string) {
	tokens := TokenizeWithUnicode(text)

	for _, token := range tokens {
		token = strings.ToLower(token)
		if len(token) < 2 {
			continue
		}
		if a.useStopWords && stopWords[token] {
			continue
		}

		originals = append(originals, token)

		if a.useStemming {
			stemmed = append(stemmed, StemCached(token))
		} else {
			stemmed = append(stemmed, token)
		}
	}
	return stemmed, originals
}

// Tokenize splits text into tokens (deprecated: use TokenizeWithUnicode)
func Tokenize(text string) []string {
	return TokenizeWithUnicode(text)
}

// TokenizeWithUnicode splits text into tokens with Unicode support
func TokenizeWithUnicode(text string) []string {
	if len(text) == 0 {
		return nil
	}

	estimatedTokens := len(text) / 5
	if estimatedTokens < 8 {
		estimatedTokens = 8
	}
	tokens := make([]string, 0, estimatedTokens)

	var buf strings.Builder
	buf.Grow(32)

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			buf.WriteRune(r)
		} else if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}

	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}

	return tokens
}

// IsStopWord checks if a word is a stop word
func IsStopWord(word string) bool {
	return stopWords[strings.ToLower(word)]
}
