package parser

import (
	"fmt"
	htmlLib "html"
	"log"
	"regexp"
	"strings"
	"sync"

	"my-ssg/builder/renderer/native"
)

// LaTeX SSR logic and regex patterns

var (
	// Block math: $$...$$ (greedy, supports newlines)
	blockMathRegex = regexp.MustCompile(`(?s)\$\$(.+?)\$\$`)

	// Inline math: $...$ (non-greedy, no newlines usually, but we allow strict matching)
	inlineMathRegex = regexp.MustCompile(`\$((?:\\.|[^$\n<>])+?)\$`)

	// Display Math: \[ ... \]
	displayMathRegex = regexp.MustCompile(`(?s)\\\[(.*?)\\\]`)

	// Inline Math: \( ... \)
	inlineParenRegex = regexp.MustCompile(`(?s)\\\((.*?)\\\)`)

	// Currency pattern: starts with a digit (e.g., $5, $10.00)
	currencyPattern = regexp.MustCompile(`^\d`)
)

// ExtractMathExpressions finds all LaTeX expressions in HTML and returns them with metadata
func ExtractMathExpressions(html string) []native.MathExpression {
	var expressions []native.MathExpression
	seen := make(map[string]bool) // Deduplicate

	// 1. Extract block math ($$...$$)
	for _, match := range blockMathRegex.FindAllStringSubmatch(html, -1) {
		if len(match) >= 2 {
			latex := htmlLib.UnescapeString(match[1])
			latex = strings.TrimSpace(latex)
			hash := native.HashContent("math-block", latex)
			if !seen[hash] {
				seen[hash] = true
				expressions = append(expressions, native.MathExpression{LaTeX: latex, DisplayMode: true, Hash: hash})
			}
		}
	}

	// 2. Extract Display Math (\[ ... \])
	for _, match := range displayMathRegex.FindAllStringSubmatch(html, -1) {
		if len(match) >= 2 {
			latex := htmlLib.UnescapeString(match[1])
			latex = strings.TrimSpace(latex)
			hash := native.HashContent("math-display", latex)
			if !seen[hash] {
				seen[hash] = true
				expressions = append(expressions, native.MathExpression{LaTeX: latex, DisplayMode: true, Hash: hash})
			}
		}
	}

	// 3. Extract inline math ($...$)
	for _, match := range inlineMathRegex.FindAllStringSubmatch(html, -1) {
		if len(match) >= 2 {
			latex := htmlLib.UnescapeString(match[1])
			if currencyPattern.MatchString(latex) {
				continue
			}
			hash := native.HashContent("math-inline", latex)
			if !seen[hash] {
				seen[hash] = true
				expressions = append(expressions, native.MathExpression{LaTeX: latex, DisplayMode: false, Hash: hash})
			}
		}
	}

	// 4. Extract Inline Paren Math (\( ... \))
	for _, match := range inlineParenRegex.FindAllStringSubmatch(html, -1) {
		if len(match) >= 2 {
			latex := htmlLib.UnescapeString(match[1])
			hash := native.HashContent("math-paren", latex)
			if !seen[hash] {
				seen[hash] = true
				expressions = append(expressions, native.MathExpression{LaTeX: latex, DisplayMode: false, Hash: hash})
			}
		}
	}

	return expressions
}

// ReplaceMathExpressions replaces LaTeX expressions in HTML with rendered output
func ReplaceMathExpressions(html string, rendered map[string]string, cache map[string]string, cacheMu *sync.Mutex) string {
	if len(rendered) == 0 && len(cache) == 0 {
		return html
	}

	// Ensure cache is not nil to avoid panic on assignment
	if cache == nil {
		cache = make(map[string]string)
	}

	result := html
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

	// 1. Replace block math ($$...$$)
	result = blockMathRegex.ReplaceAllStringFunc(result, func(match string) string {
		submatch := blockMathRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		latex := htmlLib.UnescapeString(submatch[1])
		latex = strings.TrimSpace(latex)
		hash := native.HashContent("math-block", latex)
		if html, ok := getRendered(hash); ok {
			return fmt.Sprintf(`<div class="katex-display">%s</div>`, html)
		}
		return match
	})

	// 2. Replace Display Math (\[ ... \])
	result = displayMathRegex.ReplaceAllStringFunc(result, func(match string) string {
		submatch := displayMathRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		latex := htmlLib.UnescapeString(submatch[1])
		latex = strings.TrimSpace(latex)
		hash := native.HashContent("math-display", latex)
		if html, ok := getRendered(hash); ok {
			return fmt.Sprintf(`<div class="katex-display">%s</div>`, html)
		}
		return match
	})

	// 3. Replace inline math ($...$)
	result = inlineMathRegex.ReplaceAllStringFunc(result, func(match string) string {
		submatch := inlineMathRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		latex := htmlLib.UnescapeString(submatch[1])
		if currencyPattern.MatchString(latex) {
			return match
		}
		hash := native.HashContent("math-inline", latex)
		if html, ok := getRendered(hash); ok {
			return fmt.Sprintf(`<span class="katex-inline">%s</span>`, html)
		}
		return match
	})

	// 4. Replace Inline Paren Math (\( ... \))
	result = inlineParenRegex.ReplaceAllStringFunc(result, func(match string) string {
		submatch := inlineParenRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		latex := htmlLib.UnescapeString(submatch[1])
		hash := native.HashContent("math-paren", latex)
		if html, ok := getRendered(hash); ok {
			return fmt.Sprintf(`<span class="katex-inline">%s</span>`, html)
		}
		return match
	})

	return result
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML
func RenderMathForHTML(html string, renderer *native.Renderer, cache map[string]string, cacheMu *sync.Mutex) string {
	expressions := ExtractMathExpressions(html)
	if len(expressions) == 0 {
		return html
	}

	cacheMu.Lock()
	cachedCopy := make(map[string]string)
	for k, v := range cache {
		cachedCopy[k] = v
	}
	cacheMu.Unlock()

	rendered, err := renderer.RenderAllMath(expressions, cachedCopy)
	if err != nil {
		log.Printf("   ⚠️  LaTeX batch render failed: %v", err)
	}

	return ReplaceMathExpressions(html, rendered, cache, cacheMu)
}
