package parser

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// Package-level context keys, isolated by being unexported.
var (
	tocKey          = parser.NewContextKey()
	plainTextKey    = parser.NewContextKey()
	ssrHashesKey    = parser.NewContextKey()
	contextKeyBuild = parser.NewContextKey()
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

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Extract plain text for search indexing simultaneously
		switch n.Kind() {
		case ast.KindText:
			t := n.(*ast.Text)
			plainText.Write(t.Segment.Value(reader.Source()))
			plainText.WriteString(" ")
		case ast.KindCodeBlock, ast.KindFencedCodeBlock:
			l := n.Lines().Len()
			for i := range l {
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
}
