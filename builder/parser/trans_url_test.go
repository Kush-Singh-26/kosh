package parser

import (
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func TestURLTransformer_Basic(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedLink string
	}{
		{
			name:         "external link",
			input:        "[Example](https://example.com)",
			expectedLink: "https://example.com",
		},
		{
			name:         "markdown to html conversion",
			input:        "[Post](post.md)",
			expectedLink: "post.html",
		},
		{
			name:         "lowercase conversion",
			input:        "[Post](MyPost.md)",
			expectedLink: "mypost.html",
		},
		{
			name:         "trim leading dot",
			input:        "[Post](./post.md)",
			expectedLink: "post.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := goldmark.New(
				goldmark.WithParserOptions(
					parser.WithASTTransformers(
						util.Prioritized(&unifiedTransformer{}, 100),
					),
				),
			)

			context := parser.NewContext()
			reader := text.NewReader([]byte(tt.input))
			doc := md.Parser().Parse(reader, parser.WithContext(context))

			var foundLink string
			_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if !entering {
					return ast.WalkContinue, nil
				}
				if link, ok := n.(*ast.Link); ok {
					foundLink = string(link.Destination)
				}
				return ast.WalkContinue, nil
			})

			if foundLink != tt.expectedLink {
				t.Errorf("link destination = %q, want %q", foundLink, tt.expectedLink)
			}
		})
	}
}
