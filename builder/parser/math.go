package parser

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var mathExpressionsKey = parser.NewContextKey()

// MathTransformer walks the AST to collect all LaTeX expressions AND replaces them with placeholders.
type MathTransformer struct{}

func (t *MathTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	var expressions []native.MathExpression
	source := reader.Source()

	type replacement struct {
		old ast.Node
		new ast.Node
	}
	var toReplace []replacement

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		var latex string
		var typeStr string
		var displayMode bool

		if n.Kind() == passthrough.KindPassthroughInline {
			m := n.(*passthrough.PassthroughInline)
			val := string(m.Segment.Value(source))
			// Strip delimiters $...$ or \(...\)
			if strings.HasPrefix(val, "$") && strings.HasSuffix(val, "$") {
				latex = val[1 : len(val)-1]
			} else if strings.HasPrefix(val, `\(`) && strings.HasSuffix(val, `\)`) {
				latex = val[2 : len(val)-2]
			} else {
				latex = val
			}
			latex = strings.TrimSpace(latex)
			typeStr = "math-inline"
			displayMode = false
		} else if n.Kind() == passthrough.KindPassthroughBlock {
			m := n.(*passthrough.PassthroughBlock)
			var lines strings.Builder
			l := m.Lines().Len()
			for i := range l {
				line := m.Lines().At(i)
				lines.Write(line.Value(source))
			}
			val := lines.String()
			// Strip delimiters $$...$$ or \[...\]
			if strings.HasPrefix(val, "$$") && strings.HasSuffix(val, "$$") {
				latex = val[2 : len(val)-2]
			} else if strings.HasPrefix(val, `\[`) && strings.HasSuffix(val, `\]`) {
				latex = val[2 : len(val)-2]
			} else {
				latex = val
			}
			latex = strings.TrimSpace(latex)
			typeStr = "math-block"
			displayMode = true
		} else {
			return ast.WalkContinue, nil
		}

		if latex == "" {
			return ast.WalkContinue, nil
		}

		hash := native.HashContent(typeStr, latex)
		expressions = append(expressions, native.MathExpression{
			LaTeX:       latex,
			DisplayMode: displayMode,
			Hash:        hash,
		})

		// Use RawHTMLInline for inline math, RawHTMLBlock for block math
		placeholder := "<!--KOSH_MATH:" + hash + "-->"
		var newNode ast.Node
		if displayMode {
			newNode = &RawHTMLBlock{Content: []byte(placeholder)}
		} else {
			newNode = &RawHTMLInline{Content: []byte(placeholder)}
		}
		toReplace = append(toReplace, replacement{old: n, new: newNode})

		return ast.WalkSkipChildren, nil
	})

	// Perform replacements after the walk to avoid skipping siblings
	for _, r := range toReplace {
		parent := r.old.Parent()
		if parent != nil {
			parent.ReplaceChild(parent, r.old, r.new)
		}
	}

	if len(expressions) > 0 {
		pc.Set(mathExpressionsKey, expressions)
	}
}

// ReplaceMathExpressions replaces LaTeX placeholders in HTML with rendered output.
func ReplaceMathExpressions(html string, expressions []native.MathExpression, rendered map[string]string) string {
	if len(expressions) == 0 {
		return html
	}

	for _, expr := range expressions {
		placeholder := "<!--KOSH_MATH:" + expr.Hash + "-->"
		if renderedHTML, ok := rendered[expr.Hash]; ok {
			var replacement string
			if expr.DisplayMode {
				replacement = `<div class="katex-display">` + renderedHTML + `</div>`
			} else {
				replacement = `<span class="katex-inline">` + renderedHTML + `</span>`
			}
			html = strings.ReplaceAll(html, placeholder, replacement)
		}
	}

	return html
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML.
func RenderMathForHTML(ctx context.Context, html string, renderer *native.Renderer, cacheLookup func(string) (string, bool), preCollected []native.MathExpression) (string, []string, map[string]string) {
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
func GetMathExpressions(pc parser.Context) []native.MathExpression {
	if v := pc.Get(mathExpressionsKey); v != nil {
		return v.([]native.MathExpression)
	}
	return nil
}
