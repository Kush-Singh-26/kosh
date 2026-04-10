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

const (
	unifiedTransformerPriority = 100
	rawHTMLRendererPriority    = 500
)

func codeBlockWrapper(writer util.BufWriter, codeCtx highlighting.CodeBlockContext, entering bool) {
	if entering {
		langBytes, _ := codeCtx.Language()
		lang := string(langBytes)
		if lang == "" {
			lang = "text"
		}

		title := ""
		if attrs := codeCtx.Attributes(); attrs != nil {
			if titleVal, ok := attrs.Get([]byte("title")); ok {
				if titleBytes, ok := titleVal.([]byte); ok {
					title = string(titleBytes)
				} else if titleStr, ok := titleVal.(string); ok {
					title = titleStr
				}
			}
		}

		// Write the wrapper div with data-lang attribute
		_, _ = writer.WriteString(`<div class="code-block-container">`)
		if title != "" {
			_, _ = writer.WriteString(`<div class="code-header">` + title + `</div>`)
		}
		_, _ = writer.WriteString(`<div class="code-wrapper" data-lang="` + lang + `">`)
	} else {
		_, _ = writer.WriteString(`</div></div>`)
	}
}

// New creates a new Goldmark markdown parser with SSR support for diagrams
func New(cfg *config.Config, renderer *native.Renderer, diagramCache SSRMap, d2Group *singleflight.Group) goldmark.Markdown {
	baseURL := cfg.BaseURL
	compress := cfg.ShouldCompressImages

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
				}, unifiedTransformerPriority),
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			goldmarkRenderer.WithNodeRenderers(
				util.Prioritized(&rawHTMLBlockRenderer{}, rawHTMLRendererPriority),
			),
		),
	)
}
