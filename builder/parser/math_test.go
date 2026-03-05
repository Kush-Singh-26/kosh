package parser

import (
	"testing"
)

func TestMathLexer_Basic(t *testing.T) {
	input := `
		Inline math: $E=mc^2$
		Block math:
		$$
		a^2 + b^2 = c^2
		$$
		Display math: \[ \int f(x) dx \]
		Inline paren: \( \sqrt{2} \)
		Escaped: \$100
	`

	lexer := NewMathLexer(input)
	matches := lexer.Scan()

	expectedCount := 4
	if len(matches) != expectedCount {
		t.Errorf("Expected %d matches, got %d", expectedCount, len(matches))
	}

	types := make(map[MathType]int)
	for _, m := range matches {
		types[m.Type]++
	}

	if types[MathInline] != 1 {
		t.Errorf("Expected 1 inline math, got %d", types[MathInline])
	}
	if types[MathBlock] != 1 {
		t.Errorf("Expected 1 block math, got %d", types[MathBlock])
	}
	if types[MathDisplay] != 1 {
		t.Errorf("Expected 1 display math, got %d", types[MathDisplay])
	}
	if types[MathParen] != 1 {
		t.Errorf("Expected 1 paren math, got %d", types[MathParen])
	}
}

func TestMathLexer_Escape(t *testing.T) {
	input := `Price is \$5.00 and \$10.00`
	lexer := NewMathLexer(input)
	matches := lexer.Scan()

	if len(matches) != 0 {
		t.Errorf("Expected 0 matches (all escaped), got %d", len(matches))
	}
}

func TestExtractMathExpressions(t *testing.T) {
	html := `<p>Solve $x+1=0$ and $$y^2=4$$</p>`
	exprs := ExtractMathExpressions(html)

	if len(exprs) != 2 {
		t.Errorf("Expected 2 expressions, got %d", len(exprs))
	}

	if exprs[0].LaTeX != "x+1=0" {
		t.Errorf("Expected 'x+1=0', got %q", exprs[0].LaTeX)
	}
	if exprs[1].LaTeX != "y^2=4" {
		t.Errorf("Expected 'y^2=4', got %q", exprs[1].LaTeX)
	}
}

func TestMathLexer_Currency(t *testing.T) {
	// Original regex: inlineMathRegex = regexp.MustCompile(`\$((?:\\.|[^$\n<>])+?)\$`)
	// We handle currency starting with digits in ExtractMathExpressions logic
	html := `Price is $5 and $10`
	exprs := ExtractMathExpressions(html)

	if len(exprs) != 0 {
		t.Errorf("Expected 0 expressions (currency filtered), got %d", len(exprs))
	}
}
