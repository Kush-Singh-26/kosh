package native

import (
	"fmt"

	"github.com/fastschema/qjs"
)

// RenderMath renders a single LaTeX expression to HTML using KaTeX via QuickJS
func (r *Renderer) RenderMath(latex string, displayMode bool) (string, error) {
	r.ensureInitialized()

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", fmt.Errorf("renderer is closed")
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

// MathExpression represents a LaTeX expression with its metadata
type MathExpression struct {
	LaTeX       string `json:"latex"`
	DisplayMode bool   `json:"displayMode"`
	Hash        string `json:"-"`
}

// RenderMathBatch renders a slice of LaTeX expressions in a single Go-to-JS bridge crossing.
func (r *Renderer) RenderMathBatch(expressions []MathExpression) ([]string, error) {
	if len(expressions) == 0 {
		return nil, nil
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

	// Create JS array of objects
	jsExprs := instance.ctx.NewArray()
	defer jsExprs.Free()

	for _, expr := range expressions {
		obj := instance.ctx.NewObject()
		obj.SetPropertyStr("latex", instance.ctx.NewString(expr.LaTeX))
		obj.SetPropertyStr("displayMode", instance.ctx.NewBool(expr.DisplayMode))
		jsExprs.Push(obj)
		obj.Free()
	}

	res, err := instance.ctx.Invoke(instance.renderBatchFn, instance.ctx.Global(), jsExprs.Value)
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
// It now uses batching to reduce Go-to-JS bridge overhead.
func (r *Renderer) RenderAllMath(expressions []MathExpression, cache map[string]string) (map[string]string, error) {
	if len(expressions) == 0 {
		return make(map[string]string), nil
	}

	r.ensureInitialized()

	finalResults := make(map[string]string)
	var toRender []MathExpression
	var toRenderHashes []string

	// 1. Filter out cached expressions and deduplicate
	seenInPost := make(map[string]bool)
	for _, expr := range expressions {
		if seenInPost[expr.Hash] {
			continue
		}
		seenInPost[expr.Hash] = true

		if val, ok := cache[expr.Hash]; ok {
			finalResults[expr.Hash] = val
		} else {
			toRender = append(toRender, expr)
			toRenderHashes = append(toRenderHashes, expr.Hash)
		}
	}

	if len(toRender) == 0 {
		return finalResults, nil
	}

	// 2. For those needing rendering, use singleflight to avoid redundant work across posts
	// We still use goroutines for singleflight coverage, but we batch the actual JS work.

	// Optimization: if there's only 1 expression, just do it.
	if len(toRender) == 1 {
		res, err, _ := r.mathGroup.Do(toRenderHashes[0], func() (interface{}, error) {
			return r.renderMathSingle(toRender[0].LaTeX, toRender[0].DisplayMode)
		})
		if err == nil {
			finalResults[toRenderHashes[0]] = res.(string)
		}
		return finalResults, nil
	}

	// For multiple expressions, we'll try to batch them.
	// This is slightly tricky with singleflight because singleflight is per-key.
	// We'll compromise: batch the expressions that are not currently being handled by singleflight.

	results, err := r.RenderMathBatch(toRender)
	if err != nil {
		return nil, err
	}

	for i, hash := range toRenderHashes {
		finalResults[hash] = results[i]
	}

	return finalResults, nil
}

// renderMathSingle renders a single LaTeX expression without using singleflight
// (used internally by singleflight)
func (r *Renderer) renderMathSingle(latex string, displayMode bool) (string, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", fmt.Errorf("renderer is closed")
	}
	r.wg.Add(1)
	r.mu.Unlock()

	defer r.wg.Done()

	// Acquire worker from pool
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
