package post

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
)

func (s *postService) processCachedMath(html string, exprs []models.MathExpression) (string, bool) {
	if s.diagramAdapter == nil || len(exprs) == 0 {
		return html, true
	}

	renderedMath := make(map[string]string)
	missingCount := 0
	for _, expr := range exprs {
		key := "math:" + expr.Hash
		if v, ok := s.diagramAdapter.Get(key); ok {
			if s, ok := v.(string); ok {
				renderedMath[expr.Hash] = s
			}
		} else {
			missingCount++
		}
	}

	if missingCount > 0 {
		return html, false
	}

	if len(renderedMath) > 0 {
		return mdParser.ReplaceMathExpressions(html, exprs, renderedMath), true
	}
	return html, true
}

func (s *postService) renderMath(ctx context.Context, path string, res *ParsedMarkdownResult) string {
	if len(res.MathExpressions) == 0 {
		return res.HTMLContent
	}

	cachedSubset := make(map[string]string)
	if s.diagramAdapter != nil {
		for _, e := range res.MathExpressions {
			key := "math:" + e.Hash
			if v, ok := s.diagramAdapter.GetLocal(key); ok {
				if s, ok := v.(string); ok {
					cachedSubset[e.Hash] = s
				}
			}
		}
	}

	rendered, err := s.nativeRenderer.RenderAllMath(ctx, res.MathExpressions, cachedSubset)
	if err != nil {
		s.logger.Warn("Math render failed for post", "path", path, "error", err)
	}

	if s.diagramAdapter != nil && len(rendered) > 0 {
		newMath := make(map[string]any)
		for h, v := range rendered {
			if _, ok := cachedSubset[h]; !ok {
				key := "math:" + h
				newMath[key] = v
			}
		}
		if len(newMath) > 0 {
			s.diagramAdapter.Merge(newMath)
		}
	}

	return mdParser.ReplaceMathExpressions(res.HTMLContent, res.MathExpressions, rendered)
}
