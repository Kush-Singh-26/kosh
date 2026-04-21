package parser

import (
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

// ContextKeyFilePath stores the current file path being parsed.
// Isolated via parser.NewContextKey() which guarantees global uniqueness.
var ContextKeyFilePath = parser.NewContextKey()

func (t *unifiedTransformer) processImageDestination(_ *ast.Image, dest []byte) {
	src := string(dest)
	if src == "" {
		return
	}
	if strings.HasPrefix(src, "http") || strings.HasPrefix(src, "//") || strings.HasPrefix(src, "data:") { //nolint:staticcheck
		return
	}
}

func (t *unifiedTransformer) processDestination(n ast.Node, dest []byte, _ parser.Context) {
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

func hasTextChild(n ast.Node, source []byte) bool {
	hasText := false
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node == n {
			return ast.WalkContinue, nil
		}
		if t, ok := node.(*ast.Text); ok {
			if len(strings.TrimSpace(string(t.Segment.Value(source)))) > 0 {
				hasText = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return hasText
}

func getAttrValue(n ast.Node, key string) string {
	for _, attr := range n.Attributes() {
		if string(attr.Name) == key {
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
