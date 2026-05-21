package renderer

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	shortcodePkg "github.com/Kush-Singh-26/kosh/builder/shortcodes"
)

type SSRHTMLCollector struct {
	mu      sync.Mutex
	staged  []string
	tracked []string
}

func (c *SSRHTMLCollector) Add(html string) {
	if html == "" {
		return
	}
	c.mu.Lock()
	c.staged = append(c.staged, html)
	c.mu.Unlock()
}

func (c *SSRHTMLCollector) Snapshot() {
	c.mu.Lock()
	if len(c.staged) == 0 {
		c.mu.Unlock()
		return
	}
	c.tracked = append(c.tracked, c.staged...)
	c.staged = nil
	c.mu.Unlock()
}

func (c *SSRHTMLCollector) PopAll() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.tracked) == 0 {
		return nil
	}
	out := make([]string, len(c.tracked))
	copy(out, c.tracked)
	c.tracked = nil
	return out
}

type D2RenderFn func(ctx context.Context, code string) (lightSVG, darkSVG string, err error)
type RenderMathFn func(ctx context.Context, latex string, displayMode bool) (string, error)

func RenderDataWithShortcodes(
	input string,
	mdPool *sync.Pool,
	shortcodes *shortcodePkg.Processor,
	renderD2 D2RenderFn,
	renderMath RenderMathFn,
) (template.HTML, error) {
	if input == "" {
		return "", nil
	}

	rendered, err := processShortcodes(input, shortcodes)
	if err != nil {
		return "", err
	}

	if mdPool == nil {
		return template.HTML(rendered), nil
	}

	mdEngine, ok := mdPool.Get().(goldmark.Markdown)
	if !ok {
		return template.HTML(rendered), fmt.Errorf("unexpected mdPool type %T", mdEngine)
	}
	defer mdPool.Put(mdEngine)

	mdCtx := parser.NewContext()
	doc := mdEngine.Parser().Parse(text.NewReader([]byte(rendered)), parser.WithContext(mdCtx))

	var buf bytes.Buffer
	if err := mdEngine.Renderer().Render(&buf, []byte(rendered), doc); err != nil {
		return template.HTML(rendered), nil
	}
	html := buf.String()

	if renderD2 != nil {
		d2Exprs := mdParser.GetD2Expressions(mdCtx)
		if len(d2Exprs) > 0 {
			renderedD2 := make(map[string]models.SSRThemePair, len(d2Exprs))
			for _, expr := range d2Exprs {
				lightSVG, darkSVG, err := renderD2(context.Background(), expr.Code)
				if err != nil {
					continue
				}
				renderedD2[expr.Hash] = models.SSRThemePair{Light: lightSVG, Dark: darkSVG}
			}
			if len(renderedD2) > 0 {
				html = mdParser.LateReplaceD2(html, renderedD2)
			}
		}
	}

	if renderMath != nil {
		mathExprs := mdParser.GetMathExpressions(mdCtx)
		if len(mathExprs) > 0 {
			renderedMath := make(map[string]string, len(mathExprs))
			for _, expr := range mathExprs {
				out, err := renderMath(context.Background(), expr.LaTeX, expr.DisplayMode)
				if err != nil {
					continue
				}
				renderedMath[expr.Hash] = out
			}
			if len(renderedMath) > 0 {
				html = mdParser.LateReplaceMath(html, renderedMath)
			}
		}
	}

	return template.HTML(html), nil
}

func processShortcodes(input string, shortcodes *shortcodePkg.Processor) (string, error) {
	if shortcodes == nil || input == "" {
		return input, nil
	}
	processed, err := shortcodes.Process([]byte(input))
	if err != nil {
		return "", err
	}
	return string(processed), nil
}
