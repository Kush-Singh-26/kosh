package search

import (
	"slices"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)


const (
	minSuggestionPrefixLen = 2
	defaultSuggestionCount = 1
	defaultSuggestionLimit = 8
)

type suggestion struct {
	term  string
	count int
}

// GetSuggestions returns a list of suggested terms based on a prefix
func GetSuggestions(index *searchpkg.SearchIndex, prefix string) []string {
	prefix = core.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < minSuggestionPrefixLen {
		return nil
	}

	var suggestions []suggestion
	
	// 1. Lexicon prefix match (fast binary search via core.PrefixExpand)
	prefixTerms := core.PrefixExpand(prefix, index.Terms)
	for _, term := range prefixTerms {
		termIdx := index.LookupTerm(term)
		if termIdx >= 0 {
			count := int(index.PostingOffsets[termIdx+1] - index.PostingOffsets[termIdx])
			suggestions = append(suggestions, suggestion{term: term, count: count})
		}
	}

	// 2. StemMap prefix match
	for stem, origins := range index.StemMap {
		if strings.HasPrefix(stem, prefix) {
			for _, origin := range origins {
				if !containsSuggestion(suggestions, origin) {
					suggestions = append(suggestions, suggestion{
						term:  origin,
						count: defaultSuggestionCount,
					})
				}
			}
		} else {
			for _, origin := range origins {
				if strings.HasPrefix(origin, prefix) && !containsSuggestion(suggestions, origin) {
					suggestions = append(suggestions, suggestion{
						term:  origin,
						count: defaultSuggestionCount,
					})
				}
			}
		}
	}

	return sortAndFinalizeSuggestions(suggestions)
}

func containsSuggestion(suggestions []suggestion, term string) bool {
	for _, sugg := range suggestions {
		if sugg.term == term {
			return true
		}
	}
	return false
}

func sortAndFinalizeSuggestions(suggestions []suggestion) []string {
	slices.SortFunc(suggestions, func(a, b suggestion) int {
		if a.count != b.count {
			return b.count - a.count
		}
		return len(a.term) - len(b.term)
	})

	limit := defaultSuggestionLimit
	result := make([]string, 0, min(len(suggestions), limit))
	seen := make(map[string]bool)
	for _, sugg := range suggestions {
		if !seen[sugg.term] {
			result = append(result, sugg.term)
			seen[sugg.term] = true
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}
