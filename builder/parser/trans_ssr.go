package parser

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"golang.org/x/sync/singleflight"
)

// ssrTransformer handles server-side rendering of D2 diagrams and LaTeX math
type ssrTransformer struct {
	Renderer *native.Renderer
	Cache    *sync.Map           // Thread-safe cache for D2 diagrams
	D2Group  *singleflight.Group // Shared singleflight group to deduplicate D2 rendering across posts
}

// themePair stores both light and dark versions together for atomic access
type themePair struct {
	light string
	dark  string
}

// --- Custom AST node for raw HTML injection ---

// KindRawHTMLBlock is the NodeKind for RawHTMLBlock
var KindRawHTMLBlock = ast.NewNodeKind("RawHTMLBlock")

// RawHTMLBlock is a custom AST node that holds pre-rendered HTML content.
type RawHTMLBlock struct {
	ast.BaseBlock
	Content []byte
}

func (n *RawHTMLBlock) Kind() ast.NodeKind {
	return KindRawHTMLBlock
}

func (n *RawHTMLBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// rawHTMLBlockRenderer renders RawHTMLBlock nodes by writing their content directly.
type rawHTMLBlockRenderer struct{}

func (r *rawHTMLBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindRawHTMLBlock, r.renderRawHTMLBlock)
}

func (r *rawHTMLBlockRenderer) renderRawHTMLBlock(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*RawHTMLBlock)
		_, _ = w.Write(n.Content)
	}
	return ast.WalkContinue, nil
}

// --- d2 block info ---

type d2BlockInfo struct {
	node *ast.FencedCodeBlock
	code string
	hash string
}

func (t *ssrTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	// Pre-allocate slice with a reasonable estimate to reduce growth allocations
	d2Blocks := make([]d2BlockInfo, 0, 4)

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if n.Kind() == ast.KindFencedCodeBlock {
			fcb := n.(*ast.FencedCodeBlock)
			lang := strings.ToLower(strings.TrimSpace(string(fcb.Language(source))))

			if lang == "d2" {
				buf := utils.SharedBufferPool.Get()
				defer utils.SharedBufferPool.Put(buf)

				lines := fcb.Lines()
				for i := 0; i < lines.Len(); i++ {
					line := lines.At(i)
					buf.Write(line.Value(source))
				}
				code := strings.TrimSpace(buf.String())
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
		return ast.WalkContinue, nil
	})

	if len(d2Blocks) == 0 {
		return
	}

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
				pair := pairVal.(themePair)
				results[idx] = pair
				return
			}

			if t.Renderer == nil {
				return
			}

			// Use shared D2Group if provided, otherwise fall back to no singleflight (each render runs)
			if t.D2Group != nil {
				v, err, _ := t.D2Group.Do(b.hash, func() (interface{}, error) {
					pairVal, exists := t.Cache.Load(b.hash)
					if exists {
						return pairVal.(themePair), nil
					}

					lightSVG, renderErr := t.Renderer.RenderD2(ctx, b.code, 0)
					if renderErr != nil {
						if !errors.Is(renderErr, context.Canceled) {
							slog.Warn("D2 light theme render failed", "error", renderErr)
						}
						return themePair{}, renderErr
					}
					darkSVG, renderErr := t.Renderer.RenderD2(ctx, b.code, 200)
					if renderErr != nil {
						if !errors.Is(renderErr, context.Canceled) {
							slog.Warn("D2 dark theme render failed", "error", renderErr)
						}
						return themePair{}, renderErr
					}

					pair := themePair{light: lightSVG, dark: darkSVG}
					t.Cache.Store(b.hash, pair)
					return pair, nil
				})

				if err == nil {
					results[idx] = v.(themePair)
				}
			} else {
				// No shared group - render directly (fallback for backward compatibility)
				pairVal, exists := t.Cache.Load(b.hash)
				if exists {
					results[idx] = pairVal.(themePair)
				} else if t.Renderer != nil {
					lightSVG, renderErr := t.Renderer.RenderD2(ctx, b.code, 0)
					if renderErr == nil {
						darkSVG, renderErr := t.Renderer.RenderD2(ctx, b.code, 200)
						if renderErr == nil {
							pair := themePair{light: lightSVG, dark: darkSVG}
							t.Cache.Store(b.hash, pair)
							results[idx] = pair
						}
					}
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
		defer utils.SharedStringBuilderPool.Put(sb)

		sb.WriteString(`<div class="d2-container" data-diagram="true"><div class="d2-light">`)
		sb.WriteString(pair.light)
		sb.WriteString(`</div><div class="d2-dark">`)
		sb.WriteString(pair.dark)
		sb.WriteString(`</div><span class="zoom-hint">🔍 Click to zoom</span></div>`)

		rawNode := &RawHTMLBlock{Content: []byte(sb.String())}
		parent := block.node.Parent()
		if parent != nil {
			parent.ReplaceChild(parent, block.node, rawNode)
		}
	}
}

func GetContext(pc parser.Context) context.Context {
	if pc == nil {
		return context.Background()
	}
	if v := pc.Get(contextKeyBuild); v != nil {
		if ctx, ok := v.(context.Context); ok {
			return ctx
		}
	}
	return context.Background()
}

func WithContext(pc parser.Context, ctx context.Context) {
	if pc != nil {
		pc.Set(contextKeyBuild, ctx)
	}
}
