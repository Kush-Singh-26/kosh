package native

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/fastschema/qjs"
)

// RenderGlobalBatch renders all unique math expressions across the entire site
// in large parallel chunks to minimize Go-to-JS bridge overhead.
// Each worker processes a batch of expressions in a single JS call, reducing
// the number of context switches between Go and QuickJS runtime.
func (r *Renderer) RenderGlobalBatch(ctx context.Context, expressions []models.MathExpression) (map[string]string, error) {
	if len(expressions) == 0 {
		return make(map[string]string), nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	r.ensureInitialized()

	// 1. Deduplicate globally
	uniqueExprs := make([]models.MathExpression, 0, len(expressions))
	seen := make(map[string]bool)
	for _, e := range expressions {
		if !seen[e.Hash] {
			seen[e.Hash] = true
			uniqueExprs = append(uniqueExprs, e)
		}
	}

	if len(uniqueExprs) == 0 {
		return make(map[string]string), nil
	}

	slog.Info("Global math batch render", "total", len(uniqueExprs), "workers", r.numWorkers)
	timer := utils.StartPhase("Global math render")
	defer timer.Stop()

	// 2. Chunk expressions by number of workers
	numWorkers := min(r.numWorkers, len(uniqueExprs))

	chunkSize := (len(uniqueExprs) + numWorkers - 1) / numWorkers

	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var globalErr error

	for i := 0; i < numWorkers; i++ {
		start := i * chunkSize
		if start >= len(uniqueExprs) {
			break
		}
		end := min(start+chunkSize, len(uniqueExprs))

		wg.Add(1)
		go func(chunk []models.MathExpression) {
			defer wg.Done()

			rendered, err := r.RenderMathBatch(ctx, chunk)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if globalErr == nil {
					globalErr = err
				}
				return
			}

			for j, html := range rendered {
				results[chunk[j].Hash] = html
			}
		}(uniqueExprs[start:end])
	}

	wg.Wait()
	return results, globalErr
}

// RenderMath renders a single LaTeX expression to HTML using KaTeX via QuickJS
func (r *Renderer) RenderMath(ctx context.Context, latex string, displayMode bool) (string, error) {
	if err := r.withSchedulerAndClosedCheck(ctx, utils.TaskMath); err != nil {
		return "", err
	}
	defer r.wg.Done()

	// Acquire worker
	instance := <-r.pool
	defer func() {
		r.mu.Lock()
		isClosed := r.closed
		r.mu.Unlock()
		if !isClosed {
			r.pool <- instance
		}
	}()

	if instance.ctx == nil || instance.renderFn == nil {
		return "", fmt.Errorf("KaTeX not initialized in worker")
	}

	jsLatex := instance.ctx.NewString(latex)
	defer jsLatex.Free()

	// Update displayMode in pre-created options
	displayModeVal := instance.ctx.NewBool(displayMode)
	defer displayModeVal.Free()
	instance.opts.SetPropertyStr("displayMode", displayModeVal)

	res, err := instance.ctx.Invoke(instance.renderFn, instance.katex, jsLatex, instance.opts)
	if err != nil {
		return "", fmt.Errorf("KaTeX render failed: %w", err)
	}
	defer res.Free()

	return res.String(), nil
}

// RenderMathBatch renders a slice of LaTeX expressions in a single Go-to-JS bridge crossing.
func (r *Renderer) RenderMathBatch(ctx context.Context, expressions []models.MathExpression) ([]string, error) {
	if len(expressions) == 0 {
		return nil, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if r.scheduler != nil {
		if err := r.scheduler.Acquire(ctx, utils.TaskMath); err != nil {
			return nil, err
		}
		defer r.scheduler.Release(utils.TaskMath)
	}

	r.ensureInitialized()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, fmt.Errorf("renderer is closed")
	}
	r.wg.Add(1)
	r.mu.Unlock()

	defer r.wg.Done()

	// Acquire worker
	instance := <-r.pool
	defer func() {
		r.mu.Lock()
		isClosed := r.closed
		r.mu.Unlock()
		if !isClosed {
			r.pool <- instance
		}
	}()

	if instance.ctx == nil || instance.renderBatchFn == nil {
		return nil, fmt.Errorf("renderBatch not initialized in worker")
	}

	// Create parallel JS arrays
	jsLatexs := instance.ctx.NewArray()
	defer jsLatexs.Free()
	jsModes := instance.ctx.NewArray()
	defer jsModes.Free()

	for _, expr := range expressions {
		jsLatex := instance.ctx.NewString(expr.LaTeX)
		jsLatexs.Push(jsLatex)
		jsLatex.Free()

		mode := 0
		if expr.DisplayMode {
			mode = 1
		}
		jsMode := instance.ctx.NewInt32(int32(mode))
		jsModes.Push(jsMode)
		jsMode.Free()
	}

	res, err := instance.ctx.Invoke(instance.renderBatchFn, instance.ctx.Global(), jsLatexs.Value, jsModes.Value)
	if err != nil {
		return nil, fmt.Errorf("KaTeX batch render failed: %w", err)
	}
	defer res.Free()

	if !res.IsArray() {
		return nil, fmt.Errorf("expected array response from renderBatch, got %s", res.String())
	}

	length := res.Len()
	results := make([]string, length)
	arr := qjs.NewArray(res)
	for i := 0; i < int(length); i++ {
		item := arr.Get(int64(i))
		results[i] = item.String()
		item.Free()
	}

	return results, nil
}

// RenderAllMath renders multiple LaTeX expressions in parallel using the worker pool.
// It now uses a single response channel per call to reduce primitive churn.
func (r *Renderer) RenderAllMath(ctx context.Context, expressions []models.MathExpression, cache map[string]string) (map[string]string, error) {
	if len(expressions) == 0 {
		return make(map[string]string), nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	r.ensureInitialized()

	finalResults := make(map[string]string)
	var toRender []models.MathExpression

	// 1. Filter out cached expressions
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

	// 2. Use singleflight to deduplicate globally, and a single result channel
	type mathRes struct {
		hash string
		html string
		err  error
	}
	resChan := make(chan mathRes, len(toRender))
	var wg sync.WaitGroup

	for _, expr := range toRender {
		wg.Add(1)
		go func(e models.MathExpression) {
			defer wg.Done()

			val, err, _ := r.mathGroup.Do(e.Hash, func() (any, error) {
				singleResChan := make(chan string, 1)
				singleErrChan := make(chan error, 1)

				r.mathQueue <- mathRequest{
					expr: e,
					res:  singleResChan,
					err:  singleErrChan,
				}

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case html := <-singleResChan:
					return html, nil
				case err := <-singleErrChan:
					return nil, err
				}
			})

			if err != nil {
				resChan <- mathRes{hash: e.Hash, err: err}
			} else {
				html, ok := val.(string)
				if !ok {
					resChan <- mathRes{hash: e.Hash, err: fmt.Errorf("math render returned non-string result")}
				} else {
					resChan <- mathRes{hash: e.Hash, html: html}
				}
			}
		}(expr)
	}

	// Close channel when all renders are done
	go func() {
		wg.Wait()
		close(resChan)
	}()

	var firstErr error
	for res := range resChan {
		if res.err != nil {
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		finalResults[res.hash] = res.html
	}

	return finalResults, firstErr
}
