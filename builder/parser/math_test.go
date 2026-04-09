package parser

import (
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestReplaceMathExpressions(t *testing.T) {
	html := `<p>Solve <!--KOSH_MATH:hash1--> and <!--KOSH_MATH:hash2--></p>`
	expressions := []models.MathExpression{
		{LaTeX: "x+1=0", DisplayMode: false, Hash: "hash1"},
		{LaTeX: "y^2=4", DisplayMode: true, Hash: "hash2"},
	}
	rendered := map[string]string{
		"hash1": "<span>RENDERED_INLINE</span>",
		"hash2": "<div>RENDERED_BLOCK</div>",
	}

	result := ReplaceMathExpressions(html, expressions, rendered)
	
	expected1 := `<span class="katex-inline" data-latex="x+1=0"><button class="katex-copy-btn" aria-label="Copy LaTeX">Copy</button><span>RENDERED_INLINE</span></span>`
	expected2 := `<div class="katex-display" data-latex="y^2=4"><button class="katex-copy-btn" aria-label="Copy LaTeX">Copy</button><div>RENDERED_BLOCK</div></div>`

	if !strings.Contains(result, expected1) {
		t.Errorf("Expected result to contain %q, but got %q", expected1, result)
	}
	if !strings.Contains(result, expected2) {
		t.Errorf("Expected result to contain %q, but got %q", expected2, result)
	}
}
