package parser

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

const headingAnchorRendererPriority = 200

type headingAnchorRenderer struct {
	html.Config
}

func newHeadingAnchorRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &headingAnchorRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *headingAnchorRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.render)
}

func (r *headingAnchorRenderer) render(writer util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if entering {
		_, _ = writer.WriteString("<h")
		_ = writer.WriteByte("0123456"[n.Level])
		if n.Attributes() != nil {
			html.RenderAttributes(writer, node, html.HeadingAttributeFilter)
		}
		_ = writer.WriteByte('>')
		if id := getHeadingID(n); id != "" {
			_, _ = writer.WriteString(`<a href="#` + id + `" class="heading-anchor" aria-label="Link to this section">#</a>`)
		}
	} else {
		_, _ = writer.WriteString("</h")
		_ = writer.WriteByte("0123456"[n.Level])
		_, _ = writer.WriteString(">\n")
	}
	return ast.WalkContinue, nil
}

func getHeadingID(n *ast.Heading) string {
	if n.Attributes() == nil {
		return ""
	}
	for _, attr := range n.Attributes() {
		if string(attr.Name) == "id" {
			switch v := attr.Value.(type) {
			case []byte:
				return string(v)
			case string:
				return v
			}
		}
	}
	return ""
}
