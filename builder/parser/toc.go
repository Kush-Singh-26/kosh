package parser

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"my-ssg/builder/models"
)

var tocKey = parser.NewContextKey()
var d2SVGKey = parser.NewContextKey()
var d2OrderedKey = parser.NewContextKey()

func GetTOC(pc parser.Context) []models.TOCEntry {
	if v := pc.Get(tocKey); v != nil {
		return v.([]models.TOCEntry)
	}
	return nil
}

func GetD2SVGPairMap(pc parser.Context) map[string]D2SVGPair {
	if v := pc.Get(d2SVGKey); v != nil {
		return v.(map[string]D2SVGPair)
	}
	return nil
}

func GetD2SVGPairSlice(pc parser.Context) []D2SVGPair {
	if v := pc.Get(d2OrderedKey); v != nil {
		return v.([]D2SVGPair)
	}
	return nil
}

type TOCTransformer struct{}

func (t *TOCTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	var toc []models.TOCEntry

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if n.Kind() == ast.KindHeading {
			heading := n.(*ast.Heading)
			if heading.Level < 2 || heading.Level > 6 {
				return ast.WalkContinue, nil
			}

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
		return ast.WalkContinue, nil
	})

	pc.Set(tocKey, toc)
}
