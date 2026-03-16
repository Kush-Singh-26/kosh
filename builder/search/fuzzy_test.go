package search

import (
	"testing"
)

func TestLevenshteinDistance_Identical(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"hello", "hello", 0},
		{"test", "test", 0},
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

func TestLevenshteinDistance_Empty(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "abc", 3},
		{"abc", "", 3},
		{"", "hello", 5},
		{"test", "", 4},
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

func TestLevenshteinDistance_SingleEdit(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"abc", "ab", 1},     // deletion
		{"ab", "abc", 1},     // insertion
		{"abc", "adc", 1},    // substitution
		{"cat", "bat", 1},    // substitution
		{"dog", "do", 1},     // deletion
		{"test", "tes", 1},   // deletion
		{"test", "testt", 1}, // insertion
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

func TestLevenshteinDistance_MultipleEdits(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"intention", "execution", 5},
		{"algorithm", "altruistic", 6},
		{"hello", "hallo", 1},
		{"hello", "helo", 1},
		{"hello", "heilo", 1},
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

func TestLevenshteinDistance_Unicode(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"café", "cafe", 1},
		{"naïve", "naive", 1},
		{"日本", "日本", 0},
		{"日本", "中国", 2},
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

func TestFuzzyMatchExtended(t *testing.T) {
	tests := []struct {
		term     string
		target   string
		maxDist  int
		expected bool
	}{
		{"test", "test", 2, true},
		{"test", "tes", 1, true},
		{"test", "tet", 1, true},
		{"test", "text", 1, true},
		{"test", "best", 1, true},
		{"test", "rest", 1, true},
		{"test", "testing", 2, false}, // length diff > maxDist
		{"prog", "program", 2, false}, // length diff > maxDist
		{"cat", "bat", 1, true},
		{"cat", "rat", 1, true},
		{"hello", "hallo", 1, true},
		{"hello", "helo", 1, true},
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

func TestGenerateTrigrams(t *testing.T) {
	tests := []struct {
		word     string
		expected []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"ab", []string{"ab"}},
		{"abc", []string{"abc"}},
		{"abcd", []string{"abc", "bcd"}},
		{"hello", []string{"hel", "ell", "llo"}},
		{"test", []string{"tes", "est"}},
		{"program", []string{"pro", "rog", "ogr", "gra", "ram"}},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := generateTrigrams(tt.word)
			if len(result) != len(tt.expected) {
				t.Errorf("generateTrigrams(%q) returned %d trigrams, want %d", tt.word, len(result), len(tt.expected))
			}
			for i, exp := range tt.expected {
				if i >= len(result) || result[i] != exp {
					t.Errorf("generateTrigrams(%q)[%d] = %q, want %q", tt.word, i, result[i], exp)
				}
			}
		})
	}
}

func TestGenerateTrigrams_Unicode(t *testing.T) {
	tests := []struct {
		word     string
		expected []string
	}{
		{"café", []string{"caf", "afé"}},
		{"日本", []string{"日本"}},
		{"日本語", []string{"日本語"}},
		{"日本語英", []string{"日本語", "本語英"}},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := generateTrigrams(tt.word)
			if len(result) != len(tt.expected) {
				t.Errorf("generateTrigrams(%q) returned %d trigrams, want %d", tt.word, len(result), len(tt.expected))
			}
			for i, exp := range tt.expected {
				if i >= len(result) || result[i] != exp {
					t.Errorf("generateTrigrams(%q)[%d] = %q, want %q", tt.word, i, result[i], exp)
				}
			}
		})
	}
}

func TestFuzzyExpand(t *testing.T) {
	inverted := map[string]map[string][]int{
		"test":    {"doc1": {1}},
		"testing": {"doc2": {2}},
		"tes":     {"doc3": {3}},
		"best":    {"doc4": {4}},
		"rest":    {"doc5": {5}},
		"text":    {"doc6": {6}},
	}

	tests := []struct {
		term     string
		maxDist  int
		minCount int // minimum expected candidates
	}{
		{"test", 2, 4},
		{"test", 1, 2},
		{"tes", 1, 2},
		{"best", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.term, func(t *testing.T) {
			result := FuzzyExpand(tt.term, inverted, tt.maxDist)
			if len(result) < tt.minCount {
				t.Errorf("FuzzyExpand(%q) returned %d candidates, want at least %d", tt.term, len(result), tt.minCount)
			}
		})
	}
}

func TestFuzzyExpandWithNgrams(t *testing.T) {
	ngramIndex := map[string][]string{
		"tes": {"test", "testing", "best"},
		"est": {"test", "testing", "rest"},
		"sti": {"testing"},
		"tin": {"testing"},
		"ing": {"testing"},
	}

	tests := []struct {
		term     string
		maxDist  int
		minCount int
	}{
		{"test", 2, 1},
		{"testing", 2, 1},
		{"best", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.term, func(t *testing.T) {
			result := FuzzyExpandWithNgrams(tt.term, ngramIndex, tt.maxDist)
			if len(result) < tt.minCount {
				t.Errorf("FuzzyExpandWithNgrams(%q) returned %d candidates, want at least %d", tt.term, len(result), tt.minCount)
			}
		})
	}
}

func TestBuildNgramIndex(t *testing.T) {
	inverted := map[string]map[string][]int{
		"test":    {"doc1": {1}},
		"testing": {"doc2": {2}},
		"best":    {"doc3": {3}},
	}

	result := BuildNgramIndex(inverted)

	// Verify trigrams are indexed
	if _, ok := result["tes"]; !ok {
		t.Error("Expected 'tes' trigram in index")
	}
	if _, ok := result["est"]; !ok {
		t.Error("Expected 'est' trigram in index")
	}
	if _, ok := result["ing"]; !ok {
		t.Error("Expected 'ing' trigram in index")
	}
}

func TestMin3(t *testing.T) {
	tests := []struct {
		a, b, c  int
		expected int
	}{
		{1, 2, 3, 1},
		{3, 2, 1, 1},
		{2, 1, 3, 1},
		{5, 5, 5, 5},
		{0, 1, 2, 0},
		{-1, 0, 1, -1},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.a))+string(rune(tt.b))+string(rune(tt.c)), func(t *testing.T) {
			result := min3(tt.a, tt.b, tt.c)
			if result != tt.expected {
				t.Errorf("min3(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, result, tt.expected)
			}
		})
	}
}

func TestParsedQueryExtended(t *testing.T) {
	tests := []struct {
		query     string
		wantTerms int
		wantPhrases int
	}{
		{"hello world", 2, 0},
		{"\"machine learning\"", 0, 1},
		{"hello \"machine learning\" world", 2, 1},
		{"", 0, 0},
		{"test", 1, 0},
		{"\"test phrase\" another", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := ParseQuery(tt.query)
			if len(result.Terms) != tt.wantTerms {
				t.Errorf("ParseQuery(%q) Terms = %d, want %d", tt.query, len(result.Terms), tt.wantTerms)
			}
			if len(result.Phrases) != tt.wantPhrases {
				t.Errorf("ParseQuery(%q) Phrases = %d, want %d", tt.query, len(result.Phrases), tt.wantPhrases)
			}
			if result.Raw != tt.query {
				t.Errorf("ParseQuery(%q) Raw = %q, want %q", tt.query, result.Raw, tt.query)
			}
		})
	}
}
