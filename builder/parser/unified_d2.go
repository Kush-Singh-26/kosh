package parser

import (
	"regexp"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/yuin/goldmark/ast"
)

var d2LateReplaceRe = regexp.MustCompile(`<!--KOSH_D2:([a-f0-9]+)-->(?:<!--KOSH_D2_REG:([a-f0-9]+):([^:]+):([^:]+)-->)?`)

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

// LateReplaceD2 performs a final replacement of D2 placeholders using registry comments.
func LateReplaceD2(htmlContent string, rendered map[string]models.SSRThemePair) string {
	if len(rendered) == 0 || !HasD2Placeholders(htmlContent) {
		return htmlContent
	}

	return d2LateReplaceRe.ReplaceAllStringFunc(htmlContent, func(match string) string {
		parts := d2LateReplaceRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		hash := parts[1]
		pair, ok := rendered[hash]
		if !ok {
			return match
		}

		buf := pools.SharedBufferPool.Get()
		defer pools.SharedBufferPool.Put(buf)

		buf.WriteString(`<div class="d2-container" data-diagram="true"><div class="d2-light">`)
		buf.WriteString(pair.Light)
		buf.WriteString(`</div><div class="d2-dark">`)
		buf.WriteString(pair.Dark)
		buf.WriteString(`</div><span class="zoom-hint">Click to zoom</span></div>`)

		return buf.String()
	})
}
