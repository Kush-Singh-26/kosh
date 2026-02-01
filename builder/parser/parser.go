// Configures the markdown parser and URL transformation logic
package parser

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	chroma_html "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"my-ssg/builder/models"
	"my-ssg/builder/renderer/headless"
)

func codeBlockWrapper(w util.BufWriter, c highlighting.CodeBlockContext, entering bool) {
	if entering {
		langBytes, _ := c.Language()
		lang := string(langBytes)
		if lang == "" {
			lang = "text"
		}
		// Write the wrapper div with data-lang attribute
		_, _ = w.WriteString(`<div class="code-wrapper" data-lang="` + lang + `">`)
	} else {
		_, _ = w.WriteString(`</div>`)
	}
}

// --- TOC Logic Start ---
var tocKey = parser.NewContextKey()
var mermaidKey = parser.NewContextKey()
var mermaidSVGKey = parser.NewContextKey()     // Stores map[string]MermaidSVGPair of code -> {light, dark} SVGs
var mermaidOrderedKey = parser.NewContextKey() // Stores []MermaidSVGPair in order of appearance

func GetTOC(pc parser.Context) []models.TOCEntry {
	if v := pc.Get(tocKey); v != nil {
		return v.([]models.TOCEntry)
	}
	return nil
}

func HasMermaid(pc parser.Context) bool {
	return pc.Get(mermaidKey) != nil
}

// GetMermaidSVGPairMap retrieves the rendered SVG pairs from the parser context
func GetMermaidSVGPairMap(pc parser.Context) map[string]MermaidSVGPair {
	if v := pc.Get(mermaidSVGKey); v != nil {
		return v.(map[string]MermaidSVGPair)
	}
	return nil
}

// GetMermaidSVGPairSlice retrieves the ordered slice of SVG pairs from the parser context
func GetMermaidSVGPairSlice(pc parser.Context) []MermaidSVGPair {
	if v := pc.Get(mermaidOrderedKey); v != nil {
		return v.([]MermaidSVGPair)
	}
	return nil
}

// GetMermaidSVGs retrieves the rendered SVGs from the parser context (legacy support)
func GetMermaidSVGs(pc parser.Context) map[string]string {
	if v := pc.Get(mermaidSVGKey); v != nil {
		// Convert pair map to single SVG map (using dark theme as default)
		pairMap := v.(map[string]MermaidSVGPair)
		svgMap := make(map[string]string)
		for code, pair := range pairMap {
			svgMap[code] = pair.Dark
		}
		return svgMap
	}
	return nil
}

// mermaidPreRegex matches mermaid code blocks (both <pre class=mermaid> and <div class=code-wrapper data-lang=mermaid>)
var mermaidPreRegex = regexp.MustCompile(`(?s)<(?:pre|div) class=["']?(?:mermaid|code-wrapper)["']?(?: data-lang=["']?mermaid["']?)?>(.*?)</(?:pre|div)>`)

// ReplaceMermaidBlocks replaces mermaid pre blocks with rendered SVG
func ReplaceMermaidBlocks(html string, svgs map[string]string) string {
	if len(svgs) == 0 {
		return html
	}

	// Convert map to slice of SVGs for ordered replacement
	svgList := make([]string, 0, len(svgs))
	for _, svg := range svgs {
		svgList = append(svgList, svg)
	}

	svgIndex := 0

	// Replace all mermaid pre blocks with rendered SVGs
	return mermaidPreRegex.ReplaceAllStringFunc(html, func(match string) string {
		if svgIndex >= len(svgList) {
			return match // No more SVGs to use
		}

		svg := svgList[svgIndex]
		svgIndex++

		return fmt.Sprintf(`<div class="mermaid-container">%s</div>`, svg)
	})
}

// ReplaceMermaidBlocksWithThemeSupport replaces mermaid blocks with both light and dark SVGs
// The browser will show/hide based on the data-theme attribute
func ReplaceMermaidBlocksWithThemeSupport(html string, pairs []MermaidSVGPair) string {
	if len(pairs) == 0 {
		return html
	}

	pairIndex := 0

	// Replace all mermaid pre blocks with dual-themed SVGs in order
	return mermaidPreRegex.ReplaceAllStringFunc(html, func(match string) string {
		if pairIndex >= len(pairs) {
			return match // No more pairs to use
		}

		pair := pairs[pairIndex]
		pairIndex++

		// Output container with both light and dark versions
		// CSS will show/hide based on data-theme attribute
		return fmt.Sprintf(`<div class="mermaid-container" data-diagram="true"><div class="mermaid-light">%s</div><div class="mermaid-dark">%s</div></div>`,
			pair.Light, pair.Dark)
	})
}

