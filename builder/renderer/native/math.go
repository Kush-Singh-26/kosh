package native

import (
	"fmt"
	"log"
	"sync"
)

// RenderMath renders a single LaTeX expression to HTML using KaTeX via goja
func (r *Renderer) RenderMath(latex string, displayMode bool) (string, error) {
	r.ensureInitialized()

	// Acquire worker
	instance := <-r.pool
	defer func() { r.pool <- instance }() // Release worker

	instance.ensureInitialized()

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
		if _, exists := cache[expr.Hash]; exists {
			continue
		}

		wg.Add(1)
		go func(e MathExpression) {
			defer wg.Done()

			// Acquire worker from pool
			instance := <-r.pool
			defer func() { r.pool <- instance }()

			instance.ensureInitialized()

			if instance.vm == nil || instance.renderFn == nil {
				return
			}

			opts := instance.vm.NewObject()
			_ = opts.Set("displayMode", e.DisplayMode)
			_ = opts.Set("throwOnError", false)
			_ = opts.Set("output", "html")

			res, err := instance.renderFn(instance.katex, instance.vm.ToValue(e.LaTeX), opts)
			if err != nil {
				log.Printf("   ⚠️  LaTeX render failed for %s: %v", e.Hash[:8], err)
				return
			}

			mu.Lock()
			results[e.Hash] = res.String()
			mu.Unlock()
		}(expr)
	}

	wg.Wait()
	return results, nil
}
