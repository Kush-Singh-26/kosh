package parser

import (
	htmlLib "html"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

var (
	// Currency pattern: starts with a digit (e.g., $5, $10.00)
	currencyPattern = regexp.MustCompile(`^\d`)
)

type ScannedMathMatch struct {
	Match       MathMatch
	Latex       string
	TypeStr     string
	Hash        string
	DisplayMode bool
}

func ScanMathExpressions(html string) ([]ScannedMathMatch, []native.MathExpression) {
	lexer := NewMathLexer(html)
	matches := lexer.Scan()
	if len(matches) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool, len(matches))
	processed := make([]ScannedMathMatch, 0, len(matches))
	expressions := make([]native.MathExpression, 0, len(matches))

	for _, m := range matches {
		latex := htmlLib.UnescapeString(m.Content)
		if m.Type == MathBlock || m.Type == MathDisplay {
			latex = strings.TrimSpace(latex)
		}
		if m.Type == MathInline && currencyPattern.MatchString(latex) {
			continue
		}

		typeStr := "math-inline"
		displayMode := false
		switch m.Type {
		case MathBlock:
			typeStr = "math-block"
			displayMode = true
		case MathDisplay:
			typeStr = "math-display"
			displayMode = true
		case MathParen:
			typeStr = "math-paren"
		}

		hash := native.HashContent(typeStr, latex)
		processed = append(processed, ScannedMathMatch{Match: m, Latex: latex, TypeStr: typeStr, Hash: hash, DisplayMode: displayMode})
		if !seen[hash] {
			seen[hash] = true
			expressions = append(expressions, native.MathExpression{LaTeX: latex, DisplayMode: displayMode, Hash: hash})
		}
	}

	return processed, expressions
}

// ExtractMathExpressions finds all LaTeX expressions in HTML and returns them with metadata.
func ExtractMathExpressions(html string) []native.MathExpression {
	_, expressions := ScanMathExpressions(html)
	return expressions
}

// ReplaceMathExpressions replaces LaTeX expressions in HTML with rendered output.
func ReplaceMathExpressions(html string, matches []ScannedMathMatch, rendered map[string]string) string {
	if len(matches) == 0 {
		return html
	}

	var sb strings.Builder
	sb.Grow(len(html) + 512)
	lastPos := 0

	for _, sm := range matches {
		m := sm.Match
		// Append text before match
		sb.WriteString(html[lastPos:m.Start])

		if renderedHTML, ok := rendered[sm.Hash]; ok {
			if m.Type == MathBlock || m.Type == MathDisplay {
				sb.WriteString(`<div class="katex-display">`)
				sb.WriteString(renderedHTML)
				sb.WriteString(`</div>`)
			} else {
				sb.WriteString(`<span class="katex-inline">`)
				sb.WriteString(renderedHTML)
				sb.WriteString(`</span>`)
			}
		} else {
			// If not rendered, keep original
			sb.WriteString(html[m.Start:m.End])
		}
		lastPos = m.End
	}

	// Append remaining text
	sb.WriteString(html[lastPos:])

	return sb.String()
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML.
// It returns the rendered HTML, a slice of SSR input hashes, and newly rendered cache entries.
func RenderMathForHTML(html string, renderer *native.Renderer, cacheLookup func(string) (string, bool), preCollected []native.MathExpression) (string, []string, map[string]string) {
	var matches []ScannedMathMatch
	var expressions []native.MathExpression

	if len(preCollected) > 0 {
		// Use pre-collected expressions from AST to skip discovery, but we still need matches for replacement.
		// Actually, we still need matches to know WHERE to replace.
		// So we still call ScanMathExpressions, but we can skip the Hash calculation if we want.
		// However, ScanMathExpressions is already fast. The bridge is the slow part.
		matches, expressions = ScanMathExpressions(html)
	} else {
		matches, expressions = ScanMathExpressions(html)
	}

	if len(expressions) == 0 {
		return html, nil, nil
	}

	hashes := make([]string, len(expressions))
	for i, expr := range expressions {
		hashes[i] = expr.Hash
	}

	cachedSubset := make(map[string]string, len(expressions))
	if cacheLookup != nil {
		for _, expr := range expressions {
			if v, ok := cacheLookup(expr.Hash); ok {
				cachedSubset[expr.Hash] = v
			}
		}
	}

	rendered, err := renderer.RenderAllMath(expressions, cachedSubset)
	if err != nil {
		slog.Warn("LaTeX batch render failed", "error", err)
	}

	newEntries := make(map[string]string)
	for hash, value := range rendered {
		if _, existed := cachedSubset[hash]; !existed {
			newEntries[hash] = value
		}
	}

	return ReplaceMathExpressions(html, matches, rendered), hashes, newEntries
}