// --- LaTeX SSR ---

// Regex patterns for LaTeX math
var (
	// Block math: $$...$$ (greedy but stops at first $$)
	blockMathRegex = regexp.MustCompile(`\$\$([^\$]+)\$\$`)
	// Inline math: $...$ (non-greedy, excludes $$ and common false positives)
	inlineMathRegex = regexp.MustCompile(`(?:^|[^$])\$([^$\n]+?)\$(?:[^$]|$)`)
)

// ExtractMathExpressions finds all LaTeX expressions in HTML and returns them with metadata
func ExtractMathExpressions(html string) []headless.MathExpression {
	var expressions []headless.MathExpression
	seen := make(map[string]bool) // Deduplicate

	// Extract block math ($$...$$)
	for _, match := range blockMathRegex.FindAllStringSubmatch(html, -1) {
		if len(match) >= 2 {
			latex := match[1]
			hash := headless.HashContent("math-block", latex)
			if !seen[hash] {
				seen[hash] = true
				expressions = append(expressions, headless.MathExpression{
					LaTeX:       latex,
					DisplayMode: true,
					Hash:        hash,
				})
			}
		}
	}

	// Extract inline math ($...$)
	for _, match := range inlineMathRegex.FindAllStringSubmatch(html, -1) {
		if len(match) >= 2 {
			latex := match[1]
			// Skip if it looks like currency (e.g., $5, $10.00)
			if regexp.MustCompile(`^\d`).MatchString(latex) {
				continue
			}
			hash := headless.HashContent("math-inline", latex)
			if !seen[hash] {
				seen[hash] = true
				expressions = append(expressions, headless.MathExpression{
					LaTeX:       latex,
					DisplayMode: false,
					Hash:        hash,
				})
			}
		}
	}

	return expressions
}

