package parser

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
)

var mermaidLateReplaceRe = regexp.MustCompile(`<!--KOSH_MERMAID:([a-f0-9]+)-->(?:<!--KOSH_MERMAID_REG:([a-f0-9]+):([^:]+):([^:]+)-->)?`)

// mermaidBlockInfo holds Mermaid diagram block information for deduplication.
type mermaidBlockInfo struct {
	node *ast.FencedCodeBlock
	code string
	hash string
}

// ReplaceMermaidExpressions replaces Mermaid placeholders in HTML with rendered SVG output.
func ReplaceMermaidExpressions(htmlContent string, expressions []models.MermaidExpression, rendered map[string]models.SSRThemePair) string {
	if len(expressions) == 0 {
		return htmlContent
	}

	replacements := make([]string, 0, len(expressions)*2)
	for _, expr := range expressions {
		if pair, ok := rendered[expr.Hash]; ok {
			placeholder := "<!--KOSH_MERMAID:" + expr.Hash + "-->"
			buf := pools.SharedBufferPool.Get()
			buf.WriteString(`<div class="mermaid-container" data-diagram="true"><div class="mermaid-light">`)
			buf.WriteString(pair.Light)
			buf.WriteString(`</div><div class="mermaid-dark">`)
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

// HasMermaidPlaceholders checks if the HTML content has Mermaid placeholders.
func HasMermaidPlaceholders(html string) bool {
	return strings.Contains(html, "<!--KOSH_MERMAID:")
}

// ExtractMermaidHashes returns all unique Mermaid diagram hashes found in the HTML.
func ExtractMermaidHashes(html string) []string {
	if !HasMermaidPlaceholders(html) {
		return nil
	}

	matches := mermaidLateReplaceRe.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	hashes := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			hash := m[1]
			if _, ok := seen[hash]; !ok {
				seen[hash] = struct{}{}
				hashes = append(hashes, hash)
			}
		}
	}
	return hashes
}

// LateReplaceMermaid performs a final replacement of Mermaid placeholders using registry comments.
func LateReplaceMermaid(htmlContent string, rendered map[string]models.SSRThemePair) string {
	if len(rendered) == 0 || !HasMermaidPlaceholders(htmlContent) {
		return htmlContent
	}

	return mermaidLateReplaceRe.ReplaceAllStringFunc(htmlContent, func(match string) string {
		parts := mermaidLateReplaceRe.FindStringSubmatch(match)
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

		buf.WriteString(`<div class="mermaid-container" data-diagram="true"><div class="mermaid-light">`)
		buf.WriteString(pair.Light)
		buf.WriteString(`</div><div class="mermaid-dark">`)
		buf.WriteString(pair.Dark)
		buf.WriteString(`</div><span class="zoom-hint">Click to zoom</span></div>`)

		return buf.String()
	})
}
