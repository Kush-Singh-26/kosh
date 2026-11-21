package parser

import (
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestNormalizeMathMLVariants(t *testing.T) {
	cases := map[string]string{
		`<mi mathvariant="double-struck">R</mi>`: `<mi>ℝ</mi>`,
		`<mi mathvariant="double-struck">w</mi>`: `<mi>𝕨</mi>`,
		`<mi mathvariant="script">L</mi>`:        `<mi>ℒ</mi>`,
		`<mn mathvariant="bold">529</mn>`:        `<mn>𝟓𝟐𝟗</mn>`,
		// No math-alphanumeric form exists for primes, so the run is left alone.
		`<mo mathvariant="bold" lspace="0em">′</mo>`: `<mo mathvariant="bold" lspace="0em">′</mo>`,
	}
	for in, want := range cases {
		if got := normalizeMathML(in); got != want {
			t.Errorf("normalizeMathML(%q) = %q, want %q", in, got, want)
		}
	}
}

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
