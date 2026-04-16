package content

import (
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
)

func (service *contentService) processCachedMath(html string, expressions []models.MathExpression) (string, bool) {
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

func (service *contentService) processCachedD2(html string, expressions []models.D2Expression) (string, bool) {
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

func (service *contentService) renderSSRGlobal(ctx context.Context, tasks []renderTask) {
	if len(tasks) == 0 {
		return
	}

	allMath, allD2 := collectExpressions(tasks)

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
		renderedMath, mathErr = service.renderMathWithCache(ctx, allMath)
		if mathErr != nil {
			service.logger.Warn("Global Math render failed", "error", mathErr)
		}
	}()

	go func() {
		defer wg.Done()
		renderedD2, d2Err = service.renderD2WithCache(ctx, allD2)
		if d2Err != nil {
			service.logger.Warn("Global D2 render failed", "error", d2Err)
		}
	}()

	wg.Wait()

	if len(renderedMath) == 0 && len(renderedD2) == 0 {
		return
	}

	service.replaceExpressionsInTasks(tasks, renderedMath, renderedD2)
}

// collectExpressions gathers all Math and D2 expressions from tasks.
func collectExpressions(tasks []renderTask) (math map[string]models.MathExpression, d2 map[string]models.D2Expression) {
	math = make(map[string]models.MathExpression)
	d2 = make(map[string]models.D2Expression)

	for _, task := range tasks {
		if len(task.parseResult.MathExpressions) > 0 && mdParser.HasMathPlaceholders(task.htmlContent) {
			for _, expr := range task.parseResult.MathExpressions {
				math[expr.Hash] = expr
			}
		}
		if len(task.parseResult.D2Expressions) > 0 && mdParser.HasD2Placeholders(task.htmlContent) {
			for _, expr := range task.parseResult.D2Expressions {
				d2[expr.Hash] = expr
			}
		}
	}

	return math, d2
}

// renderMathWithCache renders Math expressions with caching.
func (service *contentService) renderMathWithCache(ctx context.Context, allMath map[string]models.MathExpression) (map[string]string, error) {
	if len(allMath) == 0 {
		return nil, nil
	}

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

	renderedMath, err := service.nativeRenderer.RenderAllMath(ctx, mathList, cachedMath)
	if err != nil {
		return nil, err
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

	return renderedMath, nil
}

// renderD2WithCache renders D2 expressions with caching.
func (service *contentService) renderD2WithCache(ctx context.Context, allD2 map[string]models.D2Expression) (map[string]models.SSRThemePair, error) {
	if len(allD2) == 0 {
		return nil, nil
	}

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

	renderedD2, err := service.nativeRenderer.RenderAllD2(ctx, d2List, cachedD2)
	if err != nil {
		return nil, err
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

	return renderedD2, nil
}

// replaceExpressionsInTasks replaces Math and D2 placeholders in task HTML content.
func (service *contentService) replaceExpressionsInTasks(tasks []renderTask, renderedMath map[string]string, renderedD2 map[string]models.SSRThemePair) {
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

func rewriteStaticAssetPaths(htmlContent, relativePrefix string) string {
	if relativePrefix == "" {
		return htmlContent
	}

	htmlContent = strings.ReplaceAll(htmlContent, "src=\"static/", "src=\""+relativePrefix+"static/")
	htmlContent = strings.ReplaceAll(htmlContent, "src=static/", "src="+relativePrefix+"static/")
	htmlContent = strings.ReplaceAll(htmlContent, "src=\"static/wasm/", "src=\""+relativePrefix+"static/wasm/")
	htmlContent = strings.ReplaceAll(htmlContent, "src=static/wasm/", "src="+relativePrefix+"static/wasm/")
	return htmlContent
}
