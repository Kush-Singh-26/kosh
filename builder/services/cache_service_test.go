package services

import (
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func setupCacheServiceTest(t *testing.T) (*cacheServiceImpl, *cache.Manager, func()) {
	t.Helper()

	// Create a test cache
	mgr, cleanup := testutil.CreateTestCache(t)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	service := NewCacheService(mgr, logger).(*cacheServiceImpl)
	return service, mgr, cleanup
}

func TestNewCacheService(t *testing.T) {
	mgr, cleanup := testutil.CreateTestCache(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewCacheService(mgr, logger)

	if service == nil {
		t.Fatal("NewCacheService should not return nil")
	}

	if _, ok := service.(*cacheServiceImpl); !ok {
		t.Error("NewCacheService should return *cacheServiceImpl")
	}
}

func TestCacheService_GetPost(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create and store a test post
	post := testutil.CreateSamplePostMeta()
	posts := []*cache.PostMeta{post}
	if err := service.BatchCommit(posts, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	// Retrieve the post
	retrieved, err := service.GetPost(post.PostID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPost should return the post")
	}

	if retrieved.PostID != post.PostID {
		t.Errorf("PostID = %q, want %q", retrieved.PostID, post.PostID)
	}
}

func TestCacheService_GetPost_NotFound(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Try to get a non-existent post
	retrieved, err := service.GetPost("non-existent-post")
	if err != nil {
		t.Fatalf("GetPost should not error for missing post: %v", err)
	}

	if retrieved != nil {
		t.Error("GetPost should return nil for non-existent post")
	}
}

func TestCacheService_ListAllPosts(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Initially should be empty
	posts, err := service.ListAllPosts()
	if err != nil {
		t.Fatalf("ListAllPosts failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts initially, got %d", len(posts))
	}

	// Add some posts
	post1 := testutil.CreateSamplePostMeta()
	post1.PostID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.PostID = "post-2"

	if err := service.BatchCommit([]*cache.PostMeta{post1, post2}, nil, nil); err != nil {
		t.Fatalf("Failed to commit posts: %v", err)
	}

	// Should now have 2 posts
	posts, err = service.ListAllPosts()
	if err != nil {
		t.Fatalf("ListAllPosts failed: %v", err)
	}

	if len(posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(posts))
	}
}

func TestCacheService_GetPostByPath(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create and store a test post
	post := testutil.CreateSamplePostMeta()
	post.Path = "content/posts/my-post.md"

	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	// Retrieve by path
	retrieved, err := service.GetPostByPath("content/posts/my-post.md")
	if err != nil {
		t.Fatalf("GetPostByPath failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPostByPath should return the post")
	}

	if retrieved.Path != post.Path {
		t.Errorf("Path = %q, want %q", retrieved.Path, post.Path)
	}
}

func TestCacheService_GetPostsByIDs(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create and store test posts
	post1 := testutil.CreateSamplePostMeta()
	post1.PostID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.PostID = "post-2"
	post3 := testutil.CreateSamplePostMeta()
	post3.PostID = "post-3"

	if err := service.BatchCommit([]*cache.PostMeta{post1, post2, post3}, nil, nil); err != nil {
		t.Fatalf("Failed to commit posts: %v", err)
	}

	// Retrieve specific posts
	posts, err := service.GetPostsByIDs([]string{"post-1", "post-3", "non-existent"})
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

func TestCacheService_GetSearchRecord(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create a post with search record
	post := testutil.CreateSamplePostMeta()
	record := testutil.CreateSampleSearchRecord()
	records := map[string]*cache.SearchRecord{
		post.PostID: record,
	}

	if err := service.BatchCommit([]*cache.PostMeta{post}, records, nil); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Retrieve search record
	retrieved, err := service.GetSearchRecord(post.PostID)
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

func TestCacheService_StoreHTMLAndRetrieve(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Store some HTML
	content := []byte("<html><body>Test content</body></html>")
	hash, err := service.StoreHTML(content)
	if err != nil {
		t.Fatalf("StoreHTML failed: %v", err)
	}

	if hash == "" {
		t.Error("StoreHTML should return a hash")
	}

	// Create a post referencing this HTML
	post := testutil.CreateSamplePostMeta()
	post.HTMLHash = hash
	post.InlineHTML = nil

	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	// Retrieve HTML content
	retrieved, err := service.GetHTMLContent(post)
	if err != nil {
		t.Fatalf("GetHTMLContent failed: %v", err)
	}

	if string(retrieved) != string(content) {
		t.Errorf("Content mismatch: got %q, want %q", retrieved, content)
	}
}

func TestCacheService_StoreHTMLForPost_Inline(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create a post with small HTML (should be inlined)
	post := testutil.CreateSamplePostMeta()
	smallContent := []byte("<p>Small content</p>")

	if err := service.StoreHTMLForPost(post, smallContent); err != nil {
		t.Fatalf("StoreHTMLForPost failed: %v", err)
	}

	// Small content should be inlined
	if post.InlineHTML == nil {
		t.Error("Small content should be inlined")
	}

	if string(post.InlineHTML) != string(smallContent) {
		t.Errorf("InlineHTML = %q, want %q", post.InlineHTML, smallContent)
	}

	if post.HTMLHash != "" {
		t.Error("HTMLHash should be empty for inlined content")
	}
}

func TestCacheService_StoreHTMLForPost_Large(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create a post with large HTML (should be stored by hash)
	post := testutil.CreateSamplePostMeta()
	largeContent := testutil.CreateLargeHTML()

	if err := service.StoreHTMLForPost(post, largeContent); err != nil {
		t.Fatalf("StoreHTMLForPost failed: %v", err)
	}

	// Large content should not be inlined
	if post.InlineHTML != nil {
		t.Error("Large content should not be inlined")
	}

	if post.HTMLHash == "" {
		t.Error("HTMLHash should be set for large content")
	}
}

func TestCacheService_DirtyTracking(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	postID := "test-post-123"

	// Initially not dirty
	if service.IsDirty(postID) {
		t.Error("Post should not be dirty initially")
	}

	// Mark as dirty
	service.MarkDirty(postID)

	// Should now be dirty
	if !service.IsDirty(postID) {
		t.Error("Post should be dirty after MarkDirty")
	}

	// Mark same post again (should not panic)
	service.MarkDirty(postID)

	// Should still be dirty
	if !service.IsDirty(postID) {
		t.Error("Post should still be dirty")
	}
}

func TestCacheService_DeletePost(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create and store a test post
	post := testutil.CreateSamplePostMeta()
	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	// Verify post exists
	retrieved, err := service.GetPost(post.PostID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Post should exist before deletion")
	}

	// Delete the post
	if err := service.DeletePost(post.PostID); err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}

	// Verify post is deleted
	retrieved, err = service.GetPost(post.PostID)
	if err != nil {
		t.Fatalf("GetPost failed after delete: %v", err)
	}
	if retrieved != nil {
		t.Error("Post should not exist after deletion")
	}
}

func TestCacheService_SocialCardHash(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	path := "content/posts/test.md"
	expectedHash := "abc123"

	// Set hash
	if err := service.SetSocialCardHash(path, expectedHash); err != nil {
		t.Fatalf("SetSocialCardHash failed: %v", err)
	}

	// Get hash
	hash, err := service.GetSocialCardHash(path)
	if err != nil {
		t.Fatalf("GetSocialCardHash failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Hash = %q, want %q", hash, expectedHash)
	}

	// Get non-existent path
	hash, err = service.GetSocialCardHash("non-existent.md")
	if err != nil {
		t.Fatalf("GetSocialCardHash should not error: %v", err)
	}

	if hash != "" {
		t.Errorf("Hash for non-existent path should be empty, got %q", hash)
	}
}

func TestCacheService_GraphHash(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	expectedHash := "graph-hash-123"

	// Set hash
	if err := service.SetGraphHash(expectedHash); err != nil {
		t.Fatalf("SetGraphHash failed: %v", err)
	}

	// Get hash
	hash, err := service.GetGraphHash()
	if err != nil {
		t.Fatalf("GetGraphHash failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Hash = %q, want %q", hash, expectedHash)
	}
}

func TestCacheService_WasmHash(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	expectedHash := "wasm-hash-456"

	// Set hash
	if err := service.SetWasmHash(expectedHash); err != nil {
		t.Fatalf("SetWasmHash failed: %v", err)
	}

	// Get hash
	hash, err := service.GetWasmHash()
	if err != nil {
		t.Fatalf("GetWasmHash failed: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("Hash = %q, want %q", hash, expectedHash)
	}
}

func TestCacheService_IncrementBuildCount(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Get initial stats
	stats, err := service.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	initialCount := stats.BuildCount

	// Increment build count
	if err := service.IncrementBuildCount(); err != nil {
		t.Fatalf("IncrementBuildCount failed: %v", err)
	}

	// Check stats again
	stats, err = service.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.BuildCount != initialCount+1 {
		t.Errorf("BuildCount = %d, want %d", stats.BuildCount, initialCount+1)
	}
}

func TestCacheService_Close(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)

	// Close the service
	if err := service.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Cleanup should not panic even after Close
	cleanup()
}

func TestCacheService_Save(t *testing.T) {
	_, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Save method was removed as it was empty
}

func TestCacheService_Manager(t *testing.T) {
	service, mgr, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Manager should return the underlying manager
	returnedMgr := service.Manager()

	if returnedMgr != mgr {
		t.Error("Manager() should return the underlying manager")
	}
}

func TestCacheService_BatchCommit_WithDependencies(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create a post with dependencies
	post := testutil.CreateSamplePostMeta()
	deps := &cache.Dependencies{
		Templates: []string{"layouts/post.html", "partials/header.html"},
		Tags:      []string{"go", "tutorial"},
		Includes:  []string{"partials/footer.html"},
	}

	depsMap := map[string]*cache.Dependencies{
		post.PostID: deps,
	}

	// Commit with dependencies
	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Post should be retrievable
	retrieved, err := service.GetPost(post.PostID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if retrieved == nil {
		t.Error("Post should exist after commit")
	}
}

func TestCacheService_GetPostsByTemplate(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// This is a more complex test - for now just verify it doesn't panic/error
	posts, err := service.GetPostsByTemplate("layouts/post.html")
	if err != nil {
		t.Fatalf("GetPostsByTemplate failed: %v", err)
	}

	// Initially should be empty or nil (implementation dependent)
	if len(posts) > 0 {
		t.Error("GetPostsByTemplate should return empty results for non-existent template")
	}
}

func TestCacheService_GetSearchRecords(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	// Create posts with search records
	post1 := testutil.CreateSamplePostMeta()
	post1.PostID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.PostID = "post-2"

	record1 := testutil.CreateSampleSearchRecord()
	record1.Title = "Post 1"
	record2 := testutil.CreateSampleSearchRecord()
	record2.Title = "Post 2"

	records := map[string]*cache.SearchRecord{
		"post-1": record1,
		"post-2": record2,
	}

	if err := service.BatchCommit([]*cache.PostMeta{post1, post2}, records, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve search records
	retrieved, err := service.GetSearchRecords([]string{"post-1", "post-2", "non-existent"})
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
