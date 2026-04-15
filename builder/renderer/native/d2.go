package native

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

const (
	d2LightTheme = 0
	d2DarkTheme  = 200
)

// RenderGlobalD2Batch renders all unique D2 diagrams across the entire site
// in parallel using the worker pool.
func (r *Renderer) RenderGlobalD2Batch(ctx context.Context, expressions []models.D2Expression) (map[string]models.SSRThemePair, error) {
	if len(expressions) == 0 {
		return make(map[string]models.SSRThemePair), nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	r.ensureInitialized()

	// 1. Deduplicate globally
	uniqueExprs := make([]models.D2Expression, 0, len(expressions))
	seen := make(map[string]bool)
	for _, e := range expressions {
		if !seen[e.Hash] {
			seen[e.Hash] = true
			uniqueExprs = append(uniqueExprs, e)
		}
	}

	if len(uniqueExprs) == 0 {
		return make(map[string]models.SSRThemePair), nil
	}

	slog.Info("Global D2 batch render", "total", len(uniqueExprs), "workers", r.numWorkers)
	timer := timeutil.StartPhase("Global D2 render")
	defer timer.Stop()

	return r.RenderAllD2(ctx, uniqueExprs, nil)
}

func (r *Renderer) spawnD2Worker(ctx context.Context, taskChan <-chan models.D2Expression, results map[string]models.SSRThemePair, mu *sync.Mutex, wg *sync.WaitGroup, globalErr *error) {
	wg.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    slog.Default(),
		Operation: "d2 render",
		Fn: func() error {
			for expr := range taskChan {
				lightSVG, err := r.RenderD2(ctx, expr.Code, d2LightTheme)
				if err != nil {
					mu.Lock()
					if *globalErr == nil {
						*globalErr = err
					}
					mu.Unlock()
					return nil
				}

				darkSVG, err := r.RenderD2(ctx, expr.Code, d2DarkTheme)
				if err != nil {
					mu.Lock()
					if *globalErr == nil {
						*globalErr = err
					}
					mu.Unlock()
					return nil
				}

				mu.Lock()
				results[expr.Hash] = models.SSRThemePair{Light: lightSVG, Dark: darkSVG}
				mu.Unlock()
			}
			return nil
		},
		Cleanup: wg.Done,
	})
}

// RenderAllD2 renders multiple D2 diagrams in parallel.
func (r *Renderer) RenderAllD2(ctx context.Context, expressions []models.D2Expression, cache map[string]models.SSRThemePair) (map[string]models.SSRThemePair, error) {
	if len(expressions) == 0 {
		return make(map[string]models.SSRThemePair), nil
	}

	finalResults := make(map[string]models.SSRThemePair)
	var toRender []models.D2Expression
	for _, expr := range expressions {
		if val, ok := cache[expr.Hash]; ok {
			finalResults[expr.Hash] = val
		} else {
			toRender = append(toRender, expr)
		}
	}

	if len(toRender) == 0 {
		return finalResults, nil
	}

	numWorkers := min(r.numWorkers, len(toRender))
	results := make(map[string]models.SSRThemePair)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var globalErr error

	taskChan := make(chan models.D2Expression, len(toRender))
	for _, expr := range toRender {
		taskChan <- expr
	}
	close(taskChan)

	for i := 0; i < numWorkers; i++ {
		r.spawnD2Worker(ctx, taskChan, results, &mu, &wg, &globalErr)
	}

	wg.Wait()
	if globalErr != nil {
		return finalResults, globalErr
	}

	for hash, pair := range results {
		finalResults[hash] = pair
	}
	return finalResults, nil
}

// RenderD2 renders a D2 diagram to SVG with the specified theme ID.
func (r *Renderer) RenderD2(ctx context.Context, code string, themeID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if r.scheduler != nil {
		if err := r.scheduler.Acquire(ctx, scheduler.TaskD2); err != nil {
			return "", err
		}
		defer r.scheduler.Release(scheduler.TaskD2)
	}

	r.ensureInitialized()

	r.mu.Lock()
	if r.isClosed {
		r.mu.Unlock()
		return "", errRendererClosed
	}
	r.taskWg.Add(1)
	r.mu.Unlock()

	defer r.taskWg.Done()

	ruler := r.rulerPool.Get().(*textmeasure.Ruler)
	if ruler == nil {
		// Fallback or error if ruler is nil
		return "", errRulerPoolUnavailable
	}
	defer r.rulerPool.Put(ruler)

	// Configure layout
	layout := func(ctx context.Context, g *d2graph.Graph) error {
		return d2dagrelayout.Layout(ctx, g, nil)
	}

	compileOpts := &d2lib.CompileOptions{
		Layout: nil,
		Ruler:  ruler,
	}

	compileOpts.LayoutResolver = func(_ string) (d2graph.LayoutGraph, error) {
		return layout, nil
	}

	renderOpts := &d2svg.RenderOpts{
		ThemeID: &themeID,
		Pad:     go2.Pointer(int64(0)),
	}

	// Use provided context instead of Background
	// Wrap with D2 default logger to silence warnings
	ctx = d2log.WithDefault(ctx)
	diagram, _, err := d2lib.Compile(ctx, code, compileOpts, renderOpts)
	if err != nil {
		return "", fmt.Errorf("d2 compile failed: %w", err)
	}

	out, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return "", fmt.Errorf("d2 render failed: %w", err)
	}

	return string(out), nil
}
