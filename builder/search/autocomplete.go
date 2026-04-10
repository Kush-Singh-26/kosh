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

// GetSuggestions returns a list of suggested terms based on a prefix
func GetSuggestions(index *models.SearchIndex, prefix string) []string {
	prefix = core.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < minSuggestionPrefixLen {
		return nil
	}

	type suggestion struct {
		term  string
		count int
	}

	var suggestions []suggestion

	// Check inverted index for matches
	for term, docs := range index.Inverted {
		if strings.HasPrefix(term, prefix) {
			suggestions = append(suggestions, suggestion{
				term:  term,
				count: len(docs),
			})
		}
	}

	// Also check stem map origins
	for stem, origins := range index.StemMap {
		if strings.HasPrefix(stem, prefix) {
			for _, origin := range origins {
				// Avoid duplicates if original is also in inverted (it should be, but just in case)
				found := false
				for _, sugg := range suggestions {
					if sugg.term == origin {
						found = true
						break
					}
				}
				if !found {
					// We don't have easy access to original term frequency here without
					// a separate map, so we'll use a heuristic or just look it up
					// if we really wanted to. For now, we'll just add it.
					suggestions = append(suggestions, suggestion{
						term:  origin,
						count: defaultSuggestionCount,
					})
				}
			}
		} else {
			// Check if any origin starts with the prefix
			for _, origin := range origins {
				if strings.HasPrefix(origin, prefix) {
					found := false
					for _, sugg := range suggestions {
						if sugg.term == origin {
							found = true
							break
						}
					}
					if !found {
						suggestions = append(suggestions, suggestion{
							term:  origin,
							count: defaultSuggestionCount,
						})
					}
				}
			}
		}
	}

	// Sort by frequency (descending) and then length (ascending)
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
