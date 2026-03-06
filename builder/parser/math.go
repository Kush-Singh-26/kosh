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

// ExtractMathExpressions finds all LaTeX expressions in HTML and returns them with metadata.
func ExtractMathExpressions(html string) []native.MathExpression {
	var expressions []native.MathExpression
	seen := make(map[string]bool) // Deduplicate

	lexer := NewMathLexer(html)
	matches := lexer.Scan()

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
		if !seen[hash] {
			seen[hash] = true
			expressions = append(expressions, native.MathExpression{LaTeX: latex, DisplayMode: displayMode, Hash: hash})
		}
	}

	return expressions
}

// ReplaceMathExpressions replaces LaTeX expressions in HTML with rendered output.
func ReplaceMathExpressions(html string, rendered map[string]string) string {
	lexer := NewMathLexer(html)
	matches := lexer.Scan()
	if len(matches) == 0 {
		return html
	}

	var sb strings.Builder
	sb.Grow(len(html) + 512)
	lastPos := 0

	for _, m := range matches {
		// Append text before match
		sb.WriteString(html[lastPos:m.Start])

		latex := htmlLib.UnescapeString(m.Content)
		typeStr := "math-inline"
		switch m.Type {
		case MathBlock:
			typeStr = "math-block"
			latex = strings.TrimSpace(latex)
		case MathDisplay:
			typeStr = "math-display"
			latex = strings.TrimSpace(latex)
		case MathParen:
			typeStr = "math-paren"
		}

		hash := native.HashContent(typeStr, latex)
		if renderedHTML, ok := rendered[hash]; ok {
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
func RenderMathForHTML(html string, renderer *native.Renderer, cacheLookup func(string) (string, bool)) (string, []string, map[string]string) {
	expressions := ExtractMathExpressions(html)
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

	return ReplaceMathExpressions(html, rendered), hashes, newEntries
}
