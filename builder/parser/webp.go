package parser

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type webpTransformer struct {
	Compress bool
}

func (t *webpTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if n.Kind() == ast.KindImage {
			img := n.(*ast.Image)
			src := string(img.Destination)

			// Only transform local images if compression is enabled
			if t.Compress && !strings.HasPrefix(src, "http") && !strings.HasPrefix(src, "//") {
				lowerSrc := strings.ToLower(src)
				if strings.HasSuffix(lowerSrc, ".jpg") || strings.HasSuffix(lowerSrc, ".jpeg") || strings.HasSuffix(lowerSrc, ".png") {
					// Swap extension to .webp
					idx := strings.LastIndex(src, ".")
					if idx != -1 {
						img.Destination = []byte(src[:idx] + ".webp")
					}
				}
			}
		}

		return ast.WalkContinue, nil
	})
}
