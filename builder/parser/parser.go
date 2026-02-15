// Configures the markdown parser and URL transformation logic
package parser

import (
	"strings"
	"sync"

	chroma_html "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	admonitions "github.com/stefanfritsch/goldmark-admonitions"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

func codeBlockWrapper(w util.BufWriter, c highlighting.CodeBlockContext, entering bool) {
	if entering {
		langBytes, _ := c.Language()
		lang := string(langBytes)
		if lang == "" {
			lang = "text"
		}

		title := ""
		if attrs := c.Attributes(); attrs != nil {
			if t, ok := attrs.Get([]byte("title")); ok {
				if b, ok := t.([]byte); ok {
					title = string(b)
				} else if s, ok := t.(string); ok {
					title = s
				}
			}
		}

		// Write the wrapper div with data-lang attribute
		_, _ = w.WriteString(`<div class="code-block-container">`)
		if title != "" {
			_, _ = w.WriteString(`<div class="code-header">` + title + `</div>`)
		}
		_, _ = w.WriteString(`<div class="code-wrapper" data-lang="` + lang + `">`)
	} else {
		_, _ = w.WriteString(`</div></div>`)
	}
}

// ExtractPlainText walks the AST and returns a clean string of all text content
func ExtractPlainText(node ast.Node, source []byte) string {
	var out strings.Builder
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n.Kind() {
		case ast.KindText:
			t := n.(*ast.Text)
			out.Write(t.Segment.Value(source))
			out.WriteString(" ")
		case ast.KindCodeBlock, ast.KindFencedCodeBlock:
			// Include code blocks in search
			l := n.Lines().Len()
			for i := 0; i < l; i++ {
				line := n.Lines().At(i)
				out.Write(line.Value(source))
			}
			out.WriteString(" ")
		case ast.KindHeading:
			// Ensure headings are separated
			out.WriteString("\n")
		}
		return ast.WalkContinue, nil
	})
	return out.String()
}

// New creates a new Goldmark markdown parser with SSR support for diagrams
func New(baseURL string, renderer *native.Renderer, diagramCache *sync.Map) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			highlighting.NewHighlighting(
				highlighting.WithStyle("nord"),
				highlighting.WithFormatOptions(
					chroma_html.WithClasses(true),
				),
				highlighting.WithWrapperRenderer(codeBlockWrapper),
			),
			passthrough.New(passthrough.Config{
				InlineDelimiters: []passthrough.Delimiters{{Open: "$", Close: "$"}, {Open: "\\(", Close: "\\)"}},
				BlockDelimiters:  []passthrough.Delimiters{{Open: "$$", Close: "$$"}, {Open: "\\[", Close: "\\]"}},
			}),
			&admonitions.Extender{},
		),
		goldmark.WithParserOptions(
			// Register Transformers
			parser.WithASTTransformers(
				util.Prioritized(&urlTransformer{BaseURL: baseURL}, 100),
				util.Prioritized(&tocTransformer{}, 200),
				util.Prioritized(&ssrTransformer{
					Renderer: renderer,
					Cache:    diagramCache,
				}, 50), // Run SSR early (lower priority = runs first)
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}
