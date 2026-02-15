package search

import (
	"strings"
	"testing"
)

func TestStemmer(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"running"},
		{"runs"},
		{"ran"},
		{"easily"},
		{"processing"},
		{"processed"},
		{"processes"},
		{"caresses"},
		{"ponies"},
		{"caress"},
		{"cats"},
		{"feed"},
		{"agreed"},
		{"plastered"},
		{"bled"},
		{"motoring"},
		{"sing"},
		{"conflated"},
		{"troubled"},
		{"sized"},
		{"hopping"},
		{"tanned"},
		{"falling"},
		{"hissing"},
		{"fizzed"},
		{"failing"},
		{"filing"},
		{"happy"},
		{"sky"},
		{"relational"},
		{"conditional"},
		{"rational"},
		{"valenci"},
		{"hesitanci"},
		{"digitizer"},
		{"conformabli"},
		{"radicalli"},
		{"differentli"},
		{"vileli"},
		{"analogousli"},
		{"vietnamization"},
		{"predication"},
		{"operator"},
		{"feudalism"},
		{"decisiveness"},
		{"hopefulness"},
		{"callousness"},
		{"formaliti"},
		{"sensitiviti"},
		{"sensibiliti"},
		{"triplicate"},
		{"formative"},
		{"formalize"},
		{"electriciti"},
		{"electrical"},
		{"hopeful"},
		{"goodness"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Stem(tt.input)
			if len(result) == 0 {
				t.Errorf("Stem(%q) returned empty string", tt.input)
			}
			if len(result) > len(tt.input) {
				t.Errorf("Stem(%q) = %q, result longer than input", tt.input, result)
			}
		})
	}
}

func TestStemmerConsistency(t *testing.T) {
	wordGroups := [][]string{
		{"compute", "computes", "computing"},
		{"network", "networks", "networking"},
		{"transform", "transforms", "transforming"},
	}

	for _, group := range wordGroups {
		stems := make(map[string]bool)
		for _, word := range group {
			stems[Stem(word)] = true
		}
		if len(stems) > 2 {
			t.Errorf("Words %v produced too many different stems: %v", group, stems)
		}
	}
}

func TestAnalyzer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "stop words removed",
			input:    "the quick brown fox",
			contains: []string{"quick", "brown", "fox"},
			excludes: []string{"the"},
		},
		{
			name:     "short words filtered",
			input:    "a an is it the transformer",
			contains: []string{"transform"},
			excludes: []string{"a", "an", "is", "it", "the"},
		},
		{
			name:     "mixed case normalized",
			input:    "Machine Learning Neural Networks",
			contains: []string{"machin", "learn", "neural", "network"},
			excludes: []string{},
		},
		{
			name:     "stemming works",
			input:    "running quickly",
			contains: []string{"run", "quick"},
			excludes: []string{"the"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultAnalyzer.Analyze(tt.input)

			for _, want := range tt.contains {
				found := false
				for _, got := range result {
					if strings.Contains(got, want) || got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected %q (or stem) in result, got %v", want, result)
				}
			}

			for _, notWant := range tt.excludes {
				for _, got := range result {
					if got == notWant {
						t.Errorf("Did not expect %q in result", notWant)
					}
				}
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"go", "go", 0},
		{"", "test", 4},
		{"test", "", 4},
		{"abc", "abc", 0},
		{"abc", "abx", 1},
		{"transformer", "transfomer", 1},
		{"neural", "nural", 1},
		{"machine", "machne", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			result := LevenshteinDistance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		term, target string
		maxDist      int
		expected     bool
	}{
		{"transformer", "transformer", 0, true},
		{"transformer", "transfomer", 1, true},
		{"transformer", "transfrmer", 1, true},
		{"transformer", "tansformer", 1, true},
		{"neural", "nural", 1, true},
		{"machine", "machne", 1, true},
		{"machine", "macine", 1, true},
		{"machine", "machin", 1, true},
		{"abc", "xyz", 2, false},
		{"transformer", "transform", 2, true},
		{"transform", "transformer", 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.term+"_"+tt.target, func(t *testing.T) {
			result := FuzzyMatch(tt.term, tt.target, tt.maxDist)
			if result != tt.expected {
				t.Errorf("FuzzyMatch(%q, %q, %d) = %v, want %v", tt.term, tt.target, tt.maxDist, result, tt.expected)
			}
		})
	}
}

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantTerms   []string
		wantPhrases []string
	}{
		{
			name:        "simple terms",
			query:       "machine learning",
			wantTerms:   []string{"machin", "learn"},
			wantPhrases: nil,
		},
		{
			name:        "phrase only",
			query:       `"machine learning"`,
			wantTerms:   nil,
			wantPhrases: []string{"machine learning"},
		},
		{
			name:        "mixed terms and phrase",
			query:       `neural "deep learning" networks`,
			wantPhrases: []string{"deep learning"},
		},
		{
			name:        "multiple phrases",
			query:       `"machine learning" and "neural networks"`,
			wantPhrases: []string{"machine learning", "neural networks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseQuery(tt.query)

			for _, want := range tt.wantTerms {
				found := false
				for _, got := range result.Terms {
					if strings.Contains(got, want) || got == want {
						found = true
						break
					}
				}
				if !found && want != "" {
					t.Errorf("Expected term containing %q in result, got %v", want, result.Terms)
				}
			}

			for _, want := range tt.wantPhrases {
				found := false
				for _, got := range result.Phrases {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected phrase %q in result, got %v", want, result.Phrases)
				}
			}
		})
	}
}

func TestTrigramGeneration(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"abc", []string{"abc"}},
		{"abcd", []string{"abc", "bcd"}},
		{"transformer", []string{"tra", "ran", "ans", "nsf", "sfo", "for", "orm", "rme", "mer"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := generateTrigrams(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("generateTrigrams(%q) returned %d trigrams, want %d", tt.input, len(result), len(tt.expected))
				return
			}
			for i, exp := range tt.expected {
				if result[i] != exp {
					t.Errorf("generateTrigrams(%q)[%d] = %q, want %q", tt.input, i, result[i], exp)
				}
			}
		})
	}
}

func TestStopWords(t *testing.T) {
	stopWordList := []string{"the", "a", "an", "and", "or", "but", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "will", "would", "could", "should", "may", "might", "must", "shall", "can", "need", "dare", "ought", "used", "to", "of", "in", "for", "on", "with", "at", "by", "from", "as", "into", "through", "during", "before", "after", "above", "below", "between", "under", "again", "further", "then", "once", "here", "there", "when", "where", "why", "how", "all", "each", "few", "more", "most", "other", "some", "such", "no", "nor", "not", "only", "own", "same", "so", "than", "too", "very", "just", "also"}

	for _, word := range stopWordList {
		if !IsStopWord(word) {
			t.Errorf("IsStopWord(%q) = false, want true", word)
		}
	}

	nonStopWords := []string{"machine", "learning", "transformer", "neural", "network", "optimization"}
	for _, word := range nonStopWords {
		if IsStopWord(word) {
			t.Errorf("IsStopWord(%q) = true, want false", word)
		}
	}
}
