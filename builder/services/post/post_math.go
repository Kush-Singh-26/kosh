package post

import (
	"context"

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

func (service *postService) renderMath(ctx context.Context, path string, result *ParsedMarkdownResult) string {
	if len(result.MathExpressions) == 0 {
		return result.HTMLContent
	}

	cachedSubset := make(map[string]string)
	if service.diagramAdapter != nil {
		for _, expr := range result.MathExpressions {
			key := "math:" + expr.Hash
			if val, ok := service.diagramAdapter.GetLocal(key); ok {
				if renderedStr, ok := val.(string); ok {
					cachedSubset[expr.Hash] = renderedStr
				}
			}
		}
	}

	rendered, err := service.nativeRenderer.RenderAllMath(ctx, result.MathExpressions, cachedSubset)
	if err != nil {
		service.logger.Warn("Math render failed for post", "path", path, "error", err)
	}

	if service.diagramAdapter != nil && len(rendered) > 0 {
		newMath := make(map[string]any) // values are rendered HTML/SVG strings.
		for hash, content := range rendered {
			if _, ok := cachedSubset[hash]; !ok {
				key := "math:" + hash
				newMath[key] = content
			}
		}
		if len(newMath) > 0 {
			service.diagramAdapter.Merge(newMath)
		}
	}

	return mdParser.ReplaceMathExpressions(result.HTMLContent, result.MathExpressions, rendered)
}
