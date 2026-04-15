package parser

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"golang.org/x/sync/singleflight"
)

const (
	minHeadingLevel = 2
	maxHeadingLevel = 6
)

// slugify creates a URL-safe slug from text (same logic as goldmark's auto-heading-ID)
func slugify(s string) string {
	s = strings.ToLower(s)
	var buf strings.Builder
	buf.Grow(len(s))
	for _, r := range s {
		if ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			buf.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			buf.WriteRune('-')
		}
	}
	return buf.String()
}

// SSRMap provides a key-value interface for server-side rendered content.
// This allows the transformer to use either a raw sync.Map or a persistent
// cache adapter.
type SSRMap interface {
	Load(key string) (any, bool)
	Store(key string, value any)
}

// MemorySSRMap is a thread-safe in-memory implementation of SSRMap
// using sync.Map.
type MemorySSRMap struct {
	m sync.Map // key: string, value: string or models.SSRThemePair
}

// NewMemorySSRMap creates a new in-memory SSR map.
func NewMemorySSRMap() *MemorySSRMap {
	return &MemorySSRMap{}
}

// Load returns a cached SSR value by key.
func (m *MemorySSRMap) Load(key string) (any, bool) {
	return m.m.Load(key)
}

// Store caches an SSR value by key.
func (m *MemorySSRMap) Store(key string, value any) {
	m.m.Store(key, value)
}

type unifiedTransformer struct {
	BaseURL  string
	Compress bool
	Renderer *native.Renderer
	Cache    SSRMap
	D2Group  *singleflight.Group
	A11yMap  sync.Map // key: string, value: string (a11y warning)
	IsDev    bool
}

type replacement struct {
	old ast.Node
	new ast.Node
}

type transformContext struct {
	inHeading    bool
	headingLevel int
	headingID    string
	headingText  strings.Builder
}

type transformState struct {
	source          []byte
	toc             []models.TOCEntry
	plainText       strings.Builder
	d2Blocks        []d2BlockInfo
	mathExpressions []models.MathExpression
	toReplace       []replacement
	ctx             *transformContext
	pc              parser.Context
	transformer     *unifiedTransformer
}

var (
	transformStatePool = sync.Pool{
		New: func() any {
			return &transformState{
				ctx: &transformContext{},
			}
		},
	}
)

func (s *transformState) reset(transformer *unifiedTransformer, source []byte, pc parser.Context) {
	s.source = source
	s.transformer = transformer
	s.pc = pc
	s.toc = s.toc[:0]
	s.d2Blocks = s.d2Blocks[:0]
	s.mathExpressions = s.mathExpressions[:0]
	s.toReplace = s.toReplace[:0]
	s.plainText.Reset()
	s.ctx.inHeading = false
	s.ctx.headingLevel = 0
	s.ctx.headingID = ""
	s.ctx.headingText.Reset()
}

// Transform walks the AST and extracts TOC, plaintext, math, and D2 data.
func (t *unifiedTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	state := transformStatePool.Get().(*transformState)
	state.reset(t, reader.Source(), pc)
	defer transformStatePool.Put(state)

	_ = ast.Walk(node, state.walkFunc)

	// Post-walk: Collect D2 expressions and add placeholders
	if len(state.d2Blocks) > 0 {
		d2Exprs := make([]models.D2Expression, len(state.d2Blocks))
		for i, block := range state.d2Blocks {
			d2Exprs[i] = models.D2Expression{
				Code: block.code,
				Hash: block.hash,
			}
			placeholder := "<!--KOSH_D2:" + block.hash + "-->"
			state.toReplace = append(state.toReplace, replacement{
				old: block.node,
				new: &RawHTMLBlock{Content: []byte(placeholder)},
			})
		}
		pc.Set(d2ExpressionsKey, d2Exprs)
	}

	// Apply all replacements
	for _, r := range state.toReplace {
		parent := r.old.Parent()
		if parent != nil {
			parent.ReplaceChild(parent, r.old, r.new)
		}
	}

	// Extract results
	pc.Set(tocKey, state.toc)
	pc.Set(plainTextKey, state.plainText.String())
	if len(state.mathExpressions) > 0 {
		pc.Set(mathExpressionsKey, state.mathExpressions)
	}
}

func (s *transformState) walkFunc(n ast.Node, entering bool) (ast.WalkStatus, error) {
	kind := n.Kind()

	// 1. Handle Headings
	if kind == ast.KindHeading {
		s.handleHeading(n.(*ast.Heading), entering)
		return ast.WalkContinue, nil
	}

	// 2. Collection (Text/Code)
	if entering {
		s.collectText(n)
	}

	// Don't process children if exiting
	if !entering {
		return ast.WalkContinue, nil
	}

	// 3. Links and Images
	if kind == ast.KindLink || kind == ast.KindImage {
		s.handleLinkOrImage(n, kind)
	}

	// 4. Raw HTML
	if kind == ast.KindHTMLBlock || kind == ast.KindRawHTML {
		s.handleHTML(n, kind)
	}

	// 5. D2 Diagrams
	if kind == ast.KindFencedCodeBlock {
		s.handleD2(n.(*ast.FencedCodeBlock))
	}

	// 6. Math (LaTeX)
	if kind == passthrough.KindPassthroughInline || kind == passthrough.KindPassthroughBlock {
		return s.handleMath(n, kind)
	}

	return ast.WalkContinue, nil
}

