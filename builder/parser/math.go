package parser

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/parser"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

var (
	mathExpressionsKey = parser.NewContextKey()
	mathLateReplaceRe  = regexp.MustCompile(`<!--KOSH_MATH:([a-f0-9]+)-->(?:<!--KOSH_MATH_REG:([a-f0-9]+):([^:]+):([^:]+):([^:]+)-->)?`)
	// unarySignRe matches a sign that starts an operand. MathML spacing is
	// form-based, so a sign after "=", ",", an opening fence or a row/fraction
	// start would otherwise get binary spacing.
	unarySignRe = regexp.MustCompile(`(<mrow>|<mtd>|<msqrt>|<mo[^>]*>(?:=|,|\(|\[|\{|\+|−|×|⋅|·|±)</mo>)<mo>([+−])</mo>`)
	// accentRe swaps KaTeX's spacing accents for modifier/combining marks: Chrome
	// floats the spacing glyphs far above the base, while these sit like KaTeX.
	accentRe = strings.NewReplacer(
		"<mo>^</mo>", `<mo>&#x2C6;</mo>`,
		"<mo>~</mo>", `<mo>&#x2DC;</mo>`,
		"<mo>ˉ</mo>", `<mo>&#x304;</mo>`,
	)
	// mathVariantRe matches runs Chrome renders in the wrong style: it ignores
	// mathvariant, so bold/double-struck/script text would fall back to italic.
	mathVariantRe = regexp.MustCompile(`<m([int]) mathvariant="(bold|bold-italic|double-struck|script)">([^<]+)</m[int]>`)
)

// mathAlphabet locates a math-alphanumeric block; a variant may place some
// letters outside it (Script L, double-struck R, ...), hence the holes.
type mathAlphabet struct {
	upper, lower, digit    rune
	upperHoles, lowerHoles map[rune]rune
}

var mathAlphabets = map[string]mathAlphabet{
	"bold":        {0x1D400, 0x1D41A, 0x1D7CE, nil, nil},
	"bold-italic": {0x1D468, 0x1D482, 0, nil, nil},
	"double-struck": {0x1D538, 0x1D552, 0x1D7D8, map[rune]rune{
		'C': 0x2102, 'H': 0x210D, 'N': 0x2115, 'P': 0x2119, 'Q': 0x211A, 'R': 0x211D, 'Z': 0x2124,
	}, nil},
	"script": {0x1D49C, 0x1D4B6, 0, map[rune]rune{
		'B': 0x212C, 'E': 0x2130, 'F': 0x2131, 'H': 0x210B, 'I': 0x2110, 'L': 0x2112,
		'M': 0x2133, 'R': 0x211B,
	}, map[rune]rune{'e': 0x212F, 'g': 0x210A, 'o': 0x2134}},
}

// mathVariantRune maps an ASCII alphanumeric to its math-alphanumeric form.
func (a mathAlphabet) rune(r rune) (rune, bool) {
	switch {
	case r >= 'A' && r <= 'Z':
		if h, ok := a.upperHoles[r]; ok {
			return h, true
		}
		return a.upper + r - 'A', a.upper != 0
	case r >= 'a' && r <= 'z':
		if h, ok := a.lowerHoles[r]; ok {
			return h, true
		}
		return a.lower + r - 'a', a.lower != 0
	case r >= '0' && r <= '9' && a.digit != 0:
		return a.digit + r - '0', true
	}
	return 0, false
}

// applyMathVariants rewrites styled MathML runs to the literal math-alphanumeric
// characters, which Chrome does render with the embedded math font.
func applyMathVariants(s string) string {
	return mathVariantRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := mathVariantRe.FindStringSubmatch(m)
		alphabet := mathAlphabets[parts[2]]
		runes := []rune(parts[3])
		for i, r := range runes {
			converted, ok := alphabet.rune(r)
			if !ok {
				return m
			}
			runes[i] = converted
		}
		return "<m" + parts[1] + ">" + string(runes) + "</m" + parts[1] + ">"
	})
}

// normalizeMathML restores cues KaTeX's HTML output gives that browsers drop from MathML.
func normalizeMathML(s string) string {
	s = strings.ReplaceAll(s, `rowspacing="0.25em"`, `rowspacing="0.5em"`)
	// KaTeX emits a zero-height mpadded for \vdots rows; Chrome collapses it and overlaps neighbors.
	s = strings.ReplaceAll(s, `<mpadded height="0em" voffset="0em">`, `<mpadded>`)
	s = accentRe.Replace(s)
	s = applyMathVariants(s)
	return unarySignRe.ReplaceAllString(s, `${1}<mo lspace="0" rspace="0">${2}</mo>`)
}

