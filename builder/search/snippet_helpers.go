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

const (
	minTermLengthForScan    = 2
	windowSizeMultiMatch    = 100
	matchScoreMultiplier    = 100
	secondIslandMargin      = 50
	secondIslandMinScoreDiv = 2
	wordTrimMaxOffset       = 15
	snippetGrowFactor       = 1.3
	maxTermMaskBits         = 64
)

// match represents a term match position
type snippetMatch struct {
	pos  int
	term string
}

// truncateToLength truncates string text to maxLen and aligns to rune boundaries
func truncateToLength(text string, maxLen int) string {
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

// truncateContent truncates content to MaxSnippetContentLength and aligns to rune boundaries
func truncateContent(content string) string {
	return truncateToLength(content, MaxSnippetContentLength)
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
			if len(term) < minTermLengthForScan {
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
func scoreMatchWindow(matches []snippetMatch, termToIndex map[string]int, windowSize int, startIndex int) (int, uint64) {
	count := 0
	var mask uint64

	for j := startIndex; j < len(matches) && matches[j].pos < matches[startIndex].pos+windowSize; j++ {
		count++
		if idx, ok := termToIndex[matches[j].term]; ok {
			if idx < maxTermMaskBits {
				mask |= (1 << uint(idx))
			}
		}
	}

	return count, mask
}

// island represents a single snippet fragment
type snippetIsland struct {
	start int
	end   int
}

// findBestSnippetIslands finds up to two best window positions for the snippet
func findBestSnippetIslands(matches []snippetMatch, content string, termToIndex map[string]int) []snippetIsland {
	if len(matches) == 0 {
		return []snippetIsland{{start: 0, end: min(DefaultSnippetLength, len(content))}}
	}

	windowSize := DefaultSnippetLength
	if len(matches) > 1 {
		windowSize = windowSizeMultiMatch // Smaller windows for multiple islands
	}

	type window struct {
		start int
		score int
		idx   int // Index of the match that starts this window
	}

	var windows []window
	for i := 0; i < len(matches); i++ {
		count, mask := scoreMatchWindow(matches, termToIndex, windowSize, i)
		score := bits.OnesCount64(mask)*matchScoreMultiplier + count
		windows = append(windows, window{start: matches[i].pos, score: score, idx: i})
	}

	// Sort windows by score descending
	slices.SortFunc(windows, func(a, b window) int {
		return b.score - a.score
	})

	best := windows[0]
	islands := []snippetIsland{calculateIsland(best.start, windowSize, content)}

	// Try to find a second island that doesn't overlap
	for i := 1; i < len(windows); i++ {
		curr := windows[i]
		// Check for overlap with some margin
		if curr.start > islands[0].end+secondIslandMargin || curr.start+windowSize < islands[0].start-secondIslandMargin {
			// Good second island found
			if curr.score > best.score/secondIslandMinScoreDiv { // Only include if it's reasonably relevant
				secondIsland := calculateIsland(curr.start, windowSize, content)
				islands = append(islands, secondIsland)
				break
			}
		}
	}

	// Sort islands by position
	slices.SortFunc(islands, func(a, b snippetIsland) int {
		return a.start - b.start
	})

	return islands
}

func calculateIsland(bestStart, windowSize int, content string) snippetIsland {
	start := max(bestStart-SnippetContextBefore, 0)
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}

	end := start + windowSize + SnippetContextBefore
	if end > len(content) {
		end = len(content)
		start = max(end-(windowSize+SnippetContextBefore), 0)
		for start > 0 && !utf8.RuneStart(content[start]) {
			start--
		}
	} else {
		for end < len(content) && !utf8.RuneStart(content[end]) {
			end++
		}
	}

	if start > 0 {
		idx := strings.Index(content[start:], " ")
		if idx != -1 && idx < wordTrimMaxOffset {
			start += idx + 1
		}
	}

	return snippetIsland{start: start, end: end}
}

// buildSnippetText builds the final snippet text with highlighted matches across islands
func buildSnippetText(content string, matches []snippetMatch, islands []snippetIsland, hasMatches bool) string {
	builder := pools.SharedStringBuilderPool.Get()

	totalLen := 0
	for _, island := range islands {
		totalLen += island.end - island.start
	}
	builder.Grow(int(float64(totalLen) * snippetGrowFactor))

	if !hasMatches || len(matches) == 0 {
		island := islands[0]
		if island.start > 0 {
			builder.WriteString("...")
		}
		escapeToBuilder(builder, content[island.start:island.end])
		if island.end < len(content) {
			builder.WriteString("...")
		}
		res := builder.String()
		pools.SharedStringBuilderPool.Put(builder)
		return res
	}

	for i, island := range islands {
		if i > 0 || island.start > 0 {
			builder.WriteString("...")
		}

		lastPos := island.start
		for _, match := range matches {
			if match.pos < island.start {
				continue
			}
			if match.pos >= island.end {
				break
			}
			if match.pos < lastPos {
				continue
			}

			escapeToBuilder(builder, content[lastPos:match.pos])

			actualEnd := match.pos + len(match.term)
			if actualEnd > island.end {
				actualEnd = island.end
			}

			for actualEnd < island.end {
				char, size := utf8.DecodeRuneInString(content[actualEnd:])
				if !unicode.IsLetter(char) && !unicode.IsNumber(char) && char != '_' {
					break
				}
				actualEnd += size
			}

			builder.WriteString("<b>")
			escapeToBuilder(builder, content[match.pos:actualEnd])
			builder.WriteString("</b>")
			lastPos = actualEnd
		}

		if lastPos < island.end {
			escapeToBuilder(builder, content[lastPos:island.end])
		}

		if i == len(islands)-1 && island.end < len(content) {
			builder.WriteString("...")
		}
	}

	res := builder.String()
	pools.SharedStringBuilderPool.Put(builder)
	return res
}

// buildSimpleSnippet builds a simple snippet without term matching
func buildSimpleSnippet(content string) string {
	if len(content) > DefaultSnippetLength {
		b := pools.SharedStringBuilderPool.Get()
		defer pools.SharedStringBuilderPool.Put(b)
		truncated := truncateToLength(content, DefaultSnippetLength)
		escapeToBuilder(b, truncated)
		b.WriteString("...")
		return b.String()
	}
	b := pools.SharedStringBuilderPool.Get()
	defer pools.SharedStringBuilderPool.Put(b)
	escapeToBuilder(b, content)
	return b.String()
}

// ExtractSnippet extracts a search snippet from content, highlighting matching terms.
func ExtractSnippet(content string, terms []string, termOffsets map[string][]int) string {
	if len(content) == 0 {
		return ""
	}

	content = truncateContent(content)

	if len(terms) == 0 {
		return buildSimpleSnippet(content)
	}

	matches := findMatches(content, terms, termOffsets)

	if len(matches) == 0 {
		return buildSimpleSnippet(content)
	}

	slices.SortFunc(matches, func(a, b snippetMatch) int {
		return a.pos - b.pos
	})

	termToIndex := make(map[string]int, len(terms))
	for i, t := range terms {
		termToIndex[t] = i
	}

	islands := findBestSnippetIslands(matches, content, termToIndex)

	return buildSnippetText(content, matches, islands, true)
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
