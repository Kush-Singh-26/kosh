package search

import (
	"math/bits"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
)

// match represents a term match position
type snippetMatch struct {
	pos  int
	term string
}

// truncateContent truncates content to MaxSnippetContentLength and aligns to rune boundaries
func truncateContent(content string) string {
	if len(content) <= MaxSnippetContentLength {
		return content
	}

	content = content[:MaxSnippetContentLength]
	// Align to rune boundary to avoid invalid UTF-8
	for len(content) > 0 && !utf8.RuneStart(content[len(content)-1]) {
		content = content[:len(content)-1]
	}
	// If the last byte is the start of a multi-byte rune but we don't have the rest,
	// we should also trim it.
	r, sz := utf8.DecodeLastRuneInString(content)
	if r == utf8.RuneError && sz == 1 {
		content = content[:len(content)-1]
	}
	return content
}

// findMatches finds all term matches in the content
func findMatches(content string, terms []string, termOffsets map[string][]int) []snippetMatch {
	var matches []snippetMatch

	// First try to use term offsets if available
	if len(termOffsets) > 0 {
		for _, term := range terms {
			if offsets, ok := termOffsets[term]; ok {
				for i := 0; i < len(offsets); i += 2 {
					start := offsets[i]
					if start < len(content) {
						matches = append(matches, snippetMatch{pos: start, term: term})
					}
				}
			}
		}
	}

	// If no matches from offsets, search in content
	if len(matches) == 0 {
		contentLower := core.ToLower(content)
		for _, term := range terms {
			if len(term) < 2 {
				continue
			}
			curr := 0
			for {
				idx := strings.Index(contentLower[curr:], term)
				if idx == -1 {
					break
				}
				matches = append(matches, snippetMatch{pos: curr + idx, term: term})
				curr += idx + len(term)
			}
		}
	}

	return matches
}

// scoreMatchWindow calculates the score for a window of matches starting at startIndex
func scoreMatchWindow(matches []snippetMatch, termToIndex map[string]int, windowSize int, startIndex int) (count int, mask uint64) {
	count = 0
	mask = 0

	for j := startIndex; j < len(matches) && matches[j].pos < matches[startIndex].pos+windowSize; j++ {
		count++
		if idx, ok := termToIndex[matches[j].term]; ok {
			if idx < 64 {
				mask |= (1 << uint(idx))
			}
		}
	}

	return count, mask
}

// findBestSnippetWindow finds the best window position for the snippet
func findBestSnippetWindow(matches []snippetMatch, content string, termToIndex map[string]int) (start, end int) {
	if len(matches) == 0 {
		return 0, min(DefaultSnippetLength, len(content))
	}

	bestStart := matches[0].pos
	maxScore := 0
	windowSize := DefaultSnippetLength

	// Find the window with the best score
	for i := 0; i < len(matches); i++ {
		count, mask := scoreMatchWindow(matches, termToIndex, windowSize, i)
		score := uint64(bits.OnesCount64(mask))*100 + uint64(count)
		if score > uint64(maxScore) {
			maxScore = int(score)
			bestStart = matches[i].pos
		}
	}

	// Calculate window boundaries with context
	start = max(bestStart-SnippetContextBefore, 0)
	// Align to rune boundary to avoid panic
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}

	end = start + windowSize + SnippetContextBefore
	if end > len(content) {
		end = len(content)
		start = max(end-(windowSize+SnippetContextBefore), 0)
		for start > 0 && !utf8.RuneStart(content[start]) {
			start--
		}
	} else {
		// Align end to rune boundary
		for end < len(content) && !utf8.RuneStart(content[end]) {
			end++
		}
	}

	// Adjust start to word boundary if needed
	if start > 0 {
		idx := strings.Index(content[start:], " ")
		if idx != -1 && idx < 15 {
			start += idx + 1
		}
	}

	return start, end
}

// buildSnippetText builds the final snippet text with highlighted matches
func buildSnippetText(content string, matches []snippetMatch, start, end int, hasMatches bool) string {
	b := pools.SharedStringBuilderPool.Get()
	b.Grow(int(float64(end-start) * 1.2))

	if start > 0 {
		b.WriteString("...")
	}

	if !hasMatches || len(matches) == 0 {
		// No matches, just return truncated content
		escapeToBuilder(b, content[start:end])
		if end < len(content) {
			b.WriteString("...")
		}
		res := b.String()
		pools.SharedStringBuilderPool.Put(b)
		return res
	}

	// Build snippet with highlighted matches
	lastPos := start
	for _, m := range matches {
		if m.pos < start {
			continue
		}
		if m.pos >= end {
			break
		}
		if m.pos < lastPos {
			continue
		}

		escapeToBuilder(b, content[lastPos:m.pos])

		// Find word end (including trailing \w*)
		actualEnd := m.pos + len(m.term)
		// Match \w* behavior (unicode letter or number)
		for actualEnd < end {
			r, sz := utf8.DecodeRuneInString(content[actualEnd:])
			if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' {
				break
			}
			actualEnd += sz
		}

		b.WriteString("<b>")
		escapeToBuilder(b, content[m.pos:actualEnd])
		b.WriteString("</b>")
		lastPos = actualEnd
	}

	escapeToBuilder(b, content[lastPos:end])

	if end < len(content) {
		b.WriteString("...")
	}

	res := b.String()
	pools.SharedStringBuilderPool.Put(b)
	return res
}

// buildSimpleSnippet builds a simple snippet without term matching
func buildSimpleSnippet(content string) string {
	if len(content) > DefaultSnippetLength {
		b := pools.SharedStringBuilderPool.Get()
		defer pools.SharedStringBuilderPool.Put(b)
		escapeToBuilder(b, content[:DefaultSnippetLength])
		b.WriteString("...")
		return b.String()
	}
	b := pools.SharedStringBuilderPool.Get()
	defer pools.SharedStringBuilderPool.Put(b)
	escapeToBuilder(b, content)
	return b.String()
}

// ExtractSnippet extracts a search snippet from content, highlighting matching terms.
// This is a refactored version using helper functions for better maintainability.
func ExtractSnippet(content string, terms []string, termOffsets map[string][]int) string {
	if len(content) == 0 {
		return ""
	}

	// Truncate content if too long
	content = truncateContent(content)

	// Handle case with no search terms
	if len(terms) == 0 {
		return buildSimpleSnippet(content)
	}

	// Find all term matches
	matches := findMatches(content, terms, termOffsets)

	// If no matches found, return simple snippet
	if len(matches) == 0 {
		return buildSimpleSnippet(content)
	}

	// Sort matches by position
	slices.SortFunc(matches, func(a, b snippetMatch) int {
		return a.pos - b.pos
	})

	// Create term to index mapping for scoring
	termToIndex := make(map[string]int, len(terms))
	for i, t := range terms {
		termToIndex[t] = i
	}

	// Find the best window position for the snippet
	start, end := findBestSnippetWindow(matches, content, termToIndex)

	// Build the final snippet with highlighted matches
	return buildSnippetText(content, matches, start, end, true)
}

func escapeToBuilder(sb *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '&':
			sb.WriteString("&amp;")
		case '\'':
			sb.WriteString("&#39;")
		case '"':
			sb.WriteString("&#34;")
		case '<':
			sb.WriteString("&lt;")
		case '>':
			sb.WriteString("&gt;")
		default:
			sb.WriteByte(c)
		}
	}
}
