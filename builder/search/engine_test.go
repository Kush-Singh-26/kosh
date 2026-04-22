package search

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/core"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
)


func TestPerformSearch_PhraseBoost(t *testing.T) {
	indexedPosts := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			Record: searchpkg.ContentRecord{Title: "Exact Phrase", Link: "/0", Content: "Learn go programming."},
			PositionalIndex: map[string][]uint32{"go": {1}, "programming": {2}},
			DocLen: 4,
		},
		{
			DenseID: 1,
			Record: searchpkg.ContentRecord{Title: "Separated Words", Link: "/1", Content: "Go is great programming."},
			PositionalIndex: map[string][]uint32{"go": {0}, "programming": {4}},
			DocLen: 6,
		},
	}

	idx := index.Build(indexedPosts)
	results := PerformSearch(idx, "go programming")

	if len(results) < 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	if results[0].ID != 0 {
		t.Errorf("Expected Document 0 first, got %d", results[0].ID)
	}
}

func TestExtractSnippet_Density(t *testing.T) {
	content := "filler cluster: cat dog bird. filler."
	terms := []string{"cat", "dog", "bird"}
	snippet := ExtractSnippet(content, terms)
	if !strings.Contains(snippet, "<b>cat</b> <b>dog</b> <b>bird</b>") {
		t.Errorf("Snippet did not highlight cluster: %s", snippet)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"simple", "Hello world", []string{"Hello", "world"}},
		{"punctuation", "Hello, world!", []string{"Hello", "world"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := core.TokenizeWithUnicodeInto(tt.input, nil)
			got := make([]string, len(tokens))
			for i, token := range tokens {
				got[i] = token.Value
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPerformSearch_Tags(t *testing.T) {
	indexedPosts := []searchpkg.IndexedContent{
		{
			DenseID: 0,
			Record: searchpkg.ContentRecord{Title: "Go", Link: "/go", NormalizedTaxs: map[string][]string{"tags": {"go"}}},
			PositionalIndex: map[string][]uint32{"go": {1}},
		},
		{
			DenseID: 1,
			Record: searchpkg.ContentRecord{Title: "Rust", Link: "/rust", NormalizedTaxs: map[string][]string{"tags": {"rust"}}},
			PositionalIndex: map[string][]uint32{"rust": {1}},
		},
	}
	idx := index.Build(indexedPosts)
	
	results := PerformSearch(idx, "tag:rust")
	if len(results) != 1 || results[0].ID != 1 {
		t.Errorf("Tag search failed, got %v", results)
	}
}

func TestExtractSnippet_Fallback(t *testing.T) {
	content := "The quick brown fox jumps over the lazy dog."
	// Should return start of content if no matches
	snippet := ExtractSnippet(content, []string{"missing"})
	if !strings.Contains(snippet, "The quick brown") {
		t.Errorf("Fallback snippet failed: %s", snippet)
	}
}
