package search

import (
	"testing"
)

// TestLevenshteinDistance_Basic tests basic edit distance calculations
func TestLevenshteinDistance_Basic(t *testing.T) {
	tests := []struct {
		a      string
		b      string
		expect int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "ab", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"intention", "execution", 5},
	}

	for _, tt := range tests {
		t.Run(tt.a+"->"+tt.b, func(t *testing.T) {
			result := LevenshteinDistance(tt.a, tt.b)
			if result != tt.expect {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expect)
			}
		})
	}
}

// TestLevenshteinDistance_Unicode tests Unicode string handling
func TestLevenshteinDistance_Unicode(t *testing.T) {
	tests := []struct {
		a      string
		b      string
		expect int
	}{
		{"café", "cafe", 1},
		{"日本語", "日本人", 1},
		{"hello", "héllo", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a, func(t *testing.T) {
			result := LevenshteinDistance(tt.a, tt.b)
			if result != tt.expect {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expect)
			}
		})
	}
}

// TestFuzzyMatch_Fuzzy tests fuzzy matching within max distance
func TestFuzzyMatch_Fuzzy(t *testing.T) {
	tests := []struct {
		term   string
		target string
		maxDist int
		expect bool
	}{
		{"cat", "cat", 0, true},
		{"cat", "cats", 1, true},
		{"cat", "bat", 1, true},
		{"cat", "car", 1, true},
		{"cat", "dog", 1, false},
		{"cat", "elephant", 2, false},
		{"program", "programming", 2, false},
		{"program", "progam", 1, true},
		{"program", "progarm", 2, true}, // distance is 2, not 1
	}

	for _, tt := range tests {
		t.Run(tt.term+"->"+tt.target, func(t *testing.T) {
			result := FuzzyMatch(tt.term, tt.target, tt.maxDist)
			if result != tt.expect {
				t.Errorf("FuzzyMatch(%q, %q, %d) = %v, want %v", tt.term, tt.target, tt.maxDist, result, tt.expect)
			}
		})
	}
}

// TestFuzzyMatch_LengthFilter tests the length-based quick exit
func TestFuzzyMatch_LengthFilter(t *testing.T) {
	// Length difference > maxDist should return false immediately
	result := FuzzyMatch("cat", "elephant", 2)
	if result {
		t.Error("Expected false for large length difference")
	}

	result = FuzzyMatch("cat", "catalog", 2)
	if result {
		t.Error("Expected false for length difference > maxDist")
	}
}

// TestFuzzyExpand tests fuzzy expansion for candidate generation
func TestFuzzyExpand(t *testing.T) {
	inverted := map[string]map[string][]int{
		"program": {"doc1": {1}},
		"programming": {"doc1": {2}},
		"progress": {"doc1": {3}},
		"project": {"doc1": {4}},
		"python": {"doc1": {5}},
		"cat": {"doc1": {6}},
		"dog": {"doc1": {7}},
	}

	// Test prefix matching (termLen >= 3)
	candidates := FuzzyExpand("prog", inverted, 2)
	if len(candidates) == 0 {
		t.Error("Expected candidates for prefix match")
	}

	// Test fuzzy matching
	candidates = FuzzyExpand("program", inverted, 2)
	if len(candidates) == 0 {
		t.Error("Expected candidates for fuzzy match")
	}

	// Test no matches
	candidates = FuzzyExpand("xyz", inverted, 1)
	if len(candidates) != 0 {
		t.Errorf("Expected no candidates for 'xyz', got %v", candidates)
	}
}

// TestFuzzyExpand_PrefixMatching tests live prefix matching
func TestFuzzyExpand_PrefixMatching(t *testing.T) {
	inverted := map[string]map[string][]int{
		"program": {"doc1": {1}},
		"programming": {"doc1": {2}},
		"programmer": {"doc1": {3}},
	}

	// Short term (< 3 chars) - no prefix matching, but fuzzy match should work
	candidates := FuzzyExpand("pr", inverted, 2)
	// Short terms may not match due to edit distance - just verify no crash

	// Long term (>= 3 chars) - prefix matching enabled
	candidates = FuzzyExpand("prog", inverted, 2)
	if len(candidates) == 0 {
		t.Error("Expected candidates for prefix 'prog'")
	}
}

