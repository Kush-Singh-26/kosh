package parser

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

type d2BlockInfo struct {
	node *ast.FencedCodeBlock
	code string
	hash string
}

func (t *unifiedTransformer) renderD2Blocks(d2Blocks []d2BlockInfo, pc parser.Context, toReplace *[]replacement) {
	results := make([]models.SSRThemePair, len(d2Blocks))
	var wg sync.WaitGroup
	ctx := GetContext(pc)

	// Optimized: Deduplicate hashes locally before launching goroutines
	// This avoids launching goroutines that immediately block on singleflight
	// Map from hash to first index where it appears
	hashToFirstIndex := make(map[string]int)
	for i, block := range d2Blocks {
		if _, exists := hashToFirstIndex[block.hash]; !exists {
			hashToFirstIndex[block.hash] = i
		}
	}

	// Only launch goroutines for unique hashes
	for _, firstIdx := range hashToFirstIndex {
		idx := firstIdx
		wg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: "d2 render block",
			Fn: func() error {
				b := d2Blocks[idx]
				key := "d2:" + b.hash
				pairVal, exists := t.Cache.Load(key)
				if exists {
					if pair, ok := pairVal.(models.SSRThemePair); ok {
						results[idx] = pair
						return nil
					}
				}
				if t.Renderer == nil {
					return nil
				}

				if t.D2Group != nil {
					v, err, _ := t.D2Group.Do(b.hash, func() (any, error) {
						if pairVal, exists := t.Cache.Load(key); exists {
							if pair, ok := pairVal.(models.SSRThemePair); ok {
								return pair, nil
							}
						}
						lightSVG, err := t.Renderer.RenderD2(ctx, b.code, 0)
						if err != nil {
							if !errors.Is(err, context.Canceled) {
								slog.Warn("D2 light render failed", "error", err)
							}
							return models.SSRThemePair{}, err
						}
						darkSVG, err := t.Renderer.RenderD2(ctx, b.code, 200)
						if err != nil {
							if !errors.Is(err, context.Canceled) {
								slog.Warn("D2 dark render failed", "error", err)
							}
							return models.SSRThemePair{}, err
						}
						pair := models.SSRThemePair{Light: lightSVG, Dark: darkSVG}
						t.Cache.Store(key, pair)
						return pair, nil
					})
					if err == nil {
						if pair, ok := v.(models.SSRThemePair); ok {
							results[idx] = pair
						}
					}
				}
				return nil
			},
			Cleanup: wg.Done,
		})
	}
	wg.Wait()

	// Copy results from first occurrence to all duplicates
	for hash, firstIdx := range hashToFirstIndex {
		result := results[firstIdx]
		for i, block := range d2Blocks {
			if block.hash == hash && i != firstIdx {
				results[i] = result
			}
		}
	}

	for i, block := range d2Blocks {
		pair := results[i]
		if pair.Light == "" && pair.Dark == "" {
			continue
		}
		buf := pools.SharedBufferPool.Get()
		buf.WriteString(`<div class="d2-container" data-diagram="true"><div class="d2-light">`)
		buf.WriteString(pair.Light)
		buf.WriteString(`</div><div class="d2-dark">`)
		buf.WriteString(pair.Dark)
		buf.WriteString(`</div><span class="zoom-hint">🔍 Click to zoom</span></div>`)

		content := make([]byte, buf.Len())
		copy(content, buf.Bytes())
		rawNode := &RawHTMLBlock{Content: content}
		pools.SharedBufferPool.Put(buf)
		*toReplace = append(*toReplace, replacement{old: block.node, new: rawNode})
	}
}
