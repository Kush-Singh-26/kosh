package parser

import (
	"context"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// --- Custom AST node for raw HTML injection ---

// KindRawHTMLBlock is the NodeKind for RawHTMLBlock
var KindRawHTMLBlock = ast.NewNodeKind("RawHTMLBlock")

// KindRawHTMLInline is the NodeKind for RawHTMLInline
var KindRawHTMLInline = ast.NewNodeKind("RawHTMLInline")

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

// RawHTMLInline is a custom AST node that holds pre-rendered HTML content for inline elements.
type RawHTMLInline struct {
	ast.BaseInline
	Content []byte
}

func (n *RawHTMLInline) Kind() ast.NodeKind {
	return KindRawHTMLInline
}

func (n *RawHTMLInline) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// rawHTMLBlockRenderer renders RawHTMLBlock and RawHTMLInline nodes by writing their content directly.
type rawHTMLBlockRenderer struct{}

func (r *rawHTMLBlockRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindRawHTMLBlock, r.renderRawHTML)
	reg.Register(KindRawHTMLInline, r.renderRawHTML)
}

func (r *rawHTMLBlockRenderer) renderRawHTML(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		var content []byte
		if node.Kind() == KindRawHTMLBlock {
			content = node.(*RawHTMLBlock).Content
		} else {
			content = node.(*RawHTMLInline).Content
		}
		_, _ = w.Write(content)
	}
	return ast.WalkContinue, nil
}

// --- d2 block info ---

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
