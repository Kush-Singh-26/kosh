package parser

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/yuin/goldmark/parser"
)

var mathExpressionsKey = parser.NewContextKey()

// ReplaceMathExpressions replaces LaTeX placeholders in HTML with rendered output.
func ReplaceMathExpressions(html string, expressions []models.MathExpression, rendered map[string]string) string {
	if len(expressions) == 0 {
		return html
	}

	replacements := make([]string, 0, len(expressions)*2)
	for _, expr := range expressions {
		if renderedHTML, ok := rendered[expr.Hash]; ok {
			placeholder := "<!--KOSH_MATH:" + expr.Hash + "-->"
			var replacement string
			if expr.DisplayMode {
				replacement = `<div class="katex-display">` + renderedHTML + `</div>`
			} else {
				replacement = `<span class="katex-inline">` + renderedHTML + `</span>`
			}
			replacements = append(replacements, placeholder, replacement)
		}
	}

	if len(replacements) == 0 {
		return html
	}

	return strings.NewReplacer(replacements...).Replace(html)
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML.
func RenderMathForHTML(ctx context.Context, html string, renderer *native.Renderer, cacheLookup func(string) (string, bool), preCollected []models.MathExpression) (string, []string, map[string]string) {
	if len(preCollected) == 0 {
		return html, nil, nil
	}

	hashes := make([]string, len(preCollected))
	for i, expr := range preCollected {
		hashes[i] = expr.Hash
	}

	cachedSubset := make(map[string]string, len(preCollected))
	if cacheLookup != nil {
		for _, expr := range preCollected {
			if v, ok := cacheLookup(expr.Hash); ok {
				cachedSubset[expr.Hash] = v
			}
		}
	}

	rendered, err := renderer.RenderAllMath(ctx, preCollected, cachedSubset)
	if err != nil {
		slog.Warn("LaTeX batch render failed", "error", err)
	}

	newEntries := make(map[string]string)
	for hash, value := range rendered {
		if _, existed := cachedSubset[hash]; !existed {
			newEntries[hash] = value
		}
	}

	return ReplaceMathExpressions(html, preCollected, rendered), hashes, newEntries
}

// GetMathExpressions retrieves math from context
func GetMathExpressions(pc parser.Context) []models.MathExpression {
	if v := pc.Get(mathExpressionsKey); v != nil {
		return v.([]models.MathExpression)
	}
	return nil
}