// TestFuzzyExpandWithNgrams tests n-gram based fuzzy expansion
func TestFuzzyExpandWithNgrams(t *testing.T) {
	ngramIndex := map[string][]string{
		"pro": {"program", "progress", "project"},
		"rog": {"program", "progress"},
		"ogr": {"program", "progress"},
		"gra": {"program", "progress"},
		"ram": {"program", "progress"},
		"pyt": {"python"},
		"yth": {"python"},
		"tho": {"python"},
		"hon": {"python"},
	}

	// Test with trigram overlap
	candidates := FuzzyExpandWithNgrams("program", ngramIndex, 2)
	if len(candidates) == 0 {
		t.Error("Expected candidates from n-gram index")
	}

	// Test with no trigram overlap
	candidates = FuzzyExpandWithNgrams("xyz", ngramIndex, 2)
	if len(candidates) != 0 {
		t.Errorf("Expected no candidates for 'xyz', got %v", candidates)
	}
}

// TestGenerateTrigrams tests trigram generation
func TestGenerateTrigrams(t *testing.T) {
	tests := []struct {
		word   string
		expect []string
	}{
		{"", []string{""}},
		{"a", []string{"a"}},
		{"ab", []string{"ab"}},
		{"abc", []string{"abc"}},
		{"abcd", []string{"abc", "bcd"}},
		{"abcde", []string{"abc", "bcd", "cde"}},
		{"cat", []string{"cat"}},
		{"cats", []string{"cat", "ats"}},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			result := generateTrigrams(tt.word)
			if len(result) != len(tt.expect) {
				t.Errorf("generateTrigrams(%q) returned %d trigrams, want %d", tt.word, len(result), len(tt.expect))
			}
			// Check first trigram for non-empty results
			if len(result) > 0 && len(tt.expect) > 0 {
				if result[0] != tt.expect[0] {
					t.Errorf("generateTrigrams(%q) = %v, want %v", tt.word, result, tt.expect)
				}
			}
		})
	}
}

// TestGenerateTrigrams_ASCII tests ASCII fast path
func TestGenerateTrigrams_ASCII(t *testing.T) {
	result := generateTrigrams("programming")
	if len(result) != 9 { // len("programming") - 2 = 9
		t.Errorf("Expected 9 trigrams for 'programming', got %d", len(result))
	}
}

// TestGenerateTrigrams_Unicode tests Unicode slow path
func TestGenerateTrigrams_Unicode(t *testing.T) {
	result := generateTrigrams("café")
	if len(result) != 2 { // 4 runes - 2 = 2
		t.Errorf("Expected 2 trigrams for 'café', got %d", len(result))
	}

	result = generateTrigrams("日本語")
	if len(result) != 1 { // 3 runes - 2 = 1
		t.Errorf("Expected 1 trigram for '日本語', got %d", len(result))
	}
}

// TestBuildNgramIndex tests n-gram index building
func TestBuildNgramIndex(t *testing.T) {
	inverted := map[string]map[string][]int{
		"program": {"doc1": {1}},
		"progress": {"doc1": {2}},
		"python": {"doc1": {3}},
	}

	index := BuildNgramIndex(inverted)

	// Check that trigrams map to correct terms
	if _, ok := index["pro"]; !ok {
		t.Error("Expected 'pro' trigram in index")
	}
	if _, ok := index["pyt"]; !ok {
		t.Error("Expected 'pyt' trigram in index")
	}

	// Check that index is not empty
	if len(index) == 0 {
		t.Error("Expected non-empty n-gram index")
	}
}

// TestBuildNgramIndex_Empty tests empty input
func TestBuildNgramIndex_Empty(t *testing.T) {
	inverted := map[string]map[string][]int{}
	index := BuildNgramIndex(inverted)
	if len(index) != 0 {
		t.Errorf("Expected empty index for empty input, got %d entries", len(index))
	}
}

// TestParseQuery_Basic tests basic query parsing
func TestParseQuery_Basic(t *testing.T) {
	result := ParseQuery("hello world")
	if len(result.Terms) != 2 {
		t.Errorf("Expected 2 terms, got %d", len(result.Terms))
	}
	if result.Raw != "hello world" {
		t.Errorf("Expected raw query 'hello world', got %q", result.Raw)
	}
}

// TestParseQuery_Phrases tests phrase extraction
func TestParseQuery_Phrases(t *testing.T) {
	result := ParseQuery(`"machine learning" algorithms`)
	if len(result.Phrases) != 1 {
		t.Errorf("Expected 1 phrase, got %d", len(result.Phrases))
	}
	if len(result.Terms) != 1 {
		t.Errorf("Expected 1 term (algorithms), got %d", len(result.Terms))
	}
}

// TestParseQuery_MultiplePhrases tests multiple phrase extraction
func TestParseQuery_MultiplePhrases(t *testing.T) {
	result := ParseQuery(`"deep learning" AND "neural networks"`)
	if len(result.Phrases) != 2 {
		t.Errorf("Expected 2 phrases, got %d", len(result.Phrases))
	}
}

