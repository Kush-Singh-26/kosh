package parser

import (
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"strings"
	"testing"
)

func TestReplaceMathExpressions(t *testing.T) {
	html := `<p>Solve <!--KOSH_MATH:hash1--> and <!--KOSH_MATH:hash2--></p>`
	expressions := []native.MathExpression{
		{LaTeX: "x+1=0", DisplayMode: false, Hash: "hash1"},
		{LaTeX: "y^2=4", DisplayMode: true, Hash: "hash2"},
	}
	rendered := map[string]string{
		"hash1": "<span>RENDERED_INLINE</span>",
		"hash2": "<div>RENDERED_BLOCK</div>",
	}

	result := ReplaceMathExpressions(html, expressions, rendered)

	expected1 := `<span class="katex-inline"><span>RENDERED_INLINE</span></span>`
	expected2 := `<div class="katex-display"><div>RENDERED_BLOCK</div></div>`

	if !strings.Contains(result, expected1) {
		t.Errorf("Expected result to contain %q", expected1)
	}
	if !strings.Contains(result, expected2) {
		t.Errorf("Expected result to contain %q", expected2)
	}
}
