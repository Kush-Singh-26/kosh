package search

import (
	"slices"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
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
func GetSuggestions(index *models.SearchIndex, prefix string) []string {
	prefix = core.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < minSuggestionPrefixLen {
		return nil
	}

	var suggestions []suggestion
	suggestions = findInvertedSuggestions(index, prefix, suggestions)
	suggestions = findStemSuggestions(index, prefix, suggestions)

	return sortAndFinalizeSuggestions(suggestions)
}

func findInvertedSuggestions(index *models.SearchIndex, prefix string, suggestions []suggestion) []suggestion {
	for term, docs := range index.Inverted {
		if strings.HasPrefix(term, prefix) {
			suggestions = append(suggestions, suggestion{
				term:  term,
				count: len(docs),
			})
		}
	}
	return suggestions
}

func findStemSuggestions(index *models.SearchIndex, prefix string, suggestions []suggestion) []suggestion {
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
	return suggestions
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
