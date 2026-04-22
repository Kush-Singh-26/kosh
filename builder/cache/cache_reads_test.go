package cache

import (
	"errors"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
)

func TestGetItemByPath(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create a post
	post := createSamplePostMeta()
	post.Path = "content/posts/my-post.md"

	if err := m.BatchCommit([]*core.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve by path
	retrieved, err := m.GetItemByPath("content/posts/my-post.md")
	if err != nil {
		t.Fatalf("GetItemByPath failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetItemByPath should return the post")
	}

	if retrieved.ContentID != post.ContentID {
		t.Errorf("ContentID = %q, want %q", retrieved.ContentID, post.ContentID)
	}
}

func TestGetItemByPath_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Try to get non-existent path
	retrieved, err := m.GetItemByPath("content/posts/non-existent.md")
	if !errors.Is(err, core.ErrNoContent) {
		t.Fatalf("Expected core.ErrNoContent, got %v", err)
	}

	if retrieved != nil {
		t.Error("GetItemByPath should return nil for non-existent path")
	}
}

func TestGetItemByID(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create a post
	post := createSamplePostMeta()
	post.ContentID = "my-unique-post-id"

	if err := m.BatchCommit([]*core.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve by ID
	retrieved, err := m.GetItemByID("my-unique-post-id")
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetItemByID should return the post")
	}

	if retrieved.ContentID != "my-unique-post-id" {
		t.Errorf("ContentID = %q, want %q", retrieved.ContentID, "my-unique-post-id")
	}
}

func TestGetItemByID_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Try to get non-existent ID
	retrieved, err := m.GetItemByID("non-existent-id")
	if !errors.Is(err, core.ErrNoContent) {
		t.Fatalf("Expected core.ErrNoContent, got %v", err)
	}

	if retrieved != nil {
		t.Error("GetItemByID should return nil for non-existent ID")
	}
}

func TestGetItemsByIDs(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create posts
	post1 := createSamplePostMeta()
	post1.ContentID = "post-1"
	post2 := createSamplePostMeta()
	post2.ContentID = "post-2"
	post3 := createSamplePostMeta()
	post3.ContentID = "post-3"

	if err := m.BatchCommit([]*core.ContentMeta{post1, post2, post3}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve specific posts
	posts, err := m.GetItemsByIDs([]string{"post-1", "post-3", "non-existent"})
	if err != nil {
		t.Fatalf("GetItemsByIDs failed: %v", err)
	}

	if len(posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(posts))
	}

	if posts["post-1"] == nil {
		t.Error("Should have post-1")
	}

	if posts["post-3"] == nil {
		t.Error("Should have post-3")
	}
}

func TestGetItemsByIDs_Empty(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Empty list should return empty map
	posts, err := m.GetItemsByIDs([]string{})
	if err != nil {
		t.Fatalf("GetItemsByIDs failed: %v", err)
	}

	if posts == nil {
		t.Error("GetItemsByIDs should return empty map, not nil")
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts, got %d", len(posts))
	}
}

func TestGetSearchRecord(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create post with search record
	post := createSamplePostMeta()
	record := &core.SearchRecord{
		Title:          "Test Post",
		WordFreqs:      map[string]int{"test": 1, "post": 2},
		DocLen:         10,
		NormalizedTaxs: map[string][]string{"tags": {"test", "go"}},
	}

	records := map[string]*core.SearchRecord{
		post.ContentID: record,
	}

	if err := m.BatchCommit([]*core.ContentMeta{post}, records, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve search record
	retrieved, err := m.GetSearchRecord(post.ContentID)
	if err != nil {
		t.Fatalf("GetSearchRecord failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetSearchRecord should return the record")
	}

	if retrieved.Title != record.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, record.Title)
	}
}

func TestGetSearchRecord_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Try to get non-existent record
	retrieved, err := m.GetSearchRecord("non-existent")
	if !errors.Is(err, core.ErrNoContent) {
		t.Fatalf("Expected core.ErrNoContent, got %v", err)
	}

	if retrieved != nil {
		t.Error("GetSearchRecord should return nil for non-existent record")
	}
}

