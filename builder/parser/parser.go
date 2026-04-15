// Package parser provides markdown parsing and URL transformation logic.
package parser

import (
	chroma_html "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
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

// Options controls the configuration of the markdown parser.
type Options struct {
	Renderer     *native.Renderer
	DiagramCache SSRMap
	D2Group      *singleflight.Group
}

// Option is a functional option for configuring the parser.
type Option func(*Options)

// WithRenderer sets the native renderer for the parser.
func WithRenderer(renderer *native.Renderer) Option {
	return func(o *Options) {
		o.Renderer = renderer
	}
}

// WithDiagramCache sets the diagram cache for the parser.
func WithDiagramCache(cache SSRMap) Option {
	return func(o *Options) {
		o.DiagramCache = cache
	}
}

// WithD2Group sets the singleflight group for D2 rendering.
func WithD2Group(group *singleflight.Group) Option {
	return func(o *Options) {
		o.D2Group = group
	}
}

// New creates a new Goldmark markdown parser with SSR support for diagrams
func New(cfg *config.Config, opts ...Option) goldmark.Markdown {
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

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
		),
		goldmark.WithParserOptions(
			// Register Unified Transformer
			parser.WithASTTransformers(
				util.Prioritized(&unifiedTransformer{
					BaseURL:  baseURL,
					Compress: compress,
					Renderer: options.Renderer,
					Cache:    options.DiagramCache,
					D2Group:  options.D2Group,
					IsDev:    cfg.IsDev,
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