// ReplaceMathExpressions replaces LaTeX expressions in HTML with rendered KaTeX output
func ReplaceMathExpressions(html string, rendered map[string]string, cache map[string]string, cacheMu *sync.Mutex) string {
	if len(rendered) == 0 && len(cache) == 0 {
		return html
	}

	result := html

	// Replace block math ($$...$$)
	result = blockMathRegex.ReplaceAllStringFunc(result, func(match string) string {
		submatch := blockMathRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		latex := submatch[1]
		hash := headless.HashContent("math-block", latex)

		// Check rendered first, then cache
		if html, ok := rendered[hash]; ok {
			// Add to cache
			if cacheMu != nil {
				cacheMu.Lock()
				cache[hash] = html
				cacheMu.Unlock()
			}
			return fmt.Sprintf(`<span class="katex-display">%s</span>`, html)
		}
		if html, ok := cache[hash]; ok {
			return fmt.Sprintf(`<span class="katex-display">%s</span>`, html)
		}
		return match // Keep original if not rendered
	})

	// Replace inline math ($...$)
	result = inlineMathRegex.ReplaceAllStringFunc(result, func(match string) string {
		submatch := inlineMathRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		latex := submatch[1]

		// Skip currency patterns
		if regexp.MustCompile(`^\d`).MatchString(latex) {
			return match
		}

		hash := headless.HashContent("math-inline", latex)

		// Extract prefix/suffix characters that were captured
		prefix := ""
		suffix := ""
		if len(match) > 0 && match[0] != '$' {
			prefix = string(match[0])
		}
		if len(match) > 0 && match[len(match)-1] != '$' {
			suffix = string(match[len(match)-1])
		}

		// Check rendered first, then cache
		if html, ok := rendered[hash]; ok {
			if cacheMu != nil {
				cacheMu.Lock()
				cache[hash] = html
				cacheMu.Unlock()
			}
			return prefix + fmt.Sprintf(`<span class="katex-inline">%s</span>`, html) + suffix
		}
		if html, ok := cache[hash]; ok {
			return prefix + fmt.Sprintf(`<span class="katex-inline">%s</span>`, html) + suffix
		}
		return match
	})

	return result
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML
func RenderMathForHTML(html string, renderer *headless.Orchestrator, cache map[string]string, cacheMu *sync.Mutex) string {
	expressions := ExtractMathExpressions(html)
	if len(expressions) == 0 {
		return html
	}

	// Lock cache for reading
	cacheMu.Lock()
	cachedCopy := make(map[string]string)
	for k, v := range cache {
		cachedCopy[k] = v
	}
	cacheMu.Unlock()

	// Render uncached expressions (cache checking now happens inside RenderAllMath)
	rendered, err := renderer.RenderAllMath(expressions, cachedCopy)
	if err != nil {
		log.Printf("   ⚠️  LaTeX batch render failed: %v", err)
	}

	// Replace in HTML
	return ReplaceMathExpressions(html, rendered, cache, cacheMu)
}

// PreRenderMathForAllPosts scans all posts and pre-renders all LaTeX expressions in one batch
// This eliminates the need to open multiple Chrome tabs during post processing
func PreRenderMathForAllPosts(files []string, renderer *headless.Orchestrator, cache map[string]string, cacheMu *sync.Mutex) error {
	// Collect all unique math expressions from all files
	allExpressions := make(map[string]headless.MathExpression)

	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Extract expressions from raw markdown
		expressions := ExtractMathExpressions(string(source))
		for _, expr := range expressions {
			allExpressions[expr.Hash] = expr
		}
	}

	if len(allExpressions) == 0 {
		return nil
	}

	// Convert map to slice
	expressionList := make([]headless.MathExpression, 0, len(allExpressions))
	for _, expr := range allExpressions {
		expressionList = append(expressionList, expr)
	}

	// Check cache first
	cacheMu.Lock()
	uncachedCount := 0
	for _, expr := range expressionList {
		if _, exists := cache[expr.Hash]; !exists {
			uncachedCount++
		}
	}
	cacheMu.Unlock()

	if uncachedCount == 0 {
		return nil
	}

	// Render all expressions in one batch
	results, err := renderer.RenderAllMath(expressionList, cache)
	if err != nil {
		return fmt.Errorf("batch LaTeX render failed: %w", err)
	}

	// Store results in cache
	cacheMu.Lock()
	for hash, html := range results {
		cache[hash] = html
	}
	cacheMu.Unlock()

	return nil
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

// --- SSR Transformer for Mermaid and LaTeX ---

// SSRTransformer handles server-side rendering of Mermaid diagrams and LaTeX math
type SSRTransformer struct {
	Renderer *headless.Orchestrator
	Cache    map[string]string
	CacheMu  *sync.Mutex
}

// MermaidSVGPair stores both light and dark versions of a diagram
type MermaidSVGPair struct {
	Light string
	Dark  string
}

func (t *SSRTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Handle Mermaid code blocks
		if n.Kind() == ast.KindFencedCodeBlock {
			fcb := n.(*ast.FencedCodeBlock)
			lang := string(fcb.Language(source))

			if lang == "mermaid" {
				// Extract the mermaid code
				var codeBuilder bytes.Buffer
				lines := fcb.Lines()
				for i := 0; i < lines.Len(); i++ {
					line := lines.At(i)
					codeBuilder.Write(line.Value(source))
				}
				code := strings.TrimSpace(codeBuilder.String())

				if code == "" {
					return ast.WalkContinue, nil
				}

				// Mark that we have mermaid content
				pc.Set(mermaidKey, true)

				// Check cache for both themes
				hash := headless.HashContent("mermaid", code)
				lightHash := hash + "_light"
				darkHash := hash + "_dark"

				t.CacheMu.Lock()
				lightCached, lightExists := t.Cache[lightHash]
				darkCached, darkExists := t.Cache[darkHash]
				t.CacheMu.Unlock()

				var pair MermaidSVGPair

				if lightExists && darkExists {
					pair.Light = lightCached
					pair.Dark = darkCached
					log.Printf("   📦 Using cached Mermaid diagram (both themes)")
				} else {
					// Render using headless Chrome - both light and dark themes
					var lightSVG, darkSVG string
					var err error

					// Render light theme
					lightSVG, err = t.Renderer.RenderMermaid(code, "default")
					if err != nil {
						log.Printf("   ⚠️  Mermaid light theme render failed: %v", err)
						return ast.WalkContinue, nil
					}

					// Render dark theme
					darkSVG, err = t.Renderer.RenderMermaid(code, "dark")
					if err != nil {
						log.Printf("   ⚠️  Mermaid dark theme render failed: %v", err)
						return ast.WalkContinue, nil
					}

					pair.Light = lightSVG
					pair.Dark = darkSVG
					log.Printf("   🎨 Rendered Mermaid diagram (both themes)")

					// Cache both results
					t.CacheMu.Lock()
					t.Cache[lightHash] = lightSVG
					t.Cache[darkHash] = darkSVG
					t.CacheMu.Unlock()
				}

				// Store the code->SVG pair for post-processing (map for cache lookup)
				pairMap := GetMermaidSVGPairMap(pc)
				if pairMap == nil {
					pairMap = make(map[string]MermaidSVGPair)
					pc.Set(mermaidSVGKey, pairMap)
				}
				pairMap[code] = pair

				// Also store in ordered slice to maintain diagram order
				orderedPairs := GetMermaidSVGPairSlice(pc)
				orderedPairs = append(orderedPairs, pair)
				pc.Set(mermaidOrderedKey, orderedPairs)
			}
		}

		return ast.WalkContinue, nil
	})
}

