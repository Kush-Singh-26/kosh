package parser

import (
	htmlLib "html"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

var (
	// Currency pattern: starts with a digit (e.g., $5, $10.00)
	currencyPattern = regexp.MustCompile(`^\d`)
)

// ExtractMathExpressions finds all LaTeX expressions in HTML and returns them with metadata
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

// ReplaceMathExpressions replaces LaTeX expressions in HTML with rendered output
func ReplaceMathExpressions(html string, rendered map[string]string, cache map[string]string, cacheMu *sync.Mutex) string {
	lexer := NewMathLexer(html)
	matches := lexer.Scan()
	if len(matches) == 0 {
		return html
	}

	// Ensure cache is not nil to avoid panic on assignment
	if cache == nil {
		cache = make(map[string]string)
	}

	getRendered := func(hash string) (string, bool) {
		if h, ok := rendered[hash]; ok {
			if cacheMu != nil {
				cacheMu.Lock()
				cache[hash] = h
				cacheMu.Unlock()
			}
			return h, true
		}
		if cacheMu != nil {
			cacheMu.Lock()
			h, ok := cache[hash]
			cacheMu.Unlock()
			return h, ok
		}
		h, ok := cache[hash]
		return h, ok
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
		if renderedHTML, ok := getRendered(hash); ok {
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

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML
// Returns the rendered HTML and a slice of SSR input hashes for cache tracking
func RenderMathForHTML(html string, renderer *native.Renderer, cache map[string]string, cacheMu *sync.Mutex) (string, []string) {
	expressions := ExtractMathExpressions(html)
	if len(expressions) == 0 {
		return html, nil
	}

	hashes := make([]string, len(expressions))
	for i, expr := range expressions {
		hashes[i] = expr.Hash
	}

	cacheMu.Lock()
	cachedCopy := make(map[string]string)
	for k, v := range cache {
		cachedCopy[k] = v
	}
	cacheMu.Unlock()

	rendered, err := renderer.RenderAllMath(expressions, cachedCopy)
	if err != nil {
		slog.Warn("   ⚠️  LaTeX batch render failed", "error", err)
	}

	return ReplaceMathExpressions(html, rendered, cache, cacheMu), hashes
}
