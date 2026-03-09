package parser

import (
	"strings"

	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

// Package-level context keys, isolated by being unexported.
// parser.NewContextKey() returns globally unique keys via an atomic counter,
// so collisions across packages or multiple parser instances are impossible.
var (
	tocKey             = parser.NewContextKey()
	plainTextKey       = parser.NewContextKey()
	ssrHashesKey       = parser.NewContextKey()
	contextKeyBuild    = parser.NewContextKey()
	mathExpressionsKey = parser.NewContextKey()
)

func GetTOC(pc parser.Context) []models.TOCEntry {
	if v := pc.Get(tocKey); v != nil {
		return v.([]models.TOCEntry)
	}
	return nil
}

func GetPlainText(pc parser.Context) string {
	if v := pc.Get(plainTextKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetSSRHashes returns all SSR input hashes (D2 diagrams, LaTeX math) for cache tracking
func GetSSRHashes(pc parser.Context) []string {
	if v := pc.Get(ssrHashesKey); v != nil {
		return v.([]string)
	}
	return nil
}

func GetMathExpressions(pc parser.Context) []native.MathExpression {
	if v := pc.Get(mathExpressionsKey); v != nil {
		return v.([]native.MathExpression)
	}
	return nil
}

// AddSSRHash adds an SSR input hash to the context
func AddSSRHash(pc parser.Context, hash string) {
	var hashes []string
	if v := pc.Get(ssrHashesKey); v != nil {
		hashes = v.([]string)
	}
	hashes = append(hashes, hash)
	pc.Set(ssrHashesKey, hashes)
}

type tocTransformer struct{}

func (t *tocTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	var toc []models.TOCEntry
	var plainText strings.Builder
	var mathExprs []native.MathExpression

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Extract plain text for search indexing simultaneously
		switch n.Kind() {
		case passthrough.KindPassthroughInline:
			m := n.(*passthrough.PassthroughInline)
			latex := string(m.Segment.Value(reader.Source()))
			hash := native.HashContent("math-inline", latex)
			mathExprs = append(mathExprs, native.MathExpression{LaTeX: latex, DisplayMode: false, Hash: hash})
		case passthrough.KindPassthroughBlock:
			m := n.(*passthrough.PassthroughBlock)
			var lines strings.Builder
			l := m.Lines().Len()
			for i := 0; i < l; i++ {
				line := m.Lines().At(i)
				lines.Write(line.Value(reader.Source()))
			}
			latex := strings.TrimSpace(lines.String())
			hash := native.HashContent("math-block", latex)
			mathExprs = append(mathExprs, native.MathExpression{LaTeX: latex, DisplayMode: true, Hash: hash})
		case ast.KindText:
			t := n.(*ast.Text)
			plainText.Write(t.Segment.Value(reader.Source()))
			plainText.WriteString(" ")
		case ast.KindCodeBlock, ast.KindFencedCodeBlock:
			l := n.Lines().Len()
			for i := 0; i < l; i++ {
				line := n.Lines().At(i)
				plainText.Write(line.Value(reader.Source()))
			}
			plainText.WriteString(" ")
		case ast.KindHeading:
			plainText.WriteString("\n")

			// Handle TOC extraction
			heading := n.(*ast.Heading)
			if heading.Level >= 2 && heading.Level <= 6 {
				var headerText strings.Builder
				walker := func(child ast.Node, entering bool) (ast.WalkStatus, error) {
					if !entering {
						return ast.WalkContinue, nil
					}
					if child.Kind() == ast.KindText {
						textNode := child.(*ast.Text)
						headerText.Write(textNode.Segment.Value(reader.Source()))
					}
					return ast.WalkContinue, nil
				}
				_ = ast.Walk(heading, walker)

				id, _ := heading.AttributeString("id")
				if id != nil {
					toc = append(toc, models.TOCEntry{
						ID:    string(id.([]byte)),
						Text:  headerText.String(),
						Level: heading.Level,
					})
				}
			}
		}
		return ast.WalkContinue, nil
	})

	pc.Set(tocKey, toc)
	pc.Set(plainTextKey, plainText.String())
	pc.Set(mathExpressionsKey, mathExprs)
}
