package parser

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
)

const (
	d2LightTheme = 0
	d2DarkTheme  = 200
)

type d2BlockInfo struct {
	node *ast.FencedCodeBlock
	code string
	hash string
}

func dedupeD2Blocks(d2Blocks []d2BlockInfo) map[string]int {
	hashToFirstIndex := make(map[string]int)
	for i, block := range d2Blocks {
		if _, exists := hashToFirstIndex[block.hash]; !exists {
			hashToFirstIndex[block.hash] = i
		}
	}
	return hashToFirstIndex
}

func (t *unifiedTransformer) renderD2Block(ctx context.Context, b d2BlockInfo) (models.SSRThemePair, error) {
	key := "d2:" + b.hash
	if pairVal, exists := t.Cache.Load(key); exists {
		if pair, ok := pairVal.(models.SSRThemePair); ok {
			return pair, nil
		}
	}
	if t.Renderer == nil {
		return models.SSRThemePair{}, nil
	}

	if t.D2Group == nil {
		return models.SSRThemePair{}, nil
	}

	v, err, _ := t.D2Group.Do(b.hash, func() (any, error) {
		if pairVal, exists := t.Cache.Load(key); exists {
			if pair, ok := pairVal.(models.SSRThemePair); ok {
				return pair, nil
			}
		}
		lightSVG, err := t.Renderer.RenderD2(ctx, b.code, d2LightTheme)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Warn("D2 light render failed", "error", err)
			}
			return models.SSRThemePair{}, err
		}
		darkSVG, err := t.Renderer.RenderD2(ctx, b.code, d2DarkTheme)
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
	if err != nil {
		return models.SSRThemePair{}, err
	}
	if pair, ok := v.(models.SSRThemePair); ok {
		return pair, nil
	}
	return models.SSRThemePair{}, nil
}

func copyD2ResultsForDuplicates(d2Blocks []d2BlockInfo, hashToFirstIndex map[string]int, results []models.SSRThemePair) {
	for hash, firstIdx := range hashToFirstIndex {
		result := results[firstIdx]
		for i, block := range d2Blocks {
			if block.hash == hash && i != firstIdx {
				results[i] = result
			}
		}
	}
}

func appendD2Replacements(d2Blocks []d2BlockInfo, results []models.SSRThemePair, toReplace *[]replacement) {
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
		buf.WriteString(`</div><span class="zoom-hint">Click to zoom</span></div>`)

		content := make([]byte, buf.Len())
		copy(content, buf.Bytes())
		rawNode := &RawHTMLBlock{Content: content}
		pools.SharedBufferPool.Put(buf)
		*toReplace = append(*toReplace, replacement{old: block.node, new: rawNode})
	}
}

// ReplaceD2Expressions replaces D2 placeholders in HTML with rendered SVG output.
func ReplaceD2Expressions(htmlContent string, expressions []models.D2Expression, rendered map[string]models.SSRThemePair) string {
	if len(expressions) == 0 {
		return htmlContent
	}

	replacements := make([]string, 0, len(expressions)*2)
	for _, expr := range expressions {
		if pair, ok := rendered[expr.Hash]; ok {
			placeholder := "<!--KOSH_D2:" + expr.Hash + "-->"
			buf := pools.SharedBufferPool.Get()
			buf.WriteString(`<div class="d2-container" data-diagram="true"><div class="d2-light">`)
			buf.WriteString(pair.Light)
			buf.WriteString(`</div><div class="d2-dark">`)
			buf.WriteString(pair.Dark)
			buf.WriteString(`</div><span class="zoom-hint">Click to zoom</span></div>`)

			replacement := buf.String()
			pools.SharedBufferPool.Put(buf)
			replacements = append(replacements, placeholder, replacement)
		}
	}

	if len(replacements) == 0 {
		return htmlContent
	}

	return strings.NewReplacer(replacements...).Replace(htmlContent)
}

// HasD2Placeholders checks if the HTML content has D2 placeholders.
func HasD2Placeholders(html string) bool {
	return strings.Contains(html, "<!--KOSH_D2:")
}

func (t *unifiedTransformer) renderD2Blocks(d2Blocks []d2BlockInfo, pc parser.Context, toReplace *[]replacement) {
	results := make([]models.SSRThemePair, len(d2Blocks))
	var wg sync.WaitGroup
	ctx := GetContext(pc)

	// Optimized: Deduplicate hashes locally before launching goroutines
	// This avoids launching goroutines that immediately block on singleflight
	// Map from hash to first index where it appears
	hashToFirstIndex := dedupeD2Blocks(d2Blocks)

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
				pair, err := t.renderD2Block(ctx, b)
				if err == nil {
					results[idx] = pair
				}
				return nil
			},
			Cleanup: wg.Done,
		})
	}
	wg.Wait()

	// Copy results from first occurrence to all duplicates
	copyD2ResultsForDuplicates(d2Blocks, hashToFirstIndex, results)
	appendD2Replacements(d2Blocks, results, toReplace)
}