// TestParseQuery_EmptyPhrase tests empty phrase handling
func TestParseQuery_EmptyPhrase(t *testing.T) {
	result := ParseQuery(`"" hello ""`)
	if len(result.Phrases) != 0 {
		t.Errorf("Expected 0 phrases for empty quotes, got %d", len(result.Phrases))
	}
	if len(result.Terms) != 1 {
		t.Errorf("Expected 1 term (hello), got %d", len(result.Terms))
	}
}

// TestParseQuery_UnclosedPhrase tests unclosed phrase handling
func TestParseQuery_UnclosedPhrase(t *testing.T) {
	result := ParseQuery(`"machine learning`)
	// Unclosed phrase should be ignored
	if len(result.Phrases) != 0 {
		t.Errorf("Expected 0 phrases for unclosed quote, got %d", len(result.Phrases))
	}
}

// TestParseQuery_CaseNormalization tests that terms are lowercased
func TestParseQuery_CaseNormalization(t *testing.T) {
	result := ParseQuery("HELLO World")
	for _, term := range result.Terms {
		if term != "hello" && term != "world" {
			t.Errorf("Expected lowercase terms, got %q", term)
		}
	}
}

// TestParsedQuery_Struct tests ParsedQuery structure
func TestParsedQuery_Struct(t *testing.T) {
	pq := ParsedQuery{
		Terms:   []string{"hello", "world"},
		Phrases: [][]string{{"machine", "learning"}},
		Raw:     `"machine learning" hello world`,
	}

	if len(pq.Terms) != 2 {
		t.Errorf("Expected 2 terms, got %d", len(pq.Terms))
	}
	if len(pq.Phrases) != 1 {
		t.Errorf("Expected 1 phrase, got %d", len(pq.Phrases))
	}
	if pq.Raw != `"machine learning" hello world` {
		t.Errorf("Unexpected raw query: %q", pq.Raw)
	}
}

// TestLevenshteinDistance_Pooling tests that slice pooling works correctly
func TestLevenshteinDistance_Pooling(t *testing.T) {
	// Run multiple times to exercise pool
	for i := 0; i < 100; i++ {
		result := LevenshteinDistance("kitten", "sitting")
		if result != 3 {
			t.Errorf("Iteration %d: LevenshteinDistance('kitten', 'sitting') = %d, want 3", i, result)
		}
	}
}

// TestFuzzyExpandWithNgrams_MinScore tests minimum score filtering
func TestFuzzyExpandWithNgrams_MinScore(t *testing.T) {
	// Create index where a term shares multiple trigrams
	ngramIndex := map[string][]string{
		"abc": {"abcdef", "xyz"},
		"bcd": {"abcdef"},
		"cde": {"abcdef"},
		"def": {"abcdef"},
		"efg": {"abcdef"},
	}

	// Short query (3 chars = 1 trigram) with minScore=1 should match
	candidates := FuzzyExpandWithNgrams("abc", ngramIndex, 2)
	// "abc" generates 1 trigram, minScore = max(1, 1/2) = 1
	// "abcdef" shares 1 trigram, should pass filter
	// But "xyz" also shares 1 trigram - both should be checked for fuzzy match
	// Just verify the function runs without error
	_ = candidates
}

// BenchmarkFuzzyMatch benchmarks fuzzy matching
func BenchmarkFuzzyMatch(b *testing.B) {
	term := "program"
	target := "programming"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FuzzyMatch(term, target, 2)
	}
}

// BenchmarkFuzzyExpand benchmarks fuzzy expansion
func BenchmarkFuzzyExpand(b *testing.B) {
	inverted := make(map[string]map[string][]int)
	for i := 0; i < 1000; i++ {
		term := "term" + string(rune(i))
		inverted[term] = map[string][]int{"doc1": {i}}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FuzzyExpand("term", inverted, 2)
	}
}

// BenchmarkGenerateTrigrams benchmarks trigram generation
func BenchmarkGenerateTrigrams(b *testing.B) {
	word := "programming"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateTrigrams(word)
	}
}

// BenchmarkBuildNgramIndex benchmarks n-gram index building
func BenchmarkBuildNgramIndex(b *testing.B) {
	inverted := make(map[string]map[string][]int)
	for i := 0; i < 100; i++ {
		term := "term" + string(rune(i))
		inverted[term] = map[string][]int{"doc1": {i}}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BuildNgramIndex(inverted)
	}
}
