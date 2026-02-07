package parser

import (
	"fmt"
	"regexp"
)

// d2PreRegex matches d2 code blocks (matches the div wrapper)
var d2PreRegex = regexp.MustCompile(`(?s)<div class="code-wrapper" data-lang="d2">.*?</div>`)

// ReplaceD2BlocksWithThemeSupport replaces d2 blocks with both light and dark SVGs
// The browser will show/hide based on the data-theme attribute
func ReplaceD2BlocksWithThemeSupport(html string, pairs []D2SVGPair) string {
	if len(pairs) == 0 {
		return html
	}

	pairIndex := 0

	// Replace all d2 pre blocks with dual-themed SVGs in order
	return d2PreRegex.ReplaceAllStringFunc(html, func(match string) string {
		if pairIndex >= len(pairs) {
			return match // No more pairs to use
		}

		pair := pairs[pairIndex]
		pairIndex++

		// Output container with both light and dark versions
		// CSS will show/hide based on data-theme attribute
		return fmt.Sprintf(`<div class="d2-container" data-diagram="true"><div class="d2-light">%s</div><div class="d2-dark">%s</div><span class="zoom-hint">🔍 Click to zoom</span></div>`,
			pair.Light, pair.Dark)
	})
}

// D2SVGPair stores both light and dark versions of a diagram
type D2SVGPair struct {
	Light string
	Dark  string
}
