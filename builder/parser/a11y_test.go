package parser

import (
	"sync"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func TestA11yLinting(t *testing.T) {
	transformer := &unifiedTransformer{
		A11yMap: sync.Map{},
		IsDev:   false, // Linting is only active in production mode
	}

	markdown := `
![Image with alt](img1.png)
![](img2.png)
[Link with text](https://example.com)
[](https://example.com/no-text)
`
	source := []byte(markdown)
	gmParser := goldmark.DefaultParser()
	doc := gmParser.Parse(text.NewReader(source)).(*ast.Document)
	pc := parser.NewContext()
	pc.Set(ContextKeyFilePath, "test.md")

	state := &transformState{
		source:      source,
		transformer: transformer,
		pc:          pc,
	}

	// Walk and check
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n.Kind() {
		case ast.KindLink:
			state.checkLinkA11y(n.(*ast.Link))
		case ast.KindImage:
			state.checkImageA11y(n.(*ast.Image))
		}
		return ast.WalkContinue, nil
	})

	// Verify A11yMap
	// img1.png has alt (it has a text child)
	// img2.png has no alt
	// https://example.com has text
	// https://example.com/no-text has no text

	missingAltKey := "test.md:img:img2.png"
	if _, ok := transformer.A11yMap.Load(missingAltKey); !ok {
		t.Errorf("Expected %s to be in A11yMap", missingAltKey)
	}

	hasAltKey := "test.md:img:img1.png"
	if _, ok := transformer.A11yMap.Load(hasAltKey); ok {
		t.Errorf("Expected %s NOT to be in A11yMap", hasAltKey)
	}

	missingLinkTextKey := "test.md:https://example.com/no-text"
	if _, ok := transformer.A11yMap.Load(missingLinkTextKey); !ok {
		t.Errorf("Expected %s to be in A11yMap", missingLinkTextKey)
	}

	hasLinkTextKey := "test.md:https://example.com"
	if _, ok := transformer.A11yMap.Load(hasLinkTextKey); ok {
		t.Errorf("Expected %s NOT to be in A11yMap", hasLinkTextKey)
	}
}

func TestA11yLinting_DevMode(t *testing.T) {
	transformer := &unifiedTransformer{
		A11yMap: sync.Map{},
		IsDev:   true, // Linting should be disabled in dev mode
	}

	markdown := `![](img.png)`
	source := []byte(markdown)
	gmParser := goldmark.DefaultParser()
	doc := gmParser.Parse(text.NewReader(source)).(*ast.Document)
	pc := parser.NewContext()
	pc.Set(ContextKeyFilePath, "test.md")

	state := &transformState{
		source:      source,
		transformer: transformer,
		pc:          pc,
	}

	img := doc.FirstChild().FirstChild().(*ast.Image)
	state.checkImageA11y(img)

	missingAltKey := "test.md:img:img.png"
	if _, ok := transformer.A11yMap.Load(missingAltKey); ok {
		t.Errorf("Expected %s NOT to be in A11yMap in dev mode", missingAltKey)
	}
}
