package parser

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/gohugoio/hugo-goldmark-extensions/passthrough"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"golang.org/x/sync/singleflight"
)

type unifiedTransformer struct {
	BaseURL  string
	Compress bool
	Renderer *native.Renderer
	Cache    *sync.Map
	D2Group  *singleflight.Group
}

type replacement struct {
	old ast.Node
	new ast.Node
}

type d2BlockInfo struct {
	node *ast.FencedCodeBlock
	code string
	hash string
}

func (t *unifiedTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	var toc []models.TOCEntry
	var plainText strings.Builder
	var d2Blocks []d2BlockInfo
	var mathExpressions []native.MathExpression
	var toReplace []replacement

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		kind := n.Kind()

		// 1. TOC & PlainText extraction (merged logic)
		switch kind {
		case ast.KindText:
			textNode := n.(*ast.Text)
			plainText.Write(textNode.Segment.Value(source))
			plainText.WriteString(" ")
		case ast.KindCodeBlock, ast.KindFencedCodeBlock:
			l := n.Lines().Len()
			for i := 0; i < l; i++ {
				line := n.Lines().At(i)
				plainText.Write(line.Value(source))
			}
			plainText.WriteString(" ")
		case ast.KindHeading:
			plainText.WriteString("\n")
			heading := n.(*ast.Heading)
			if heading.Level >= 2 && heading.Level <= 6 {
				var headerText strings.Builder
				_ = ast.Walk(heading, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
					if entering && child.Kind() == ast.KindText {
						textNode := child.(*ast.Text)
						headerText.Write(textNode.Segment.Value(source))
					}
					return ast.WalkContinue, nil
				})

				id, _ := heading.AttributeString("id")
				if id != nil {
					toc = append(toc, models.TOCEntry{
						ID:    string(id.([]byte)),
						Text:  headerText.String(),
						Level: heading.Level,
					})
				}
			}
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
			} else {
				t.processDestination(img, img.Destination, pc)
				t.processImageDestination(img, img.Destination)
				img.SetAttribute([]byte("loading"), []byte("lazy"))
			}
		}

		// 3. SSR logic (D2 diagrams)
		if kind == ast.KindFencedCodeBlock {
			fcb := n.(*ast.FencedCodeBlock)
			lang := strings.ToLower(strings.TrimSpace(string(fcb.Language(source))))
			if lang == "d2" {
				buf := utils.SharedBufferPool.Get()
				lines := fcb.Lines()
				for i := 0; i < lines.Len(); i++ {
					line := lines.At(i)
					buf.Write(line.Value(source))
				}
				code := strings.TrimSpace(buf.String())
				utils.SharedBufferPool.Put(buf)

				if code != "" {
					hash := native.HashContent("d2", code)
					d2Blocks = append(d2Blocks, d2BlockInfo{
						node: fcb,
						code: code,
						hash: hash,
					})
					AddSSRHash(pc, hash)
				}
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
				if strings.HasPrefix(val, "$$") && strings.HasSuffix(val, "$$") {
					latex = val[2 : len(val)-2]
				} else if strings.HasPrefix(val, `\[`) && strings.HasSuffix(val, `\]`) {
					latex = val[2 : len(val)-2]
				} else {
					latex = val
				}
				latex = strings.TrimSpace(latex)
				typeStr = "math-block"
				displayMode = true
			}

			if latex != "" {
				hash := native.HashContent(typeStr, latex)
				mathExpressions = append(mathExpressions, native.MathExpression{
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

func (t *unifiedTransformer) renderD2Blocks(d2Blocks []d2BlockInfo, pc parser.Context, toReplace *[]replacement) {
	results := make([]themePair, len(d2Blocks))
	var wg sync.WaitGroup
	ctx := GetContext(pc)

	for i := range d2Blocks {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b := d2Blocks[idx]
			pairVal, exists := t.Cache.Load(b.hash)
			if exists {
				results[idx] = pairVal.(themePair)
				return
			}
			if t.Renderer == nil {
				return
			}

			if t.D2Group != nil {
				v, err, _ := t.D2Group.Do(b.hash, func() (any, error) {
					if pairVal, exists := t.Cache.Load(b.hash); exists {
						return pairVal.(themePair), nil
					}
					lightSVG, err := t.Renderer.RenderD2(ctx, b.code, 0)
					if err != nil {
						if !errors.Is(err, context.Canceled) {
							slog.Warn("D2 light render failed", "error", err)
						}
						return themePair{}, err
					}
					darkSVG, err := t.Renderer.RenderD2(ctx, b.code, 200)
					if err != nil {
						return themePair{}, err
					}
					pair := themePair{light: lightSVG, dark: darkSVG}
					t.Cache.Store(b.hash, pair)
					return pair, nil
				})
				if err == nil {
					results[idx] = v.(themePair)
				}
			}
		}(i)
	}
	wg.Wait()

	for i, block := range d2Blocks {
		pair := results[i]
		if pair.light == "" && pair.dark == "" {
			continue
		}
		sb := utils.SharedStringBuilderPool.Get()
		sb.WriteString(`<div class="d2-container" data-diagram="true"><div class="d2-light">`)
		sb.WriteString(pair.light)
		sb.WriteString(`</div><div class="d2-dark">`)
		sb.WriteString(pair.dark)
		sb.WriteString(`</div><span class="zoom-hint">🔍 Click to zoom</span></div>`)
		rawNode := &RawHTMLBlock{Content: []byte(sb.String())}
		utils.SharedStringBuilderPool.Put(sb)
		*toReplace = append(*toReplace, replacement{old: block.node, new: rawNode})
	}
}

// Helpers from trans_url.go
func (t *unifiedTransformer) processImageDestination(img *ast.Image, dest []byte) {
	src := string(dest)
	if src == "" || strings.HasPrefix(src, "http") || strings.HasPrefix(src, "//") || strings.HasPrefix(src, "data:") {
		return
	}
	img.Destination = []byte(strings.ToLower(src))
}

func (t *unifiedTransformer) processDestination(n ast.Node, dest []byte, pc parser.Context) {
	href := string(dest)
	idx := strings.IndexAny(href, "?#")
	query := ""
	if idx != -1 {
		query = href[idx:]
		href = href[:idx]
	}

	if strings.HasPrefix(href, "http") {
		if _, isLink := n.(*ast.Link); isLink {
			n.SetAttribute([]byte("target"), []byte("_blank"))
			n.SetAttribute([]byte("rel"), []byte("noopener noreferrer"))
		}
	} else if t.Compress {
		ext := strings.ToLower(filepath.Ext(href))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
			href = href[:len(href)-len(ext)] + ".webp"
		}
	}

	if strings.HasSuffix(href, ".md") && !strings.HasPrefix(href, "http") {
		href = strings.Replace(href, ".md", ".html", 1)
		href = strings.ToLower(href)
	}
	href = strings.TrimPrefix(href, "./")

	if !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "http") {
		if filePath, ok := pc.Get(ContextKeyFilePath).(string); ok && filePath != "" {
			version := extractVersionFromPath(filePath)
			if version != "" {
				if !isCrossVersionLink(href) && !isRootLevelLink(href) {
					href = strings.TrimPrefix(href, "../")
					href = strings.ReplaceAll(href, "\\", "/")
				}
			}
		}
	}

	fullHref := href + query
	if !strings.HasPrefix(string(dest), "http") {
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = []byte(fullHref)
		case *ast.Image:
			node.Destination = []byte(fullHref)
		}
	}

	if strings.HasPrefix(href, "/") && t.BaseURL != "" {
		newDest := []byte(t.BaseURL + fullHref)
		switch node := n.(type) {
		case *ast.Link:
			node.Destination = newDest
		case *ast.Image:
			node.Destination = newDest
		}
	}
}
