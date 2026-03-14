package generators

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/tinylib/msgp/msgp"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
)

func TestGenerateSearchIndex(t *testing.T) {
	sink := mocks.NewMemSink()
	outputDir := "public"

	indexedPosts := []models.IndexedPost{
		{
			Record: models.PostRecord{
				ID:      1,
				Title:   "Post 1",
				Link:    "/post1.html",
				Content: "Body of post 1",
			},
			DocLen: 4,
			PositionalIndex: map[string][]int{
				"body": {0},
				"post": {2},
			},
			ByteOffsets: map[string][]int{
				"body": {0, 4},
				"post": {8, 12},
			},
			StemMap: map[string]string{
				"body": "body",
				"post": "post",
			},
		},
	}

	resultPath, err := GenerateSearchIndex(sink, outputDir, indexedPosts)
	if err != nil {
		t.Fatalf("GenerateSearchIndex failed: %v", err)
	}

	expectedPath := filepath.ToSlash(filepath.Join(outputDir, "search.bin"))
	if resultPath != expectedPath {
		t.Errorf("Expected result path %s, got %s", expectedPath, resultPath)
	}

	content, ok := sink.Files[expectedPath]
	if !ok {
		t.Fatalf("Search index file not found in sink at %s", expectedPath)
	}

	// Verify decoding
	var index models.SearchIndex
	br := brotli.NewReader(bytes.NewReader(content))
	mr := msgp.NewReader(br)
	if err := index.DecodeMsg(mr); err != nil {
		t.Fatalf("Failed to decode search index: %v", err)
	}

	if index.SchemaVersion != models.CurrentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", models.CurrentSchemaVersion, index.SchemaVersion)
	}

	if index.TotalDocs != 1 {
		t.Errorf("Expected 1 doc, got %d", index.TotalDocs)
	}

	// Verify post record
	post, ok := index.Posts["1"]
	if !ok {
		t.Fatal("Post 1 record missing in index")
	}
	if post.Title != "Post 1" {
		t.Errorf("Expected post title Post 1, got %s", post.Title)
	}

	// Verify inverted index
	if _, ok := index.Inverted["body"]; !ok {
		t.Error("Inverted entry for 'body' missing")
	}

	// Verify N-gram index
	if len(index.NgramIndex) == 0 {
		t.Error("Expected N-gram index to be generated")
	}
}

func TestGenerateSearchIndex_Empty(t *testing.T) {
	sink := mocks.NewMemSink()
	_, err := GenerateSearchIndex(sink, "public", []models.IndexedPost{})
	if err != nil {
		t.Fatalf("GenerateSearchIndex failed with empty posts: %v", err)
	}

	content := sink.Files["public/search.bin"]
	if len(content) == 0 {
		t.Fatal("Expected search.bin to be written even for empty input")
	}

	var index models.SearchIndex
	br := brotli.NewReader(bytes.NewReader(content))
	mr := msgp.NewReader(br)
	if err := index.DecodeMsg(mr); err != nil {
		t.Fatalf("Failed to decode empty search index: %v", err)
	}

	if index.TotalDocs != 0 {
		t.Errorf("Expected 0 total docs, got %d", index.TotalDocs)
	}
}

func TestGenerateSearchIndex_Nil(t *testing.T) {
	sink := mocks.NewMemSink()
	_, err := GenerateSearchIndex(sink, "public", nil)
	if err != nil {
		t.Fatalf("GenerateSearchIndex failed with nil posts: %v", err)
	}

	if _, ok := sink.Files["public/search.bin"]; !ok {
		t.Error("Expected search.bin to be written for nil input")
	}
}
