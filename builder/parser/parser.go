// Configures the markdown parser and URL transformation logic
package parser

import (
	chroma_html "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	admonitions "github.com/stefanfritsch/goldmark-admonitions"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	goldmarkRenderer "github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"golang.org/x/sync/singleflight"

	"github.com/Kush-Singh-26/kosh/builder/config"
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

// New creates a new Goldmark markdown parser with SSR support for diagrams
func New(cfg *config.Config, renderer *native.Renderer, diagramCache SSRMap, d2Group *singleflight.Group) goldmark.Markdown {
	baseURL := cfg.BaseURL
	compress := cfg.CompressImages

	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
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
			// Register Unified Transformer
			parser.WithASTTransformers(
				util.Prioritized(&unifiedTransformer{
					BaseURL:  baseURL,
					Compress: compress,
					Renderer: renderer,
					Cache:    diagramCache,
					D2Group:  d2Group,
				}, 100),
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			goldmarkRenderer.WithNodeRenderers(
				util.Prioritized(&rawHTMLBlockRenderer{}, 500),
			),
		),
	)
}
