package core

import (
	"strings"
	"unicode"
)

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

	for _, char := range normalized {
		if char == '"' {
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
			phraseBuf.WriteRune(char)
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

	for _, char := range text {
		if unicode.IsSpace(char) {
			if current.Len() > 0 {
				term, operator := extractOperator(current.String())
				switch operator {
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

		if char == '+' || char == '-' {
			if current.Len() > 0 && !inOperator {
				term, operator := extractOperator(current.String())
				if operator == 0 {
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
			current.WriteRune(char)
			continue
		}

		inOperator = false
		current.WriteRune(char)
	}

	if current.Len() > 0 {
		term, operator := extractOperator(current.String())
		switch operator {
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
