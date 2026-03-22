package parser

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	goldmarkRenderer "github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

func TestThemePair_AtomicLoadStore(t *testing.T) {
	cache := NewMemorySSRMap()

	pair := models.SSRThemePair{Light: "light-svg", Dark: "dark-svg"}
	hash := "test-hash"

	cache.Store(hash, pair)

	loaded, exists := cache.Load(hash)
	if !exists {
		t.Error("Cache should contain the pair")
	}

	storedPair := loaded.(models.SSRThemePair)
	if storedPair.Light != "light-svg" || storedPair.Dark != "dark-svg" {
		t.Errorf("Loaded pair mismatch: got %+v", storedPair)
	}

	t.Log("Theme pair atomic load/store test passed")
}

func TestThemePair_ConcurrentAccess(t *testing.T) {
	cache := NewMemorySSRMap()
	var wg sync.WaitGroup
	numGoroutines := 20
	iterations := 100

	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range iterations {
				hash := "hash-" + string(rune('0'+id%10))
				pair := models.SSRThemePair{Light: "light-" + string(rune('0'+id)), Dark: "dark-" + string(rune('0'+id))}
				cache.Store(hash, pair)

				loaded, exists := cache.Load(hash)
				if exists {
					_ = loaded.(models.SSRThemePair)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Log("Theme pair concurrent access test passed")
}

func TestD2ASTReplacement(t *testing.T) {
	mockCache := NewMemorySSRMap()
	mockCache.Store("d2:"+native.HashContent("d2", "x -> y\n"), models.SSRThemePair{Light: "svg-light", Dark: "svg-dark"})

	md := goldmark.New(
		goldmark.WithParserOptions(
			parser.WithASTTransformers(
				util.Prioritized(&unifiedTransformer{
					Renderer: nil,
					Cache:    mockCache,
				}, 50),
			),
		),
		goldmark.WithRendererOptions(
			goldmarkRenderer.WithNodeRenderers(
				util.Prioritized(&rawHTMLBlockRenderer{}, 500),
			),
		),
	)

	source := []byte("```d2\nx -> y\n```")

	ctx := parser.NewContext()
	ctx.Set(ContextKeyFilePath, "test.md")

	doc := md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))

	// Verify the AST transformation
	d2Found := false
	rawFound := false

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.FencedCodeBlock:
			if string(node.Language(source)) == "d2" {
				d2Found = true
			}
		case *RawHTMLBlock:
			rawFound = true
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		t.Fatalf("Failed to walk AST: %v", err)
	}

	if d2Found {
		t.Error("Original D2 FencedCodeBlock should have been replaced, but was still found in AST")
	}

	if !rawFound {
		t.Error("RawHTMLBlock was not found in AST after transformation")
	}

	// Also verify HTML render output directly
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if !strings.Contains(buf.String(), "d2-container") {
		t.Errorf("Rendered HTML missing d2-container, got: %s", buf.String())
	}
}
