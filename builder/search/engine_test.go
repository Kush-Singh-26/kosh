package search

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple sentence",
			input:    "Hello world",
			expected: []string{"Hello", "world"},
		},
		{
			name:     "punctuation",
			input:    "Hello, world!",
			expected: []string{"Hello", "world"},
		},
		{
			name:     "numbers",
			input:    "Testing 123",
			expected: []string{"Testing", "123"},
		},
		{
			name:     "extra spaces",
			input:    "  Hello   world  ",
			expected: []string{"Hello", "world"},
		},
		{
			name:     "special characters",
			input:    "go-lang is awesome (really)",
			expected: []string{"go", "lang", "is", "awesome", "really"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPerformSearch(t *testing.T) {
	// Setup test index
	posts := []models.PostRecord{
		{
			ID:              0,
			Title:           "Go Guide",
			NormalizedTitle: "go guide",
			Description:     "A guide to Go programming language",
			Version:         "v1",
			NormalizedTags:  []string{"go", "programming"},
		},
		{
			ID:              1,
			Title:           "Rust Guide",
			NormalizedTitle: "rust guide",
			Description:     "A guide to Rust programming",
			Version:         "v1",
			NormalizedTags:  []string{"rust", "programming"},
		},
		{
			ID:              2,
			Title:           "Python Intro",
			NormalizedTitle: "python intro",
			Description:     "Introduction to Python",
			Version:         "v2",
			NormalizedTags:  []string{"python"},
		},
	}

	index := &models.SearchIndex{
		Posts:     posts,
		Inverted:  make(map[string]map[string][]int),
		DocLens:   make(map[string]int64),
		TotalDocs: 3,
		AvgDocLen: 5.0,
	}

	// Helper to populate inverted index
	addTerm := func(term string, postID int, pos int) {
		idStr := strconv.Itoa(postID)
		if index.Inverted[term] == nil {
			index.Inverted[term] = make(map[string][]int)
		}
		index.Inverted[term][idStr] = append(index.Inverted[term][idStr], pos)
	}

	// "guide" appears in 0 and 1
	addTerm("guide", 0, 1)
	addTerm("guide", 1, 1)
	// "programming" appears in 0 and 1
	addTerm("programming", 0, 3)
	addTerm("programming", 1, 3)
	// "go" appears in 0
	addTerm("go", 0, 2)
	// "python" appears in 2
	addTerm("python", 2, 2)

	index.DocLens["0"] = 6
	index.DocLens["1"] = 5
	index.DocLens["2"] = 3

	tests := []struct {
		name          string
		query         string
		versionFilter string
		wantIDs       []int
	}{
		{
			name:          "search go",
			query:         "go",
			versionFilter: "all",
			wantIDs:       []int{0},
		},
		{
			name:          "search guide",
			query:         "guide",
			versionFilter: "all",
			wantIDs:       []int{0, 1}, // Both match
		},
		{
			name:          "version filter",
			query:         "guide",
			versionFilter: "v1",
			wantIDs:       []int{0, 1},
		},
		{
			name:          "version filter mismatch",
			query:         "python",
			versionFilter: "v1",
			wantIDs:       nil, // Python is v2
		},
		{
			name:          "tag search",
			query:         "tag:rust",
			versionFilter: "all",
			wantIDs:       []int{1},
		},
		{
			name:          "tag search mismatch",
			query:         "tag:java",
			versionFilter: "all",
			wantIDs:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := PerformSearch(index, tt.query, tt.versionFilter)

			// Extract IDs
			var gotIDs []int
			for _, r := range results {
				gotIDs = append(gotIDs, r.ID)
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("PerformSearch() returned %d results, want %d", len(gotIDs), len(tt.wantIDs))
			}

			// Simple check for single result cases
			if len(tt.wantIDs) == 1 && len(gotIDs) == 1 {
				if gotIDs[0] != tt.wantIDs[0] {
					t.Errorf("PerformSearch() got ID %d, want %d", gotIDs[0], tt.wantIDs[0])
				}
			}
		})
	}
}

func TestPhraseAdjacency(t *testing.T) {
	index := &models.SearchIndex{
		Inverted: map[string]map[string][]int{
			"quick": {"0": {1}},
			"brown": {"0": {2}},
			"fox":   {"0": {3}},
			"jump":  {"0": {5}},
		},
	}

	tests := []struct {
		phrase []string
		want   bool
	}{
		{[]string{"quick", "brown"}, true},
		{[]string{"quick", "brown", "fox"}, true},
		{[]string{"brown", "quick"}, false},
		{[]string{"fox", "jump"}, false}, // gap at pos 4
		{[]string{"quick"}, true},
	}

	for _, tt := range tests {
		got := checkPhraseUnified(index, 0, tt.phrase)
		if got != tt.want {
			t.Errorf("checkPhraseUnified(%v) = %v, want %v", tt.phrase, got, tt.want)
		}
	}
}

func TestExtractSnippet(t *testing.T) {
	content := "The quick brown fox jumps over the lazy dog. It was a sunny day."

	tests := []struct {
		name     string
		terms    []string
		contains []string
	}{
		{
			name:     "found term",
			terms:    []string{"fox"},
			contains: []string{"<b>fox</b>"},
		},
		{
			name:     "multiple terms",
			terms:    []string{"quick", "dog"},
			contains: []string{"<b>quick</b>", "<b>dog</b>"},
		},
		{
			name:     "no terms",
			terms:    []string{},
			contains: []string{"The quick brown"},
		},
		{
			name:     "term not found",
			terms:    []string{"cat"},
			contains: []string{"The quick brown"}, // Returns start of content
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSnippet(content, tt.terms)
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("ExtractSnippet() result %q does not contain %q", got, c)
				}
			}
		})
	}
}

func TestHasTagNormalized(t *testing.T) {
	tags := []string{"go", "web-dev", "ssg"}

	if !HasTagNormalized(tags, "go") {
		t.Error("HasTagNormalized should find existing tag")
	}

	if HasTagNormalized(tags, "rust") {
		t.Error("HasTagNormalized should not find missing tag")
	}

	if HasTagNormalized(tags, "GO") {
		t.Error("HasTagNormalized should be case sensitive (it expects pre-normalized input)")
	}
}
