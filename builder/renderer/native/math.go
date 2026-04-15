package native

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/fastschema/qjs"
)

type mathInput struct {
	LaTeX       string `json:"l"`
	DisplayMode bool   `json:"d"`
}

func (r *Renderer) chunkAndRender(ctx context.Context, expressions []models.MathExpression, operation string) (map[string]string, error) {
	numWorkers := min(r.numWorkers, len(expressions))
	chunkSize := (len(expressions) + numWorkers - 1) / numWorkers

	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var globalErr error

	for i := 0; i < numWorkers; i++ {
		start := i * chunkSize
		if start >= len(expressions) {
			break
		}
		end := min(start+chunkSize, len(expressions))

		wg.Add(1)
		chunk := expressions[start:end]
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       ctx,
			Logger:    slog.Default(),
			Operation: operation,
			Fn: func() error {
				rendered, err := r.RenderMathBatch(ctx, chunk)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					if globalErr == nil {
						globalErr = err
					}
					return nil
				}
				for j, html := range rendered {
					results[chunk[j].Hash] = html
				}
				return nil
			},
			Cleanup: wg.Done,
		})
	}

	wg.Wait()
	return results, globalErr
}

// RenderGlobalBatch renders all unique math expressions across the entire site
func (r *Renderer) RenderGlobalBatch(ctx context.Context, expressions []models.MathExpression) (map[string]string, error) {
	if len(expressions) == 0 {
		return make(map[string]string), nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.ensureInitialized()

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
	timer := timeutil.StartPhase("Global math render")
	defer timer.Stop()

	return r.chunkAndRender(ctx, uniqueExprs, "math batch render")
}

// RenderMath renders a single LaTeX expression to HTML using KaTeX via QuickJS
func (r *Renderer) RenderMath(ctx context.Context, latex string, displayMode bool) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if r.scheduler != nil {
		if err := r.scheduler.Acquire(ctx, scheduler.TaskMath); err != nil {
			return "", err
		}
		defer r.scheduler.Release(scheduler.TaskMath)
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

	// Acquire worker
	instance := <-r.pool
	defer func() {
		r.mu.Lock()
		isClosed := r.isClosed
		r.mu.Unlock()
		if !isClosed {
			r.pool <- instance
		}
	}()

	if instance.ctx == nil || instance.renderFn == nil {
		return "", errKaTeXNotInitialized
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

	if err := r.acquireTask(ctx, scheduler.TaskMath); err != nil {
		return nil, err
	}
	defer r.releaseTask(scheduler.TaskMath)

	r.ensureInitialized()

	instance, err := r.acquireWorker()
	if err != nil {
		return nil, err
	}
	defer r.releaseWorker(instance)

	if instance.ctx == nil || instance.renderBatchFn == nil {
		return nil, errRenderBatchNotInit
	}

	jsInput, err := encodeMathBatch(instance.ctx, expressions)
	if err != nil {
		return nil, err
	}
	defer jsInput.Free()

	res, err := instance.ctx.Invoke(instance.renderBatchFn, instance.ctx.Global(), jsInput)
	if err != nil {
		return nil, fmt.Errorf("KaTeX batch render failed: %w", err)
	}
	defer res.Free()

	return decodeMathResults(res)
}

func (r *Renderer) acquireTask(ctx context.Context, taskType scheduler.TaskType) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.scheduler != nil {
		if err := r.scheduler.Acquire(ctx, taskType); err != nil {
			return err
		}
	}

	r.mu.Lock()
	if r.isClosed {
		r.mu.Unlock()
		if r.scheduler != nil {
			r.scheduler.Release(taskType)
		}
		return errRendererClosed
	}
	r.taskWg.Add(1)
	r.mu.Unlock()
	return nil
}

func (r *Renderer) releaseTask(taskType scheduler.TaskType) {
	if r.scheduler != nil {
		r.scheduler.Release(taskType)
	}
	r.taskWg.Done()
}

func (r *Renderer) acquireWorker() (*instance, error) {
	return <-r.pool, nil
}

func (r *Renderer) releaseWorker(inst *instance) {
	r.mu.Lock()
	isClosed := r.isClosed
	r.mu.Unlock()
	if !isClosed {
		r.pool <- inst
	}
}

func encodeMathBatch(ctx *qjs.Context, expressions []models.MathExpression) (*qjs.Value, error) {
	input := make([]mathInput, len(expressions))
	for i, expr := range expressions {
		input[i] = mathInput{
			LaTeX:       expr.LaTeX,
			DisplayMode: expr.DisplayMode,
		}
	}

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

	if err := json.NewEncoder(buf).Encode(input); err != nil {
		return nil, fmt.Errorf("failed to encode math batch: %w", err)
	}

	jsInput := ctx.NewString(buf.String())
	return jsInput, nil
}

func decodeMathResults(res *qjs.Value) ([]string, error) {
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

// RenderAllMath renders multiple math expressions in a single batch.
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

	results, err := r.chunkAndRender(ctx, toRender, "math render")
	if err != nil {
		return finalResults, err
	}

	for hash, html := range results {
		finalResults[hash] = html
	}

	return finalResults, nil
}
