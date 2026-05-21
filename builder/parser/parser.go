// Package parser provides markdown parsing and URL transformation logic.
package parser

import (
	"regexp"
	"strings"

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
		originalLang := string(langBytes)
		lang := originalLang
		if lang == "" {
			lang = "text"
		}

		title, hideLang := parseCodeAttributes(codeCtx)

		// Write the header bar
		_, _ = writer.WriteString(`<div class="code-block-container">`)
		writeCodeHeader(writer, originalLang, title, hideLang)

		_, _ = writer.WriteString(`<div class="code-wrapper" data-lang="` + lang + `">`)

		// If no language is provided, goldmark-highlighting often doesn't wrap it in <pre><code>
		// We add a fallback class but let the highlighter do its work if it can.
		if originalLang == "" {
			_, _ = writer.WriteString(`<pre class="chroma"><code>`)
		}
	} else {
		langBytes, _ := codeCtx.Language()
		if len(langBytes) == 0 {
			_, _ = writer.WriteString(`</code></pre>`)
		}
		_, _ = writer.WriteString(`</div></div>`)
	}
}

func parseCodeAttributes(codeCtx highlighting.CodeBlockContext) (string, bool) {
	title := ""
	hideLang := false
	if attrs := codeCtx.Attributes(); attrs != nil {
		// Check for title
		if titleVal, ok := attrs.Get([]byte("title")); ok {
			if titleBytes, ok := titleVal.([]byte); ok {
				title = string(titleBytes)
			} else if titleStr, ok := titleVal.(string); ok {
				title = titleStr
			}
		}
		// Check for nolang or hide-lang
		if _, ok := attrs.Get([]byte("nolang")); ok {
			hideLang = true
		} else if _, ok := attrs.Get([]byte("hide-lang")); ok {
			hideLang = true
		}

		// Check for nolang or hide-lang in classes (e.g. {.nolang})
		if classVal, ok := attrs.Get([]byte("class")); ok {
			classStr := ""
			if classBytes, ok := classVal.([]byte); ok {
				classStr = string(classBytes)
			} else if s, ok := classVal.(string); ok {
				classStr = s
			}
			if strings.Contains(classStr, "nolang") || strings.Contains(classStr, "hide-lang") {
				hideLang = true
			}
		}
	}
	return title, hideLang
}

func writeCodeHeader(writer util.BufWriter, originalLang, title string, hideLang bool) {
	_, _ = writer.WriteString(`<div class="code-header-bar">`)
	_, _ = writer.WriteString(`<div class="code-header-left">`)

	// Only show language label if it's explicit and not hidden
	if originalLang != "" && !hideLang {
		_, _ = writer.WriteString(`<span class="code-lang-label">` + strings.ToUpper(originalLang) + `</span>`)
		if title != "" {
			_, _ = writer.WriteString(`<span class="code-header-divider"></span>`)
		}
	}

	if title != "" {
		_, _ = writer.WriteString(`<span class="code-header-title">` + title + `</span>`)
	}
	_, _ = writer.WriteString(`</div>`)
	_, _ = writer.WriteString(`<button class="copy-btn-explicit" aria-label="Copy code">`)
	_, _ = writer.WriteString(`<span class="copy-text">Copy</span>`)
	_, _ = writer.WriteString(`</button>`)
	_, _ = writer.WriteString(`</div>`)
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

// StripRegistryComments removes Kosh internal registry comments from HTML content.
func StripRegistryComments(html string) string {
	// Pattern to match all KOSH registry comments
	re := regexp.MustCompile(`<!--KOSH_(MATH|D2|TOC|SEARCH)_REG:[^>]+-->`)
	return re.ReplaceAllString(html, "")
}

// New creates a new Goldmark markdown parser with SSR support for diagrams.
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
			parser.WithAttribute(),
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			goldmarkRenderer.WithNodeRenderers(
				util.Prioritized(&rawHTMLBlockRenderer{}, rawHTMLRendererPriority),
				util.Prioritized(newHeadingAnchorRenderer(), headingAnchorRendererPriority),
			),
		),
	)
}
