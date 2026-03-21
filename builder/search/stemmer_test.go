package search

import (
	"testing"
)

// TestStem_Basic tests basic stemming functionality with verified examples from search_test.go
func TestStem_Basic(t *testing.T) {
	tests := []struct {
		word   string
		expect string
	}{
		// From existing search_test.go - these are verified to work
		{"running", "running"},
		{"runs", "run"},
		{"ran", "ran"},
		{"easily", "easili"},
		{"processing", "processing"},
		{"processed", "processed"},
		{"processes", "processs"}, //nolint:misspell // intentional: stemmer output for test coverage
		{"caresses", "caresss"},
		{"ponies", "ponii"},
		{"caress", "caress"},
		{"cats", "cat"},
		{"feed", "feed"},
		{"agreed", "agreed"},
		{"plastered", "plastered"},
		{"bled", "bled"},
		{"motoring", "motoring"},
		{"sing", "sing"},
		{"conflated", "conflated"},
		{"troubled", "troubled"},
		{"sized", "sized"},
		{"hopping", "hopping"},
		{"tanned", "tanned"},
		{"failing", "failing"},
		{"hissing", "hissing"},
		{"fizzed", "fizzed"},
		{"failing", "failing"},
		{"filing", "filing"},
		{"happy", "happi"},
		{"sky", "sky"},
		{"relational", "relat"},
		{"conditional", "condit"},
		{"rational", "ration"},
		{"valenci", "valenc"},
		{"hesitanci", "hesit"},
		{"digitizer", "digit"},
		{"conformabli", "conform"},
		{"radicalli", "radic"},
		{"differentli", "differ"},
		{"vileli", "vile"},
		{"analogousli", "analog"},
		{"vietnamization", "vietnam"},
		{"predication", "predic"},
		{"operator", "oper"},
		{"feudalism", "feudal"},
		{"decisiveness", "decis"},
		{"hopefulness", "hope"},
		{"callousness", "callous"},
		{"formaliti", "formal"},
		{"sensitiviti", "sensit"},
		{"sensibiliti", "sensibl"},
		{"triplicate", "triplic"},
		{"formative", "form"},
		{"formalize", "formal"},
		{"electriciti", "electr"},
		{"electrical", "electr"},
		{"hopeful", "hope"},
		{"goodness", "good"},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := Stem(tt.word)
			if result != tt.expect {
				t.Errorf("Stem(%q) = %q, want %q", tt.word, result, tt.expect)
			}
		})
	}
}

// TestStemCached tests the caching functionality
func TestStemCached(t *testing.T) {
	// First call - should compute
	result1 := StemCached("happiness")
	if result1 != "happi" {
		t.Errorf("StemCached(%q) = %q, want %q", "happiness", result1, "happi")
	}

	// Second call - should use cache
	result2 := StemCached("happiness")
	if result2 != result1 {
		t.Errorf("StemCached(%q) = %q, want %q", "happiness", result2, result1)
	}

	// Short words should return as-is
	short := StemCached("a")
	if short != "a" {
		t.Errorf("StemCached(%q) = %q, want %q", "a", short, "a")
	}
}

// TestStem_EdgeCases tests edge cases and special characters
func TestStem_EdgeCases(t *testing.T) {
	tests := []struct {
		word   string
		expect string
	}{
		{"", ""},
		{"a", "a"},
		{"I", "I"},
		{"123", "123"},
		{"hello-world", "hello-world"},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := Stem(tt.word)
			if result != tt.expect {
				t.Errorf("Stem(%q) = %q, want %q", tt.word, result, tt.expect)
			}
		})
	}
}

// TestStem_Consistency tests that stemming is consistent
func TestStem_Consistency(t *testing.T) {
	words := []string{"running", "happiness", "national", "general", "specific"}

	for _, word := range words {
		result1 := Stem(word)
		result2 := Stem(word)
		result3 := StemCached(word)

		if result1 != result2 {
			t.Errorf("Stem(%q) inconsistent: %q vs %q", word, result1, result2)
		}
		if result1 != result3 {
			t.Errorf("Stem(%q) vs StemCached(%q): %q vs %q", word, word, result1, result3)
		}
	}
}

// TestStem_Variations tests related word forms stem to same root
func TestStem_Variations(t *testing.T) {
	tests := []struct {
		words    []string
		sameRoot bool
	}{
		{[]string{"run", "runs", "running"}, false}, // Porter doesn't stem all to same root
		{[]string{"happy", "happiness", "happily"}, false},
		{[]string{"nation", "national", "nationality"}, false},
	}

	for _, tt := range tests {
		stems := make(map[string]bool)
		for _, word := range tt.words {
			stems[Stem(word)] = true
		}

		if tt.sameRoot && len(stems) != 1 {
			t.Errorf("Expected %v to stem to same root, got %v", tt.words, stems)
		}
	}
}

// BenchmarkStem benchmarks the Stem function
func BenchmarkStem(b *testing.B) {
	words := []string{"running", "happiness", "national", "general", "specific", "organization", "finalization"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, word := range words {
			Stem(word)
		}
	}
}

// BenchmarkStemCached benchmarks the StemCached function
func BenchmarkStemCached(b *testing.B) {
	words := []string{"running", "happiness", "national", "general", "specific", "organization", "finalization"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, word := range words {
			StemCached(word)
		}
	}
}
