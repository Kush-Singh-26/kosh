package core

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSnippetIntegrity_AnalyzerPositionsSync(t *testing.T) {
	content := "This is a test of the analyzer positions and offsets."

	_, _, positions, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

	for term, posList := range positions {
		offList, ok := offsets[term]
		if !ok {
			t.Errorf("Term %q has positions but no offsets", term)
			continue
		}

		if len(posList)*2 != len(offList) {
			t.Errorf("Term %q: positions count %d does not match offsets count %d",
				term, len(posList), len(offList))
		}

		for i, pos := range posList {
			offIdx := i * 2
			if offIdx+1 >= len(offList) {
				continue
			}
			start := offList[offIdx]
			end := offList[offIdx+1]
			extracted := content[start:end]
			if !strings.Contains(strings.ToLower(extracted), term) &&
				!strings.Contains(strings.ToLower(extracted), term[:len(term)-1]) {
				t.Errorf("Position %d <-> offset [%d,%d] mismatch for term %q: got %q",
					pos, start, end, term, extracted)
			}
		}
	}
}

func TestSnippetIntegrity_OffsetSourceRoundTrip(t *testing.T) {
	tests := []struct {
		content string
		terms   []string
	}{
		{
			content: "The quick brown fox jumps over the lazy dog.",
			terms:   []string{"fox", "dog", "quick"},
		},
		{
			content: "Machine learning is transforming software development.",
			terms:   []string{"machine", "learning", "software"},
		},
		{
			content: "Go is great. Rust is also great. Go concurrency is powerful.",
			terms:   []string{"go", "rust", "concurrency"},
		},
		{
			content: "日本語テスト content English テスト",
			terms:   []string{"content", "english"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.content[:min(30, len(tt.content))], func(t *testing.T) {
			_, _, _, offsets := DefaultAnalyzer.AnalyzeWithPositions(tt.content)

			for _, term := range tt.terms {
				termOffsets, ok := offsets[term]
				if !ok {
					continue
				}

				for i := 0; i < len(termOffsets); i += 2 {
					start := termOffsets[i]
					end := termOffsets[i+1]
					if start < 0 || end > len(tt.content) || start >= end {
						t.Errorf("Invalid offset [%d, %d] for term %q in content %q",
							start, end, term, tt.content)
						continue
					}

					if !utf8.RuneStart(tt.content[start]) {
						t.Errorf("Offset %d is not a valid UTF-8 rune start for term %q", start, term)
					}

					extracted := tt.content[start:end]
					if !strings.Contains(strings.ToLower(extracted), term) &&
						!strings.Contains(strings.ToLower(extracted), term[:len(term)-1]) {
						t.Errorf("Offset [%d, %d] extracted %q which does not match term %q",
							start, end, extracted, term)
					}
				}
			}
		})
	}
}

func TestSnippetIntegrity_RuneBoundaryAlignment(t *testing.T) {
	content := "日本語テスト文字列"

	tokens := TokenizeWithUnicode(content)

	for _, tok := range tokens {
		if !utf8.RuneStart(content[tok.Start]) {
			t.Errorf("Token start %d is not a rune boundary in content %q", tok.Start, content)
		}
		if tok.End > len(content) {
			t.Errorf("Token end %d exceeds content length %d", tok.End, len(content))
		}
		if tok.End < len(content) && !utf8.RuneStart(content[tok.End]) {
			t.Errorf("Token end %d is not a rune boundary in content %q", tok.End, content)
		}
	}
}

func TestSnippetIntegrity_OffsetDerivedFromTokens(t *testing.T) {
	content := "The quick brown fox"

	tokens := TokenizeWithUnicode(content)

	for _, tok := range tokens {
		extracted := content[tok.Start:tok.End]
		if extracted != tok.Value {
			t.Errorf("Token value %q does not match content slice %q at [%d:%d]",
				tok.Value, extracted, tok.Start, tok.End)
		}
	}
}

func TestSnippetIntegrity_EmptyContent(t *testing.T) {
	content := ""
	_, _, _, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

	if len(offsets) != 0 {
		t.Errorf("Empty content should produce no offsets, got %d", len(offsets))
	}
}

func TestSnippetIntegrity_OnlyStopWords(t *testing.T) {
	content := "the a an is are was were"
	_, mapping, positions, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

	if len(mapping) > 0 {
		t.Errorf("Only-stop-word content should produce no mapped terms, got %d", len(mapping))
	}
	if len(positions) > 0 {
		t.Errorf("Only-stop-word content should produce no positions, got %d", len(positions))
	}
	if len(offsets) > 0 {
		t.Errorf("Only-stop-word content should produce no offsets, got %d", len(offsets))
	}
}

func TestSnippetIntegrity_MultipleOccurrences(t *testing.T) {
	content := "A cat sat on a mat. The cat slept. Another cat roamed."
	_, _, _, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

	catOffsets, ok := offsets["cat"]
	if !ok {
		t.Fatal("Expected 'cat' in offsets")
	}

	if len(catOffsets)/2 < 3 {
		t.Errorf("Expected at least 3 occurrences of 'cat', got %d", len(catOffsets)/2)
	}

	for i := 0; i < len(catOffsets); i += 2 {
		start := catOffsets[i]
		end := catOffsets[i+1]
		extracted := content[start:end]
		if !strings.Contains(strings.ToLower(extracted), "cat") {
			t.Errorf("Offset [%d,%d] extracted %q which does not contain 'cat'",
				start, end, extracted)
		}
	}
}

func TestSnippetIntegrity_StemmedTermOffsets(t *testing.T) {
	content := "running quickly through the forest"

	_, mapping, positions, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

	if len(mapping) == 0 {
		t.Skip("No stemming occurred, skipping stemmed term offset test")
	}

	for orig, stem := range mapping {
		stemOffsets, ok := offsets[stem]
		if !ok {
			t.Logf("Stem %q from %q not directly in offsets (may be expected)", stem, orig)
		}

		if ok {
			for i := 0; i < len(stemOffsets); i += 2 {
				start := stemOffsets[i]
				end := stemOffsets[i+1]
				extracted := content[start:end]
				if !strings.Contains(strings.ToLower(extracted), orig) &&
					!strings.Contains(strings.ToLower(extracted), stem) {
					t.Errorf("Stemmed offset [%d,%d] for %q->%q does not match content %q",
						start, end, orig, stem, extracted)
				}
			}
		}

		_ = positions
	}
}

func TestSnippetIntegrity_UnicodeNormalization(t *testing.T) {
	tests := []string{
		"café résumé naïve",
		"日本語Go言語",
		"Müllerstraße",
		"العربية",
		"🎉 🚀 🐍",
	}

	for _, content := range tests {
		t.Run(content[:min(20, len(content))], func(t *testing.T) {
			_, _, _, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

			for term, offList := range offsets {
				for i := 0; i < len(offList); i += 2 {
					start := offList[i]
					end := offList[i+1]
					if start < 0 || end > len(content) || start >= end {
						t.Errorf("Invalid offset [%d,%d] for term %q in Unicode content %q",
							start, end, term, content)
					}
					if !utf8.ValidString(content[start:end]) {
						t.Errorf("Offset [%d,%d] produces invalid UTF-8 in content %q",
							start, end, content)
					}
				}
			}
		})
	}
}

func TestSnippetIntegrity_LongContentOffsetBounds(t *testing.T) {
	content := strings.Repeat("word ", 1000)
	_, _, _, offsets := DefaultAnalyzer.AnalyzeWithPositions(content)

	if len(offsets) == 0 {
		t.Error("Expected offsets from long content")
	}

	for term, offList := range offsets {
		for i := 0; i < len(offList); i += 2 {
			start := offList[i]
			end := offList[i+1]
			if start < 0 || end > len(content) {
				t.Errorf("Offset [%d,%d] for term %q out of bounds for content length %d",
					start, end, term, len(content))
			}
		}
	}
}

func TestSnippetIntegrity_TokenBoundaryPreservation(t *testing.T) {
	tests := []struct {
		content string
	}{
		{"Hello, world!"},
		{"word1.word2.word3"},
		{"hello-world_test"},
		{"123numbers456"},
		{"mixed_CaseWords"},
		{"  leading and trailing  "},
		{"line1\nline2\nline3"},
		{"tab\there"},
	}

	for _, tt := range tests {
		tokens := TokenizeWithUnicode(tt.content)

		for _, tok := range tokens {
			if tok.Start < 0 || tok.End > len(tt.content) || tok.Start > tok.End {
				t.Errorf("Token %q has invalid bounds [%d,%d] for content length %d",
					tok.Value, tok.Start, tok.End, len(tt.content))
			}

			extracted := tt.content[tok.Start:tok.End]
			if extracted != tok.Value {
				t.Errorf("Token %q does not match extracted %q at [%d,%d]",
					tok.Value, extracted, tok.Start, tok.End)
			}
		}
	}
}
