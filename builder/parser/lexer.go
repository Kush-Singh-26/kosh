package parser

import (
	"strings"
)

// MathType represents the type of LaTeX math block found
type MathType int

const (
	// MathBlock represents $$...$$ blocks.
	MathBlock MathType = iota
	// MathDisplay represents \[...\] blocks.
	MathDisplay
	// MathInline represents $...$ inline math.
	MathInline
	// MathParen represents \(...\) inline math.
	MathParen
)

// MathMatch represents a found LaTeX expression
type MathMatch struct {
	Type    MathType
	Content string
	Start   int
	End     int
}

// MathLexer performs a single-pass scan of HTML to find LaTeX expressions
type MathLexer struct {
	input []byte
	pos   int
}

// NewMathLexer creates a MathLexer for the provided input.
func NewMathLexer(input string) *MathLexer {
	return &MathLexer{
		input: []byte(input),
		pos:   0,
	}
}

// Scan finds all math expressions in the input
func (l *MathLexer) Scan() []MathMatch {
	var matches []MathMatch
	n := len(l.input)

	for l.pos < n {
		char := l.input[l.pos]

		// Potential start of math
		switch {
		case char == '$':
			if l.pos+1 < n && l.input[l.pos+1] == '$' {
				// Block math $$
				startPos := l.pos
				l.pos += 2
				content, found := l.scanUntil("$$")
				if found {
					matches = append(matches, MathMatch{
						Type:    MathBlock,
						Content: content,
						Start:   startPos,
						End:     l.pos,
					})
				}
			} else {
				// Potential inline math $
				// Check for escape \$
				if l.pos > 0 && l.input[l.pos-1] == '\\' {
					l.pos++
					continue
				}

				startPos := l.pos
				l.pos++
				content, found := l.scanInlineMath()
				if found {
					matches = append(matches, MathMatch{
						Type:    MathInline,
						Content: content,
						Start:   startPos,
						End:     l.pos,
					})
				}
			}
		case char == '\\' && l.pos+1 < n:
			next := l.input[l.pos+1]
			switch next {
			case '[':
				// Display math \[
				startPos := l.pos
				l.pos += 2
				content, found := l.scanUntil("\\]")
				if found {
					matches = append(matches, MathMatch{
						Type:    MathDisplay,
						Content: content,
						Start:   startPos,
						End:     l.pos,
					})
				}
			case '(':
				// Inline paren math \(
				startPos := l.pos
				l.pos += 2
				content, found := l.scanUntil("\\)")
				if found {
					matches = append(matches, MathMatch{
						Type:    MathParen,
						Content: content,
						Start:   startPos,
						End:     l.pos,
					})
				}
			default:
				l.pos++
			}
		default:
			l.pos++
		}
	}

	return matches
}

func (l *MathLexer) scanUntil(delimiter string) (string, bool) {
	start := l.pos
	n := len(l.input)
	dLen := len(delimiter)

	for l.pos <= n-dLen {
		if string(l.input[l.pos:l.pos+dLen]) == delimiter {
			content := string(l.input[start:l.pos])
			l.pos += dLen
			return content, true
		}
		l.pos++
	}

	return "", false
}

func (l *MathLexer) scanInlineMath() (string, bool) {
	start := l.pos
	n := len(l.input)

	for l.pos < n {
		if l.input[l.pos] == '$' {
			// Check if escaped
			if l.pos > 0 && l.input[l.pos-1] == '\\' {
				l.pos++
				continue
			}
			// Found closing $
			content := string(l.input[start:l.pos])
			l.pos++

			// Basic validation: inline math shouldn't contain newlines or tags usually
			// but we follow the original regex which allows some characters.
			// inlineMathRegex = regexp.MustCompile(`\$((?:\\.|[^$\n<>])+?)\$`)
			if strings.ContainsAny(content, "\n<>") {
				return "", false
			}

			return content, true
		}
		l.pos++
	}

	return "", false
}
