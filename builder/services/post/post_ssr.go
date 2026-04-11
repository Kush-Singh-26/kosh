package post

import (
	"context"
	"runtime"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
)

func (service *postService) processCachedMath(html string, expressions []models.MathExpression) (string, bool) {
	if service.diagramAdapter == nil || len(expressions) == 0 {
		return html, true
	}

	renderedMath := make(map[string]string)
	missingCount := 0
	for _, expr := range expressions {
		key := "math:" + expr.Hash
		if val, ok := service.diagramAdapter.Get(key); ok {
			if renderedStr, ok := val.(string); ok {
				renderedMath[expr.Hash] = renderedStr
			}
		} else {
			missingCount++
		}
	}

	if missingCount > 0 {
		return html, false
	}

	if len(renderedMath) > 0 {
		return mdParser.ReplaceMathExpressions(html, expressions, renderedMath), true
	}
	return html, true
}

func (service *postService) processCachedD2(html string, expressions []models.D2Expression) (string, bool) {
	if service.diagramAdapter == nil || len(expressions) == 0 {
		return html, true
	}

	renderedD2 := make(map[string]models.SSRThemePair)
	missingCount := 0
	for _, expr := range expressions {
		key := "d2:" + expr.Hash
		if val, ok := service.diagramAdapter.Get(key); ok {
			if pair, ok := val.(models.SSRThemePair); ok {
				renderedD2[expr.Hash] = pair
			}
		} else {
			missingCount++
		}
	}

	if missingCount > 0 {
		return html, false
	}

	if len(renderedD2) > 0 {
		return mdParser.ReplaceD2Expressions(html, expressions, renderedD2), true
	}
	return html, true
}

func (service *postService) renderSSRGlobal(ctx context.Context, tasks []renderTask) {
	if len(tasks) == 0 {
		return
	}

	allMath := make(map[string]models.MathExpression)
	allD2 := make(map[string]models.D2Expression)

	for _, task := range tasks {
		if len(task.parseResult.MathExpressions) > 0 {
			if mdParser.HasMathPlaceholders(task.htmlContent) {
				for _, expr := range task.parseResult.MathExpressions {
					allMath[expr.Hash] = expr
				}
			}
		}
		if len(task.parseResult.D2Expressions) > 0 {
			if mdParser.HasD2Placeholders(task.htmlContent) {
				for _, expr := range task.parseResult.D2Expressions {
					allD2[expr.Hash] = expr
				}
			}
		}
	}

	if len(allMath) == 0 && len(allD2) == 0 {
		return
	}

	var (
		renderedMath map[string]string
		renderedD2   map[string]models.SSRThemePair
		mathErr      error
		d2Err        error
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if len(allMath) > 0 {
			mathList := make([]models.MathExpression, 0, len(allMath))
			for _, expr := range allMath {
				mathList = append(mathList, expr)
			}

			cachedMath := make(map[string]string)
			if service.diagramAdapter != nil {
				for _, expr := range mathList {
					key := "math:" + expr.Hash
					if val, ok := service.diagramAdapter.GetLocal(key); ok {
						if renderedStr, ok := val.(string); ok {
							cachedMath[expr.Hash] = renderedStr
						}
					}
				}
			}

			renderedMath, mathErr = service.nativeRenderer.RenderAllMath(ctx, mathList, cachedMath)
			if mathErr != nil {
				service.logger.Warn("Global Math render failed", "error", mathErr)
				return
			}

			if service.diagramAdapter != nil && len(renderedMath) > 0 {
				newMath := make(map[string]any)
				for hash, content := range renderedMath {
					if _, ok := cachedMath[hash]; !ok {
						newMath["math:"+hash] = content
					}
				}
				if len(newMath) > 0 {
					service.diagramAdapter.Merge(newMath)
				}
			}
		}
	}()

	go func() {
		defer wg.Done()
		if len(allD2) > 0 {
			d2List := make([]models.D2Expression, 0, len(allD2))
			for _, expr := range allD2 {
				d2List = append(d2List, expr)
			}

			cachedD2 := make(map[string]models.SSRThemePair)
			if service.diagramAdapter != nil {
				for _, expr := range d2List {
					key := "d2:" + expr.Hash
					if val, ok := service.diagramAdapter.GetLocal(key); ok {
						if pair, ok := val.(models.SSRThemePair); ok {
							cachedD2[expr.Hash] = pair
						}
					}
				}
			}

			renderedD2, d2Err = service.nativeRenderer.RenderAllD2(ctx, d2List, cachedD2)
			if d2Err != nil {
				service.logger.Warn("Global D2 render failed", "error", d2Err)
				return
			}

			if service.diagramAdapter != nil && len(renderedD2) > 0 {
				newD2 := make(map[string]any)
				for hash, pair := range renderedD2 {
					if _, ok := cachedD2[hash]; !ok {
						newD2["d2:"+hash] = pair
					}
				}
				if len(newD2) > 0 {
					service.diagramAdapter.Merge(newD2)
				}
			}
		}
	}()

	wg.Wait()

	if len(renderedMath) == 0 && len(renderedD2) == 0 {
		return
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(tasks) {
		numWorkers = len(tasks)
	}
	if numWorkers < 2 {
		numWorkers = 2
	}

	chunkSize := (len(tasks) + numWorkers - 1) / numWorkers

	var replaceWg sync.WaitGroup
	for chunk := 0; chunk < numWorkers; chunk++ {
		replaceWg.Add(1)
		go func(chunk int) {
			defer replaceWg.Done()
			start := chunk * chunkSize
			if start >= len(tasks) {
				return
			}
			end := start + chunkSize
			if end > len(tasks) {
				end = len(tasks)
			}

			for i := start; i < end; i++ {
				if len(renderedMath) > 0 && len(tasks[i].parseResult.MathExpressions) > 0 && mdParser.HasMathPlaceholders(tasks[i].htmlContent) {
					tasks[i].htmlContent = mdParser.ReplaceMathExpressions(tasks[i].htmlContent, tasks[i].parseResult.MathExpressions, renderedMath)
				}
				if len(renderedD2) > 0 && len(tasks[i].parseResult.D2Expressions) > 0 && mdParser.HasD2Placeholders(tasks[i].htmlContent) {
					tasks[i].htmlContent = mdParser.ReplaceD2Expressions(tasks[i].htmlContent, tasks[i].parseResult.D2Expressions, renderedD2)
				}
			}
		}(chunk)
	}
	replaceWg.Wait()
}
