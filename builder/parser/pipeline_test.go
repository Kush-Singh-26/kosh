package parser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/yuin/goldmark/parser"
	"golang.org/x/sync/singleflight"
)

type pipelineTestCase struct {
	name     string
	markdown string
	contains []string
	filePath string
}

func TestFullPipeline(t *testing.T) {
	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			BaseURL: "https://example.com",
		},
	}
	// Use real native renderer if possible, or nil if we skip SSR parts
	r := native.New()
	defer func() { _ = r.Close() }()

	diagramCache := NewMemorySSRMap()
	d2Group := &singleflight.Group{}

	tests := []pipelineTestCase{
		{
			name:     "GFM Task List",
			markdown: "- [x] Task 1\n- [ ] Task 2",
			contains: []string{"<input checked=\"\" disabled=\"\" type=\"checkbox\">", "Task 1"},
		},
		{
			name:     "Code Highlighting",
			markdown: "```go\nfunc main() {}\n```",
			contains: []string{"class=\"code-block-container\"", "data-lang=\"go\"", "func"},
		},
		{
			name:     "Admonitions",
			markdown: "!!! note\n    This is a note\n!!!",
			contains: []string{"class=\"admonition adm-note\"", "adm-title"},
		},
		{
			name:     "Math (Passthrough/SSR Markers)",
			markdown: "Inline $E=mc^2$ and block:\n\n$$\na^2 + b^2 = c^2\n$$",
			contains: []string{"<!--KOSH_MATH:", "-->"},
		},
		{
			name:     "D2 Diagram (SSR Marker)",
			markdown: "```d2\ndir: right\nA -> B\n```",
			contains: []string{"<!--KOSH_D2:", "-->"},
		},
		{
			name:     "URL Transformation (.md to .html)",
			markdown: "[Link](test.md)",
			contains: []string{"href=\"test.html\""},
		},
		{
			name:     "Image Transformation (to .webp)",
			markdown: "![Img](test.png)",
			contains: []string{"src=\"test.webp\"", "loading=\"lazy\""},
		},
		{
			name:     "BaseURL Transformation",
			markdown: "[Link](/root-path.html)",
			contains: []string{"href=\"https://example.com/root-path.html\""},
		},
		{
			name:     "Relative Link Transformation",
			markdown: "[Link](../other-post.md)",
			contains: []string{"href=\"../other-post.html\""},
			filePath: "content/posts/subdir/test.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Enable image compression for the image test
			if tt.name == "Image Transformation (to .webp)" {
				cfg.ShouldCompressImages = true
			} else {
				cfg.ShouldCompressImages = false
			}

			// Set BaseURL for the BaseURL test
			if tt.name == "BaseURL Transformation" {
				cfg.BaseURL = "https://example.com"
			} else {
				cfg.BaseURL = ""
			}

			p := New(cfg, WithRenderer(r), WithDiagramCache(diagramCache), WithD2Group(d2Group))

			pc := parser.NewContext()
			if tt.filePath != "" {
				pc.Set(ContextKeyFilePath, tt.filePath)
			}

			var buf bytes.Buffer
			if err := p.Convert([]byte(tt.markdown), &buf, parser.WithContext(pc)); err != nil {
				t.Fatalf("Convert failed: %v", err)
			}
			html := buf.String()
			for _, c := range tt.contains {
				if !strings.Contains(html, c) {
					t.Errorf("Expected HTML to contain %q, but it didn't.\nHTML: %s", c, html)
				}
			}
		})
	}
}

func TestTOCAndPlainText(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	markdown := []byte(`# Title 1
## Section 1
Content here.
### Subsection 1.1`)

	// Goldmark doesn't return TOC directly from Convert, it stores it in Context
	pc := parser.NewContext()
	var buf bytes.Buffer
	if err := p.Convert(markdown, &buf, parser.WithContext(pc)); err != nil {
		t.Fatal(err)
	}

	toc, ok := pc.Get(tocKey).([]models.TOCEntry)
	if !ok {
		t.Fatal("TOC not found in context")
	}

	if len(toc) != 2 { // Section 1 and Subsection 1.1 (Level 2 and 3)
		t.Errorf("Expected 2 TOC entries, got %d", len(toc))
	}

	plainText, ok := pc.Get(plainTextKey).(string)
	if !ok {
		t.Fatal("PlainText not found in context")
	}

	if !strings.Contains(plainText, "Content") || !strings.Contains(plainText, "here") {
		t.Errorf("PlainText missing body content. Got: %q", plainText)
	}
}

func TestMathHelpers(t *testing.T) {
	pc := parser.NewContext()
	exprs := []models.MathExpression{
		{LaTeX: "E=mc^2", Hash: "hash1", DisplayMode: false},
	}
	pc.Set(mathExpressionsKey, exprs)

	got := GetMathExpressions(pc)
	if len(got) != 1 || got[0].LaTeX != "E=mc^2" {
		t.Errorf("GetMathExpressions failed: got %+v", got)
	}

	html := "Body <!--KOSH_MATH:hash1-->"
	rendered := map[string]string{"hash1": "RENDERED"}
	replaced := ReplaceMathExpressions(html, exprs, rendered)
	if !strings.Contains(replaced, "katex-inline") || !strings.Contains(replaced, "RENDERED") {
		t.Errorf("ReplaceMathExpressions failed: %s", replaced)
	}
}

func TestRenderMathForHTML(t *testing.T) {
	ctx := context.Background()
	r := native.New()
	defer func() { _ = r.Close() }()

	html := "Body <!--KOSH_MATH:c8c77e1c84e0aa52-->"
	exprs := []models.MathExpression{
		{LaTeX: "E=mc^2", Hash: "c8c77e1c84e0aa52", DisplayMode: false},
	}

	// Mock cache
	cache := map[string]string{}
	lookup := func(h string) (string, bool) {
		v, ok := cache[h]
		return v, ok
	}

	out, hashes, newEntries := RenderMathForHTML(RenderMathOptions{
		Ctx:          ctx,
		HTML:         html,
		Renderer:     r,
		CacheLookup:  lookup,
		PreCollected: exprs,
	})
	if len(hashes) != 1 || len(newEntries) != 1 {
		t.Errorf("RenderMathForHTML failed: hashes=%v, newEntries=%v", hashes, newEntries)
	}
	if !strings.Contains(out, "katex-inline") {
		t.Errorf("RenderMathForHTML output missing rendered content: %s", out)
	}
}
