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
)

// Mermaid layout metrics. Font family and colors are themed at runtime through
// the --pele-* CSS variables, so only layout-affecting values live here.
const (
	mermaidFontSize = 18
	mermaidPadding  = 8
)

// RenderMermaid renders a single Mermaid diagram to a themeable SVG using Pele
// inside QuickJS. Pele emits one SVG whose colors come from --pele-* CSS
// variables, so no separate light/dark variants are needed.
func (r *Renderer) RenderMermaid(ctx context.Context, code string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := r.acquireTask(ctx, scheduler.TaskMermaid); err != nil {
		return "", err
	}
	defer r.releaseTask(scheduler.TaskMermaid)

	r.ensureInitialized()

	instance, err := r.acquireWorker()
	if err != nil {
		return "", err
	}
	defer r.releaseWorker(instance)

	if instance.ctx == nil || instance.peleRenderFn == nil {
		return "", errPeleNotInitialized
	}

	jsCode := instance.ctx.NewString(code)
	defer jsCode.Free()

	res, err := instance.ctx.Invoke(instance.peleRenderFn, instance.pele, jsCode, instance.peleOpts)
	if err != nil {
		return "", fmt.Errorf("mermaid render failed: %w", err)
	}
	defer res.Free()

	svg := res.GetPropertyStr("svg")
	if svg == nil || svg.IsNull() || svg.IsUndefined() {
		return "", errMermaidNoSVG
	}
	defer svg.Free()

	return svg.String(), nil
}

// RenderAllMermaid renders multiple Mermaid diagrams, reusing cached SVGs keyed
// by expression hash.
func (r *Renderer) RenderAllMermaid(ctx context.Context, expressions []models.MermaidExpression, cache map[string]string) (map[string]string, error) {
	if len(expressions) == 0 {
		return map[string]string{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	finalResults := make(map[string]string)
	var toRender []models.MermaidExpression
	seen := make(map[string]bool)
	for _, expr := range expressions {
		if seen[expr.Hash] {
			continue
		}
		seen[expr.Hash] = true
		if val, ok := cache[expr.Hash]; ok {
			finalResults[expr.Hash] = val
		} else {
			toRender = append(toRender, expr)
		}
	}

	if len(toRender) == 0 {
		return finalResults, nil
	}

	results, err := r.renderMermaidBatch(ctx, toRender)
	for hash, svg := range results {
		finalResults[hash] = svg
	}
	return finalResults, err
}

// renderMermaidBatch renders Mermaid diagrams across the worker pool.
func (r *Renderer) renderMermaidBatch(ctx context.Context, expressions []models.MermaidExpression) (map[string]string, error) {
	slog.Info("Mermaid batch render", "total", len(expressions), "workers", r.numWorkers)
	timer := timeutil.StartPhase("Mermaid render")
	defer timer.Stop()

	numWorkers := min(r.numWorkers, len(expressions))
	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var globalErr error

	taskChan := make(chan models.MermaidExpression, len(expressions))
	for _, expr := range expressions {
		taskChan <- expr
	}
	close(taskChan)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: "mermaid render",
			Fn: func() error {
				for expr := range taskChan {
					svg, err := r.RenderMermaid(ctx, expr.Code)
					if err != nil {
						mu.Lock()
						if globalErr == nil {
							globalErr = err
						}
						mu.Unlock()
						slog.Warn("Mermaid render failed", "hash", expr.Hash, "error", err)
						continue
					}
					mu.Lock()
					results[expr.Hash] = svg
					mu.Unlock()
				}
				return nil
			},
			Cleanup: wg.Done,
		})
	}

	wg.Wait()
	return results, globalErr
}
