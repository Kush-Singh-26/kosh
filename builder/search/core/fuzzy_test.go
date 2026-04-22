package core

import (
	"sort"
	"testing"
)

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"café", "cafe", 1},
	}
	for _, tt := range tests {
		result := LevenshteinDistance(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestFuzzyExpand(t *testing.T) {
	lexicon := []string{"best", "rest", "tes", "test", "testing", "text"}
	sort.Strings(lexicon)

	tests := []struct {
		term     string
		maxDist  int
		minCount int
	}{
		{"test", 2, 4},
		{"test", 1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.term, func(t *testing.T) {
			result := FuzzyExpand(tt.term, lexicon, tt.maxDist)
			if len(result) < tt.minCount {
				t.Errorf("FuzzyExpand(%q) returned %d candidates, want at least %d", tt.term, len(result), tt.minCount)
			}
		})
	}
}

func TestBuildNgramIndex(t *testing.T) {
	lexicon := []string{"best", "test", "testing"}
	result := BuildNgramIndex(lexicon)
	if _, ok := result["tes"]; !ok {
		t.Error("Expected 'tes' trigram")
	}
}

func TestFuzzyMatch(t *testing.T) {
	if !FuzzyMatch("test", "text", 1) {
		t.Error("Expected fuzzy match")
	}
	if FuzzyMatch("test", "testing", 1) {
		t.Error("Expected no match due to length difference")
	}
}

func TestGenerateTrigrams(t *testing.T) {
	res := GenerateTrigrams("hello")
	expected := []string{"hel", "ell", "llo"}
	if len(res) != len(expected) {
		t.Errorf("Got %v, want %v", res, expected)
	}
}

func TestParseQuery(t *testing.T) {
	q := ParseQuery("hello \"world phrase\" -exclude")
	if len(q.Terms) != 2 || len(q.Phrases) != 1 {
		t.Errorf("Parsing failed: %+v", q)
	}
	foundExclude := false
	for _, info := range q.TermInfos {
		if info.Excluded && info.Term == "exclud" {
			foundExclude = true
		}
	}
	if !foundExclude {
		t.Error("Excluded term not found")
	}
}