// MermaidDetector just detects if mermaid exists (for HasMermaid flag)
type MermaidDetector struct{}

func (t *MermaidDetector) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Check for code blocks with "mermaid" language
		if n.Kind() == ast.KindFencedCodeBlock {
			fcb := n.(*ast.FencedCodeBlock)
			lang := string(fcb.Language(reader.Source()))
			if lang == "mermaid" {
				pc.Set(mermaidKey, true)
				return ast.WalkStop, nil
			}
		}

		return ast.WalkContinue, nil
	})
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

			// Capture Heading Levels 2, 3, 4, 5, 6 (Skipping H1 as it is the page title)
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

// --- TOC Logic End ---

// URLTransformer intercepts links and images to rewrite URLs (e.g., .md -> .html).
type URLTransformer struct {
	BaseURL string
}

func (t *URLTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch target := n.(type) {
		case *ast.Link:
			t.processDestination(target, target.Destination)
		case *ast.Image:
			t.processDestination(target, target.Destination)
		}
		return ast.WalkContinue, nil
	})
}

func (t *URLTransformer) processDestination(n ast.Node, dest []byte) {
	href := string(dest)

	// Handle External Links
	if strings.HasPrefix(href, "http") {
		if _, isLink := n.(*ast.Link); isLink {
			n.SetAttribute([]byte("target"), []byte("_blank"))
			n.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	} else {
		ext := strings.ToLower(filepath.Ext(href))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			href = href[:len(href)-len(ext)] + ".webp"
			switch node := n.(type) {
			case *ast.Link:
				node.Destination = []byte(href)
			case *ast.Image:
				node.Destination = []byte(href)
			}
		}
	}

	if strings.HasSuffix(href, ".md") && !strings.HasPrefix(href, "http") {
		href = strings.Replace(href, ".md", ".html", 1)
		href = strings.ToLower(href)
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = []byte(href)
		case *ast.Image:
			node.Destination = []byte(href)
		}
	}

	if _, isImage := n.(*ast.Image); isImage {
		n.SetAttribute([]byte("loading"), []byte("lazy"))
	}

	if strings.HasPrefix(href, "/") && t.BaseURL != "" {
		newDest := []byte(t.BaseURL + href)
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = newDest
		case *ast.Image:
			node.Destination = newDest
		}
	}
}

// New creates a new Goldmark markdown parser with SSR support for diagrams
// theme parameter controls Mermaid diagram rendering ("dark" or "light")
func New(baseURL string, renderer *headless.Orchestrator, diagramCache map[string]string, cacheMu *sync.Mutex) goldmark.Markdown {
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
			// Note: Removed client-side mermaid.Extender - using SSRTransformer instead
		),
		goldmark.WithParserOptions(
			// Register Transformers
			parser.WithASTTransformers(
				util.Prioritized(&URLTransformer{BaseURL: baseURL}, 100),
				util.Prioritized(&TOCTransformer{}, 200),
				util.Prioritized(&MermaidDetector{}, 300),
				util.Prioritized(&SSRTransformer{
					Renderer: renderer,
					Cache:    diagramCache,
					CacheMu:  cacheMu,
				}, 50), // Run SSR early (lower priority = runs first)
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}
