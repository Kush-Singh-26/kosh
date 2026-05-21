package content

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

func TestInjectShowcaseExamples(t *testing.T) {
	allMath := make(map[string]models.MathExpression)
	allD2 := make(map[string]models.D2Expression)

	injectShowcaseExamples(allMath, allD2)

	expectedMathHash := native.HashContent("math-block", showcaseLaTeX)
	expectedD2Hash := native.HashContent("d2", showcaseD2Code)

	if _, ok := allMath[expectedMathHash]; !ok {
		t.Errorf("showcase math not injected at expected hash %s", expectedMathHash)
	}
	if allMath[expectedMathHash].DisplayMode != true {
		t.Error("showcase math DisplayMode should be true")
	}
	if allMath[expectedMathHash].LaTeX != showcaseLaTeX {
		t.Error("showcase math LaTeX content mismatch")
	}

	if _, ok := allD2[expectedD2Hash]; !ok {
		t.Errorf("showcase D2 not injected at expected hash %s", expectedD2Hash)
	}
	if allD2[expectedD2Hash].Code != showcaseD2Code {
		t.Error("showcase D2 code mismatch")
	}

	if expectedMathHash != "0a85ea93f69eeb76" {
		t.Errorf("math hash mismatch: got %s, want 0a85ea93f69eeb76", expectedMathHash)
	}
	if expectedD2Hash != "6830177882580129" {
		t.Errorf("D2 hash mismatch: got %s, want 6830177882580129", expectedD2Hash)
	}
}

func TestExtractMathHashes(t *testing.T) {
	html := `<!--KOSH_MATH:0a85ea93f69eeb76-->
<p>some content</p>
<!--KOSH_MATH:abc123def4567890--><!--KOSH_MATH_REG:abc123def4567890:base64:true:1-->`

	hashes := mdParser.ExtractMathHashes(html)
	if len(hashes) != 2 {
		t.Fatalf("expected 2 hashes, got %d", len(hashes))
	}
	if hashes[0] != "0a85ea93f69eeb76" {
		t.Errorf("first hash mismatch: got %s", hashes[0])
	}
	if hashes[1] != "abc123def4567890" {
		t.Errorf("second hash mismatch: got %s", hashes[1])
	}
}
