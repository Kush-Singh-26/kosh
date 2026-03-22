package parser

import (
	"log/slog"
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
	m sync.Map
}

func NewMemorySSRMap() *MemorySSRMap {
	return &MemorySSRMap{}
}

func (m *MemorySSRMap) Load(key string) (any, bool) {
	return m.m.Load(key)
}

func (m *MemorySSRMap) Store(key string, value any) {
	m.m.Store(key, value)
}

type unifiedTransformer struct {
	BaseURL  string
	Compress bool
	Renderer *native.Renderer
	Cache    SSRMap
	D2Group  *singleflight.Group
}

type replacement struct {
	old ast.Node
	new ast.Node
}

// transformContext holds state during the AST walk to avoid nested walks
type transformContext struct {
	inHeading    bool
	headingLevel int
	headingID    string
	headingText  strings.Builder
}

func (t *unifiedTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	var toc []models.TOCEntry
	var plainText strings.Builder
	var d2Blocks []d2BlockInfo
	var mathExpressions []models.MathExpression
	var toReplace []replacement

	// Context for single-pass extraction
	ctx := &transformContext{}

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		kind := n.Kind()

		// Handle heading entry/exit for single-pass text collection
		if kind == ast.KindHeading {
			heading := n.(*ast.Heading)
			if entering {
				if heading.Level >= 2 && heading.Level <= 6 {
					ctx.inHeading = true
					ctx.headingLevel = heading.Level
					ctx.headingText.Reset()
					id, _ := heading.AttributeString("id")
					if idBytes, ok := id.([]byte); ok {
						ctx.headingID = string(idBytes)
					} else {
						ctx.headingID = ""
					}
				}
			} else {
				if ctx.inHeading {
					// Heading exit - record TOC entry
					if ctx.headingID != "" {
						toc = append(toc, models.TOCEntry{
							ID:    ctx.headingID,
							Text:  ctx.headingText.String(),
							Level: ctx.headingLevel,
						})
					}
					ctx.inHeading = false
					ctx.headingLevel = 0
					ctx.headingID = ""
				}
			}
			return ast.WalkContinue, nil
		}

		// Collect text from nodes - either for plainText or heading text
		if entering {
			switch kind {
			case ast.KindText:
				textNode := n.(*ast.Text)
				segment := textNode.Segment.Value(source)
				plainText.Write(segment)
				if ctx.inHeading {
					ctx.headingText.Write(segment)
				}
				plainText.WriteString(" ")
			case ast.KindCodeBlock, ast.KindFencedCodeBlock:
				l := n.Lines().Len()
				for i := 0; i < l; i++ {
					line := n.Lines().At(i)
					plainText.Write(line.Value(source))
				}
				plainText.WriteString(" ")
			}
		}

		// Don't process children if exiting
		if !entering {
			return ast.WalkContinue, nil
		}

		// 2. URL transformation (logic from trans_url.go)
		if kind == ast.KindLink || kind == ast.KindImage {
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
				t.processDestination(ln, ln.Destination, pc)

				// A11y Lint: check for empty link text (no child text and no aria-label)
				ariaLabel := getAttrValue(n, "aria-label")
				hasText := hasTextChild(ln, source)
				if strings.TrimSpace(ariaLabel) == "" && !hasText {
					filePath, _ := pc.Get(ContextKeyFilePath).(string)
					slog.Warn("A11y Lint: Link has no text or aria-label",
						"file", filePath,
						"href", string(ln.Destination))
				}
			} else {
				t.processDestination(img, img.Destination, pc)
				t.processImageDestination(img, img.Destination)
				img.SetAttribute([]byte("loading"), []byte("lazy"))
			}
		}

		// 3. D2 logic
		if kind == ast.KindFencedCodeBlock {
			cb := n.(*ast.FencedCodeBlock)
			lang := string(cb.Language(source))
			if lang == "d2" {
				var lines strings.Builder
				l := cb.Lines().Len()
				for i := 0; i < l; i++ {
					line := cb.Lines().At(i)
					lines.Write(line.Value(source))
				}
				code := lines.String()
				hash := native.HashContent("d2", code)

				d2Blocks = append(d2Blocks, d2BlockInfo{
					node: cb,
					code: code,
					hash: hash,
				})
				AddSSRHash(pc, hash)
			}
		}

		// 4. Math logic (LaTeX)
		if kind == passthrough.KindPassthroughInline || kind == passthrough.KindPassthroughBlock {
			var latex string
			var typeStr string
			var displayMode bool

			if kind == passthrough.KindPassthroughInline {
				m := n.(*passthrough.PassthroughInline)
				val := string(m.Segment.Value(source))
				if strings.HasPrefix(val, "$") && strings.HasSuffix(val, "$") {
					latex = val[1 : len(val)-1]
				} else if strings.HasPrefix(val, `\(`) && strings.HasSuffix(val, `\)`) {
					latex = val[2 : len(val)-2]
				} else {
					latex = val
				}
				latex = strings.TrimSpace(latex)
				typeStr = "math-inline"
				displayMode = false
			} else {
				m := n.(*passthrough.PassthroughBlock)
				var lines strings.Builder
				l := m.Lines().Len()
				for i := 0; i < l; i++ {
					line := m.Lines().At(i)
					lines.Write(line.Value(source))
				}
				val := lines.String()
				valTrimmed := strings.TrimSpace(val)
				if strings.HasPrefix(valTrimmed, "$$") && strings.HasSuffix(valTrimmed, "$$") {
					latex = valTrimmed[2 : len(valTrimmed)-2]
				} else if strings.HasPrefix(valTrimmed, `\[`) && strings.HasSuffix(valTrimmed, `\]`) {
					latex = valTrimmed[2 : len(valTrimmed)-2]
				} else {
					latex = valTrimmed
				}
				latex = strings.TrimSpace(latex)
				typeStr = "math-block"
				displayMode = true
			}

			if latex != "" {
				hash := native.HashContent(typeStr, latex)
				mathExpressions = append(mathExpressions, models.MathExpression{
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
				toReplace = append(toReplace, replacement{old: n, new: newNode})
				return ast.WalkSkipChildren, nil
			}
		}

		return ast.WalkContinue, nil
	})

	// Post-walk: Perform D2 rendering and node replacements
	if len(d2Blocks) > 0 {
		t.renderD2Blocks(d2Blocks, pc, &toReplace)
	}

	// Apply all replacements
	for _, r := range toReplace {
		parent := r.old.Parent()
		if parent != nil {
			parent.ReplaceChild(parent, r.old, r.new)
		}
	}

	// Set results in context
	pc.Set(tocKey, toc)
	pc.Set(plainTextKey, plainText.String())
	if len(mathExpressions) > 0 {
		pc.Set(mathExpressionsKey, mathExpressions)
	}
}