func (s *transformState) handleHeading(heading *ast.Heading, entering bool) {
	if entering {
		if heading.Level >= minHeadingLevel && heading.Level <= maxHeadingLevel {
			s.ctx.inHeading = true
			s.ctx.headingLevel = heading.Level
			s.ctx.headingText.Reset()
			s.ctx.headingID = "" // Will be generated on exit
			id, _ := heading.AttributeString("id")
			if idBytes, ok := id.([]byte); ok {
				s.ctx.headingID = string(idBytes)
			}
		}
	} else {
		if s.ctx.inHeading {
			if s.ctx.headingID == "" {
				s.ctx.headingID = slugify(s.ctx.headingText.String())
			}
			heading.SetAttributeString("id", []byte(s.ctx.headingID))
			if s.ctx.headingID != "" {
				s.toc = append(s.toc, models.TOCEntry{
					ID:    s.ctx.headingID,
					Text:  s.ctx.headingText.String(),
					Level: s.ctx.headingLevel,
				})
			}
			s.ctx.inHeading = false
			s.ctx.headingLevel = 0
			s.ctx.headingID = ""
		}
	}
}

func (s *transformState) collectText(n ast.Node) {
	kind := n.Kind()
	switch kind {
	case ast.KindText:
		textNode := n.(*ast.Text)
		segment := textNode.Segment.Value(s.source)
		s.plainText.Write(segment)
		if s.ctx.inHeading {
			s.ctx.headingText.Write(segment)
		}
		s.plainText.WriteString(" ")
	case ast.KindCodeBlock, ast.KindFencedCodeBlock:
		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			s.plainText.Write(line.Value(s.source))
		}
		s.plainText.WriteString(" ")
	}
}

func (s *transformState) handleLinkOrImage(n ast.Node, kind ast.NodeKind) {
	var ln *ast.Link
	var img *ast.Image
	var isLink bool
	if kind == ast.KindLink {
		ln = n.(*ast.Link)
		isLink = true
	} else {
		img = n.(*ast.Image)
	}

	if isLink {
		s.transformer.processDestination(ln, ln.Destination, s.pc)
		s.checkLinkA11y(ln)
	} else {
		s.transformer.processDestination(img, img.Destination, s.pc)
		s.transformer.processImageDestination(img, img.Destination)
		img.SetAttribute([]byte("loading"), []byte("lazy"))
		s.checkImageA11y(img)
		s.handleImageCaption(img)
	}
}

func (s *transformState) checkLinkA11y(ln *ast.Link) {
	if s.transformer.IsDev {
		return
	}
	ariaLabel := getAttrValue(ln, "aria-label")
	hasText := hasTextChild(ln, s.source)
	if strings.TrimSpace(ariaLabel) == "" && !hasText {
		filePath, _ := s.pc.Get(ContextKeyFilePath).(string)
		href := string(ln.Destination)
		key := filePath + ":" + href
		if _, seen := s.transformer.A11yMap.LoadOrStore(key, true); !seen {
			slog.Warn("A11y Lint: Link has no text or aria-label", "file", filePath, "href", href)
		}
	}
}

func (s *transformState) checkImageA11y(img *ast.Image) {
	if s.transformer.IsDev {
		return
	}
	alt := string(img.Title)
	hasAlt := hasTextChild(img, s.source)
	if !hasAlt && strings.TrimSpace(alt) == "" {
		filePath, _ := s.pc.Get(ContextKeyFilePath).(string)
		src := string(img.Destination)
		key := filePath + ":img:" + src
		if _, seen := s.transformer.A11yMap.LoadOrStore(key, true); !seen {
			slog.Warn("A11y Lint: Image missing alt text", "file", filePath, "src", src)
		}
	}
}

func (s *transformState) handleImageCaption(img *ast.Image) {
	var altSB strings.Builder
	for child := img.FirstChild(); child != nil; child = child.NextSibling() {
		if tNode, ok := child.(*ast.Text); ok {
			altSB.Write(tNode.Segment.Value(s.source))
		}
	}
	altStr := strings.TrimSpace(altSB.String())

	if altStr != "" && strings.ToLower(altStr) != "image" {
		if parent := img.Parent(); parent != nil {
			if p, ok := parent.(*ast.Paragraph); ok {
				if s.isSoleImage(p, img) {
					src := string(img.Destination)
					figHTML := fmt.Sprintf("<figure><img src=\"%s\" alt=\"%s\" loading=\"lazy\"><figcaption>%s</figcaption></figure>", src, altStr, altStr)
					s.toReplace = append(s.toReplace, replacement{old: p, new: &RawHTMLBlock{Content: []byte(figHTML)}})
				}
			}
		}
	}
}

