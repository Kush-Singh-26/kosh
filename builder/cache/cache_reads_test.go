package cache

import (
	"testing"
)

func TestGetPostByPath(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create a post
	post := createSamplePostMeta()
	post.Path = "content/posts/my-post.md"

	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve by path
	retrieved, err := m.GetPostByPath("content/posts/my-post.md")
	if err != nil {
		t.Fatalf("GetPostByPath failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPostByPath should return the post")
	}

	if retrieved.PostID != post.PostID {
		t.Errorf("PostID = %q, want %q", retrieved.PostID, post.PostID)
	}
}

func TestGetPostByPath_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Try to get non-existent path
	retrieved, err := m.GetPostByPath("content/posts/non-existent.md")
	if err != nil {
		t.Fatalf("GetPostByPath should not error: %v", err)
	}

	if retrieved != nil {
		t.Error("GetPostByPath should return nil for non-existent path")
	}
}

func TestGetPostByID(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create a post
	post := createSamplePostMeta()
	post.PostID = "my-unique-post-id"

	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve by ID
	retrieved, err := m.GetPostByID("my-unique-post-id")
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPostByID should return the post")
	}

	if retrieved.PostID != "my-unique-post-id" {
		t.Errorf("PostID = %q, want %q", retrieved.PostID, "my-unique-post-id")
	}
}

func TestGetPostByID_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Try to get non-existent ID
	retrieved, err := m.GetPostByID("non-existent-id")
	if err != nil {
		t.Fatalf("GetPostByID should not error: %v", err)
	}

	if retrieved != nil {
		t.Error("GetPostByID should return nil for non-existent ID")
	}
}

func TestGetPostsByIDs(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create posts
	post1 := createSamplePostMeta()
	post1.PostID = "post-1"
	post2 := createSamplePostMeta()
	post2.PostID = "post-2"
	post3 := createSamplePostMeta()
	post3.PostID = "post-3"

	if err := m.BatchCommit([]*PostMeta{post1, post2, post3}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve specific posts
	posts, err := m.GetPostsByIDs([]string{"post-1", "post-3", "non-existent"})
	if err != nil {
		t.Fatalf("GetPostsByIDs failed: %v", err)
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

func TestGetPostsByIDs_Empty(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Empty list should return empty map
	posts, err := m.GetPostsByIDs([]string{})
	if err != nil {
		t.Fatalf("GetPostsByIDs failed: %v", err)
	}

	if posts == nil {
		t.Error("GetPostsByIDs should return empty map, not nil")
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
	record := &SearchRecord{
		Title:           "Test Post",
		NormalizedTitle: "test post",
		Tokens:          []string{"test", "post"},
		BM25Data:        map[string]int{"test": 1, "post": 2},
		DocLen:          10,
		Content:         "This is test content",
		NormalizedTags:  []string{"test", "go"},
	}

	records := map[string]*SearchRecord{
		post.PostID: record,
	}

	if err := m.BatchCommit([]*PostMeta{post}, records, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve search record
	retrieved, err := m.GetSearchRecord(post.PostID)
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
	if err != nil {
		t.Fatalf("GetSearchRecord should not error: %v", err)
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
	post1.PostID = "post-1"
	post2 := createSamplePostMeta()
	post2.PostID = "post-2"

	record1 := &SearchRecord{Title: "Post 1", Content: "Content 1"}
	record2 := &SearchRecord{Title: "Post 2", Content: "Content 2"}

	records := map[string]*SearchRecord{
		"post-1": record1,
		"post-2": record2,
	}

	if err := m.BatchCommit([]*PostMeta{post1, post2}, records, nil); err != nil {
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

// Tests for GetDependencies removed as method is unused and deleted
// func TestGetDependencies(t *testing.T) { ... }
// func TestGetDependencies_NotFound(t *testing.T) { ... }

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
	if err != nil {
		t.Fatalf("GetHTMLContent failed: %v", err)
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
	post1.PostID = "post-1"
	post1.Tags = []string{"go", "tutorial"}

	post2 := createSamplePostMeta()
	post2.PostID = "post-2"
	post2.Tags = []string{"go", "advanced"}

	post3 := createSamplePostMeta()
	post3.PostID = "post-3"
	post3.Tags = []string{"python", "tutorial"}

	deps1 := &Dependencies{Tags: post1.Tags}
	deps2 := &Dependencies{Tags: post2.Tags}
	deps3 := &Dependencies{Tags: post3.Tags}

	depsMap := map[string]*Dependencies{
		"post-1": deps1,
		"post-2": deps2,
		"post-3": deps3,
	}

	if err := m.BatchCommit([]*PostMeta{post1, post2, post3}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Get posts by tag
	posts, err := m.GetPostsByTag("go")
	if err != nil {
		t.Fatalf("GetPostsByTag failed: %v", err)
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
	posts, err := m.GetPostsByTag("non-existent-tag")
	if err != nil {
		t.Fatalf("GetPostsByTag failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts, got %d", len(posts))
	}
}

func TestGetPostsByTemplate(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create posts with templates
	post1 := createSamplePostMeta()
	post1.PostID = "post-1"

	post2 := createSamplePostMeta()
	post2.PostID = "post-2"

	deps1 := &Dependencies{Templates: []string{"layouts/post.html", "partials/header.html"}}
	deps2 := &Dependencies{Templates: []string{"layouts/post.html"}}

	depsMap := map[string]*Dependencies{
		"post-1": deps1,
		"post-2": deps2,
	}

	if err := m.BatchCommit([]*PostMeta{post1, post2}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Get posts by template
	posts, err := m.GetPostsByTemplate("layouts/post.html")
	if err != nil {
		t.Fatalf("GetPostsByTemplate failed: %v", err)
	}

	// Should find both posts
	if len(posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(posts))
	}
}

func TestGetCachedItem_Generic(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Test the generic getCachedItem function through GetPostByID
	post := createSamplePostMeta()
	post.PostID = "generic-test"

	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// This internally uses getCachedItem
	retrieved, err := m.GetPostByID("generic-test")
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}

	if retrieved == nil {
		t.Error("Should retrieve the post")
	}
}
