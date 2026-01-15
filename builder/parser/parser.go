// Configures the markdown parser and URL transformation logic
package parser

import (
	"path/filepath"
	"strings"

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
)

// --- TOC Logic Start ---
var tocKey = parser.NewContextKey()

func GetTOC(pc parser.Context) []models.TOCEntry {
	if v := pc.Get(tocKey); v != nil {
		return v.([]models.TOCEntry)
	}
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

func New(baseURL string) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			meta.Meta,
			highlighting.NewHighlighting(highlighting.WithStyle("nord")),
			passthrough.New(passthrough.Config{
				InlineDelimiters: []passthrough.Delimiters{{Open: "$", Close: "$"}, {Open: "\\(", Close: "\\)"}},
				BlockDelimiters:  []passthrough.Delimiters{{Open: "$$", Close: "$$"}, {Open: "\\[", Close: "\\]"}},
			}),
		),
		goldmark.WithParserOptions(
			// Register TOCTransformer
			parser.WithASTTransformers(
				util.Prioritized(&URLTransformer{BaseURL: baseURL}, 100),
				util.Prioritized(&TOCTransformer{}, 200),
			),
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
}