func (s *transformState) isSoleImage(p *ast.Paragraph, img *ast.Image) bool {
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		if c == img {
			continue
		}
		if tn, ok := c.(*ast.Text); ok {
			if strings.TrimSpace(string(tn.Segment.Value(s.source))) != "" {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func (s *transformState) handleHTML(n ast.Node, kind ast.NodeKind) {
	var htmlContent string
	if kind == ast.KindHTMLBlock {
		hb := n.(*ast.HTMLBlock)
		var sb strings.Builder
		for i := 0; i < hb.Lines().Len(); i++ {
			line := hb.Lines().At(i)
			sb.Write(line.Value(s.source))
		}
		htmlContent = sb.String()
	} else {
		ri := n.(*ast.RawHTML)
		var sb strings.Builder
		for i := 0; i < ri.Segments.Len(); i++ {
			seg := ri.Segments.At(i)
			sb.Write(seg.Value(s.source))
		}
		htmlContent = sb.String()
	}

	imgRe := regexp.MustCompile(`(?i)<img\b[^>]*?\balt=["'](.*?)["'][^>]*?>`)
	if imgRe.MatchString(htmlContent) {
		newHTML := imgRe.ReplaceAllStringFunc(htmlContent, func(imgTag string) string {
			matches := imgRe.FindStringSubmatch(imgTag)
			if len(matches) > 1 {
				alt := strings.TrimSpace(matches[1])
				if alt != "" && strings.ToLower(alt) != "image" {
					if !strings.Contains(htmlContent, "<figure") {
						return fmt.Sprintf("<figure>%s<figcaption>%s</figcaption></figure>", imgTag, alt)
					}
				}
			}
			return imgTag
		})

		if newHTML != htmlContent {
			if kind == ast.KindHTMLBlock {
				s.toReplace = append(s.toReplace, replacement{old: n, new: &RawHTMLBlock{Content: []byte(newHTML)}})
			} else {
				s.toReplace = append(s.toReplace, replacement{old: n, new: &RawHTMLInline{Content: []byte(newHTML)}})
			}
		}
	}
}

func (s *transformState) handleD2(cb *ast.FencedCodeBlock) {
	lang := string(cb.Language(s.source))
	if lang == "d2" {
		var lines strings.Builder
		l := cb.Lines().Len()
		for i := 0; i < l; i++ {
			line := cb.Lines().At(i)
			lines.Write(line.Value(s.source))
		}
		code := lines.String()
		hash := native.HashContent("d2", code)

		s.d2Blocks = append(s.d2Blocks, d2BlockInfo{node: cb, code: code, hash: hash})
		AddSSRHash(s.pc, hash)
	}
}

func (s *transformState) handleMath(n ast.Node, kind ast.NodeKind) (ast.WalkStatus, error) {
	var latex string
	var typeStr string
	var displayMode bool

	switch kind {
	case passthrough.KindPassthroughInline:
		m := n.(*passthrough.PassthroughInline)
		val := string(m.Segment.Value(s.source))
		switch {
		case strings.HasPrefix(val, "$") && strings.HasSuffix(val, "$"):
			latex = val[1 : len(val)-1]
		case strings.HasPrefix(val, `\(`) && strings.HasSuffix(val, `\)`):
			latex = val[2 : len(val)-2]
		default:
			latex = val
		}
		latex = strings.TrimSpace(latex)
		typeStr = "math-inline"
		displayMode = false
	case passthrough.KindPassthroughBlock:
		m := n.(*passthrough.PassthroughBlock)
		var lines strings.Builder
		l := m.Lines().Len()
		for i := 0; i < l; i++ {
			line := m.Lines().At(i)
			lines.Write(line.Value(s.source))
		}
		val := lines.String()
		valTrimmed := strings.TrimSpace(val)
		switch {
		case strings.HasPrefix(valTrimmed, "$$") && strings.HasSuffix(valTrimmed, "$$"):
			latex = valTrimmed[2 : len(valTrimmed)-2]
		case strings.HasPrefix(valTrimmed, `\[`) && strings.HasSuffix(valTrimmed, `\]`):
			latex = valTrimmed[2 : len(valTrimmed)-2]
		default:
			latex = valTrimmed
		}
		latex = strings.TrimSpace(latex)
		typeStr = "math-block"
		displayMode = true
	}

	if latex != "" {
		hash := native.HashContent(typeStr, latex)
		s.mathExpressions = append(s.mathExpressions, models.MathExpression{
			LaTeX:       latex,
			DisplayMode: displayMode,
			Hash:        hash,
		})

		placeholder := "<!--KOSH_MATH:" + hash + "-->"
		var newNode ast.Node
		if displayMode {
			newNode = &RawHTMLBlock{Content: []byte(placeholder)}
		} else {
			newNode = &RawHTMLInline{Content: []byte(placeholder)}
		}
		s.toReplace = append(s.toReplace, replacement{old: n, new: newNode})
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}
