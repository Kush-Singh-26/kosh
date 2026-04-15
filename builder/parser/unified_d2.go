package parser

import (
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/yuin/goldmark/ast"
)

// d2BlockInfo holds D2 diagram block information for deduplication.
type d2BlockInfo struct {
	node *ast.FencedCodeBlock
	code string
	hash string
}

// ReplaceD2Expressions replaces D2 placeholders in HTML with rendered SVG output.
func ReplaceD2Expressions(htmlContent string, expressions []models.D2Expression, rendered map[string]models.SSRThemePair) string {
	if len(expressions) == 0 {
		return htmlContent
	}

	replacements := make([]string, 0, len(expressions)*2)
	for _, expr := range expressions {
		if pair, ok := rendered[expr.Hash]; ok {
			placeholder := "<!--KOSH_D2:" + expr.Hash + "-->"
			buf := pools.SharedBufferPool.Get()
			buf.WriteString(`<div class="d2-container" data-diagram="true"><div class="d2-light">`)
			buf.WriteString(pair.Light)
			buf.WriteString(`</div><div class="d2-dark">`)
			buf.WriteString(pair.Dark)
			buf.WriteString(`</div><span class="zoom-hint">Click to zoom</span></div>`)

			replacement := buf.String()
			pools.SharedBufferPool.Put(buf)
			replacements = append(replacements, placeholder, replacement)
		}
	}

	if len(replacements) == 0 {
		return htmlContent
	}

	return strings.NewReplacer(replacements...).Replace(htmlContent)
}

// HasD2Placeholders checks if the HTML content has D2 placeholders.
func HasD2Placeholders(html string) bool {
	return strings.Contains(html, "<!--KOSH_D2:")
}