// ReplaceMathExpressions replaces LaTeX placeholders in HTML with rendered output.
func ReplaceMathExpressions(htmlContent string, expressions []models.MathExpression, rendered map[string]string) string {
	if len(expressions) == 0 {
		return htmlContent
	}

	replacements := make([]string, 0, len(expressions)*2)
	for _, expr := range expressions {
		if renderedHTML, ok := rendered[expr.Hash]; ok {
			renderedHTML = normalizeMathML(renderedHTML)
			placeholder := "<!--KOSH_MATH:" + expr.Hash + "-->"
			escapedLatex := html.EscapeString(expr.LaTeX)
			copyBtn := `<button class="katex-copy-btn" aria-label="Copy LaTeX">Copy</button>`
			var replacement string
			if expr.DisplayMode {
				replacement = fmt.Sprintf(`<div class="katex-display" data-latex="%s">%s%s</div>`, escapedLatex, copyBtn, renderedHTML)
			} else {
				replacement = fmt.Sprintf(`<span class="katex-inline" data-latex="%s">%s%s</span>`, escapedLatex, copyBtn, renderedHTML)
			}
			replacements = append(replacements, placeholder, replacement)
		}
	}

	if len(replacements) == 0 {
		return htmlContent
	}

	return strings.NewReplacer(replacements...).Replace(htmlContent)
}

// RenderMathOptions configures RenderMathForHTML.
type RenderMathOptions struct {
	// Required
	Ctx          context.Context
	HTML         string
	Renderer     *native.Renderer
	PreCollected []models.MathExpression

	// Optional
	CacheLookup func(string) (string, bool)
}

// RenderMathForHTML extracts, renders, and replaces all LaTeX in HTML.
func RenderMathForHTML(opts RenderMathOptions) (string, []string, map[string]string) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	if opts.Renderer == nil {
		panic("RenderMathForHTML: Renderer is nil")
	}
	if len(opts.PreCollected) == 0 {
		return opts.HTML, nil, nil
	}

	hashes := make([]string, len(opts.PreCollected))
	for i, expr := range opts.PreCollected {
		hashes[i] = expr.Hash
	}

	cachedSubset := make(map[string]string, len(opts.PreCollected))
	if opts.CacheLookup != nil {
		for _, expr := range opts.PreCollected {
			if v, ok := opts.CacheLookup(expr.Hash); ok {
				cachedSubset[expr.Hash] = v
			}
		}
	}

	rendered, err := opts.Renderer.RenderAllMath(opts.Ctx, opts.PreCollected, cachedSubset)
	if err != nil {
		slog.Warn("LaTeX batch render failed", "error", err)
	}

	newEntries := make(map[string]string)
	for hash, value := range rendered {
		if _, existed := cachedSubset[hash]; !existed {
			newEntries[hash] = value
		}
	}

	return ReplaceMathExpressions(opts.HTML, opts.PreCollected, rendered), hashes, newEntries
}

// GetMathExpressions retrieves math from context
func GetMathExpressions(pc parser.Context) []models.MathExpression {
	if v := pc.Get(mathExpressionsKey); v != nil {
		if exprs, ok := v.([]models.MathExpression); ok {
			return exprs
		}
	}
	return nil
}

// HasMathPlaceholders checks if the HTML content has math placeholders.
func HasMathPlaceholders(html string) bool {
	return strings.Contains(html, "<!--KOSH_MATH:")
}

// ExtractMathHashes returns all unique math expression hashes found in the HTML.
func ExtractMathHashes(html string) []string {
	if !HasMathPlaceholders(html) {
		return nil
	}
	matches := mathLateReplaceRe.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	hashes := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			hash := m[1]
			if _, ok := seen[hash]; !ok {
				seen[hash] = struct{}{}
				hashes = append(hashes, hash)
			}
		}
	}
	return hashes
}

// LateReplaceMath performs a final replacement of Math placeholders using registry comments.
// This is used for TOC and theme fragments where normal AST-based replacement isn't possible.
func LateReplaceMath(htmlContent string, rendered map[string]string) string {
	if len(rendered) == 0 || !HasMathPlaceholders(htmlContent) {
		return htmlContent
	}

	return mathLateReplaceRe.ReplaceAllStringFunc(htmlContent, func(match string) string {
		parts := mathLateReplaceRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		hash := parts[1]
		renderedHTML, ok := rendered[hash]
		if !ok {
			return match
		}
		renderedHTML = normalizeMathML(renderedHTML)

		// If we have registry info, we can build a full KaTeX container
		if len(parts) >= 6 && parts[3] != "" {
			latexBytes, _ := base64.StdEncoding.DecodeString(parts[3])
			escapedLatex := html.EscapeString(string(latexBytes))
			displayMode := parts[4] == "true"
			copyBtn := `<button class="katex-copy-btn" aria-label="Copy LaTeX">Copy</button>`

			if displayMode {
				return fmt.Sprintf(`<div class="katex-display" data-latex="%s">%s%s</div>`, escapedLatex, copyBtn, renderedHTML)
			}
			return fmt.Sprintf(`<span class="katex-inline" data-latex="%s">%s%s</span>`, escapedLatex, copyBtn, renderedHTML)
		}

		// Fallback for when registry is missing (should not happen with new parser)
		return renderedHTML
	})
}
