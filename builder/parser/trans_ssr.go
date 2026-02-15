package parser

import (
	"bytes"
	"log"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

// ssrTransformer handles server-side rendering of D2 diagrams and LaTeX math
type ssrTransformer struct {
	Renderer *native.Renderer
	Cache    *sync.Map // Thread-safe cache for D2 diagrams
}

func (t *ssrTransformer) Transform(node *ast.Document, reader text.Reader, pc parser.Context) {
	source := reader.Source()
	var d2Blocks []struct {
		code string
		hash string
	}

	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		if n.Kind() == ast.KindFencedCodeBlock {
			fcb := n.(*ast.FencedCodeBlock)
			lang := strings.ToLower(strings.TrimSpace(string(fcb.Language(source))))

			if lang == "d2" {
				var codeBuilder bytes.Buffer
				lines := fcb.Lines()
				for i := 0; i < lines.Len(); i++ {
					line := lines.At(i)
					codeBuilder.Write(line.Value(source))
				}
				code := strings.TrimSpace(codeBuilder.String())
				if code != "" {
					hash := native.HashContent("d2", code)
					d2Blocks = append(d2Blocks, struct {
						code string
						hash string
					}{code: code, hash: hash})
					AddSSRHash(pc, hash)
				}
			}
		}
		return ast.WalkContinue, nil
	})

	if len(d2Blocks) == 0 {
		return
	}

	// 2. Render all blocks in parallel (or use cache)
	results := make([]D2SVGPair, len(d2Blocks))
	var wg sync.WaitGroup

	for i, block := range d2Blocks {
		wg.Add(1)
		go func(idx int, b struct {
			code string
			hash string
		}) {
			defer wg.Done()

			lightHash := b.hash + "_light"
			darkHash := b.hash + "_dark"

			// Check cache (sync.Map is thread-safe, no mutex needed)
			lightCached, lightExists := t.Cache.Load(lightHash)
			darkCached, darkExists := t.Cache.Load(darkHash)

			if lightExists && darkExists {
				results[idx] = D2SVGPair{Light: lightCached.(string), Dark: darkCached.(string)}
				return
			}

			// Render
			lightSVG, err := t.Renderer.RenderD2(b.code, 0)
			if err != nil {
				log.Printf("   ⚠️  D2 light theme render failed: %v", err)
				return
			}
			darkSVG, err := t.Renderer.RenderD2(b.code, 200)
			if err != nil {
				log.Printf("   ⚠️  D2 dark theme render failed: %v", err)
				return
			}

			pair := D2SVGPair{Light: lightSVG, Dark: darkSVG}
			results[idx] = pair

			// Store in cache (sync.Map is thread-safe)
			t.Cache.Store(lightHash, lightSVG)
			t.Cache.Store(darkHash, darkSVG)
		}(i, block)
	}
	wg.Wait()

	// 3. Store in context
	pc.Set(d2OrderedKey, results)
}
