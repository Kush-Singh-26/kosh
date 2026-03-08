package native

import (
	"fmt"
	"log/slog"
	"sync"
)

// RenderMath renders a single LaTeX expression to HTML using KaTeX via goja
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

	if instance.vm == nil || instance.renderFn == nil {
		return "", fmt.Errorf("KaTeX not initialized in worker")
	}

	opts := instance.vm.NewObject()
	_ = opts.Set("displayMode", displayMode)
	_ = opts.Set("throwOnError", false)
	_ = opts.Set("output", "html")

	result, err := instance.renderFn(instance.katex, instance.vm.ToValue(latex), opts)
	if err != nil {
		return "", fmt.Errorf("KaTeX render failed: %w", err)
	}

	return result.String(), nil
}

// MathExpression represents a LaTeX expression with its metadata
type MathExpression struct {
	LaTeX       string
	DisplayMode bool
	Hash        string
}

// RenderAllMath renders multiple LaTeX expressions in parallel using the worker pool
func (r *Renderer) RenderAllMath(expressions []MathExpression, cache map[string]string) (map[string]string, error) {
	if len(expressions) == 0 {
		return make(map[string]string), nil
	}

	r.ensureInitialized()

	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, expr := range expressions {
		if val, ok := cache[expr.Hash]; ok {
			mu.Lock()
			results[expr.Hash] = val
			mu.Unlock()
			continue
		}

		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			break
		}
		r.wg.Add(1)
		r.mu.Unlock()

		wg.Add(1)
		go func(e MathExpression) {
			defer wg.Done()
			defer r.wg.Done()

			v, err, _ := r.mathGroup.Do(e.Hash, func() (interface{}, error) {
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

				if instance.vm == nil || instance.renderFn == nil {
					return "", fmt.Errorf("KaTeX not initialized")
				}

				opts := instance.vm.NewObject()
				_ = opts.Set("displayMode", e.DisplayMode)
				_ = opts.Set("throwOnError", false)
				_ = opts.Set("output", "html")

				res, renderErr := instance.renderFn(instance.katex, instance.vm.ToValue(e.LaTeX), opts)
				if renderErr != nil {
					slog.Warn("   ⚠️  LaTeX render failed", "hash", e.Hash[:8], "error", renderErr)
					return "", renderErr
				}

				return res.String(), nil
			})

			if err == nil {
				mu.Lock()
				results[e.Hash] = v.(string)
				mu.Unlock()
			}
		}(expr)
	}

	wg.Wait()
	return results, nil
}
