package parser

import (
	"context"
	"log/slog"
	"strings"

	"github.com/yuin/goldmark/parser"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
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

// RenderMathOptions configures RenderMathForHTML.
type RenderMathOptions struct {
	// Required
	Ctx          context.Context
	HTML         string
	Renderer     *native.Renderer
	PreCollected []models.MathExpression

	// Optional
	CacheLookup func(string) (string, bool)
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML.
func RenderMathForHTML(opts RenderMathOptions) (string, []string, map[string]string) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Renderer == nil {
		panic("RenderMathForHTML: Renderer is nil")
	}
	if len(opts.PreCollected) == 0 {
		return opts.HTML, nil, nil
	}

	hashes := make([]string, len(opts.PreCollected))
	for i, expr := range opts.PreCollected {
		hashes[i] = expr.Hash
	}

	cachedSubset := make(map[string]string, len(opts.PreCollected))
	if opts.CacheLookup != nil {
		for _, expr := range opts.PreCollected {
			if v, ok := opts.CacheLookup(expr.Hash); ok {
				cachedSubset[expr.Hash] = v
			}
		}
	}

	rendered, err := opts.Renderer.RenderAllMath(opts.Ctx, opts.PreCollected, cachedSubset)
	if err != nil {
		slog.Warn("LaTeX batch render failed", "error", err)
	}

	newEntries := make(map[string]string)
	for hash, value := range rendered {
		if _, existed := cachedSubset[hash]; !existed {
			newEntries[hash] = value
		}
	}

	return ReplaceMathExpressions(opts.HTML, opts.PreCollected, rendered), hashes, newEntries
}

// GetMathExpressions retrieves math from context
func GetMathExpressions(pc parser.Context) []models.MathExpression {
	if v := pc.Get(mathExpressionsKey); v != nil {
		return v.([]models.MathExpression)
	}
	return nil
}