func TestGetSearchRecords(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create posts with search records
	post1 := createSamplePostMeta()
	post1.ContentID = "post-1"
	post2 := createSamplePostMeta()
	post2.ContentID = "post-2"

	record1 := &core.SearchRecord{Title: "Post 1"}
	record2 := &core.SearchRecord{Title: "Post 2"}

	records := map[string]*core.SearchRecord{
		"post-1": record1,
		"post-2": record2,
	}

	if err := m.BatchCommit([]*core.ContentMeta{post1, post2}, records, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve multiple records
	retrieved, err := m.GetSearchRecords([]string{"post-1", "post-2", "non-existent"})
	if err != nil {
		t.Fatalf("GetSearchRecords failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 records, got %d", len(retrieved))
	}

	if retrieved["post-1"] == nil {
		t.Error("Should have record for post-1")
	}

	if retrieved["post-2"] == nil {
		t.Error("Should have record for post-2")
	}
}

func TestGetHTMLContent_Inline(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create post with inline HTML
	post := createSamplePostMeta()
	inlineHTML := []byte("<p>Inline HTML content</p>")
	post.InlineHTML = inlineHTML
	post.HTMLHash = ""

	// Get HTML content
	content, err := m.GetHTMLContent(post)
	if err != nil {
		t.Fatalf("GetHTMLContent failed: %v", err)
	}

	if string(content) != string(inlineHTML) {
		t.Errorf("Content = %q, want %q", content, inlineHTML)
	}
}

func TestGetHTMLContent_Empty(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create post with no HTML
	post := createSamplePostMeta()
	post.InlineHTML = nil
	post.HTMLHash = ""

	// Get HTML content
	content, err := m.GetHTMLContent(post)
	if !errors.Is(err, core.ErrNoContent) {
		t.Fatalf("Expected core.ErrNoContent, got %v", err)
	}

	if content != nil {
		t.Error("GetHTMLContent should return nil for empty post")
	}
}

func TestGetPostsByTag(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create posts with tags
	post1 := createSamplePostMeta()
	post1.ContentID = "post-1"
	post1.Taxonomies = map[string][]string{"tags": {"go", "tutorial"}}

	post2 := createSamplePostMeta()
	post2.ContentID = "post-2"
	post2.Taxonomies = map[string][]string{"tags": {"go", "advanced"}}

	post3 := createSamplePostMeta()
	post3.ContentID = "post-3"
	post3.Taxonomies = map[string][]string{"tags": {"python", "tutorial"}}

	deps1 := &core.Dependencies{Taxonomies: post1.Taxonomies}
	deps2 := &core.Dependencies{Taxonomies: post2.Taxonomies}
	deps3 := &core.Dependencies{Taxonomies: post3.Taxonomies}

	depsMap := map[string]*core.Dependencies{
		"post-1": deps1,
		"post-2": deps2,
		"post-3": deps3,
	}

	if err := m.BatchCommit([]*core.ContentMeta{post1, post2, post3}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Get posts by tag
	posts, err := m.GetPostsByTaxonomy("tags", "go")
	if err != nil {
		t.Fatalf("GetPostsByTaxonomy failed: %v", err)
	}

	// Should find post-1 and post-2
	found := make(map[string]bool)
	for _, id := range posts {
		found[id] = true
	}

	if !found["post-1"] {
		t.Error("Should find post-1 with tag 'go'")
	}

	if !found["post-2"] {
		t.Error("Should find post-2 with tag 'go'")
	}
}

func TestGetPostsByTag_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Get posts by non-existent tag
	posts, err := m.GetPostsByTaxonomy("tags", "non-existent-tag")
	if err != nil {
		t.Fatalf("GetPostsByTaxonomy failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts, got %d", len(posts))
	}
}

func TestGetItemsByTemplate(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create posts with templates
	post1 := createSamplePostMeta()
	post1.ContentID = "post-1"

	post2 := createSamplePostMeta()
	post2.ContentID = "post-2"

	deps1 := &core.Dependencies{Templates: []string{"layouts/post.html", "partials/header.html"}}
	deps2 := &core.Dependencies{Templates: []string{"layouts/post.html"}}

	depsMap := map[string]*core.Dependencies{
		"post-1": deps1,
		"post-2": deps2,
	}

	if err := m.BatchCommit([]*core.ContentMeta{post1, post2}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Get posts by template
	posts, err := m.GetItemsByTemplate("layouts/post.html")
	if err != nil {
		t.Fatalf("GetItemsByTemplate failed: %v", err)
	}

	// Should find both posts
	if len(posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(posts))
	}
}

func TestGetCachedItem_Generic(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Test the generic getCachedItem function through GetItemByID
	post := createSamplePostMeta()
	post.ContentID = "generic-test"

	if err := m.BatchCommit([]*core.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// This internally uses getCachedItem
	retrieved, err := m.GetItemByID("generic-test")
	if err != nil {
		t.Fatalf("GetItemByID failed: %v", err)
	}

	if retrieved == nil {
		t.Error("Should retrieve the post")
	}
}
