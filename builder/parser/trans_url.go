package parser

import (
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

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
