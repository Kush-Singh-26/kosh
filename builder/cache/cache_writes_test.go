package cache

import (
	"testing"
)

func TestBatchCommit_Empty(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Empty commit should not error
	if err := m.BatchCommit([]*PostMeta{}, nil, nil); err != nil {
		t.Fatalf("BatchCommit with empty posts failed: %v", err)
	}
}

func TestBatchCommit_SinglePost(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()

	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify post was stored
	retrieved, err := m.GetPostByID(post.PostID)
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}

	if retrieved == nil {
		t.Error("Post should be stored after BatchCommit")
	}
}

func TestBatchCommit_MultiplePosts(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post1 := createSamplePostMeta()
	post1.PostID = "batch-post-1"
	post2 := createSamplePostMeta()
	post2.PostID = "batch-post-2"
	post3 := createSamplePostMeta()
	post3.PostID = "batch-post-3"

	if err := m.BatchCommit([]*PostMeta{post1, post2, post3}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify all posts were stored
	for _, id := range []string{"batch-post-1", "batch-post-2", "batch-post-3"} {
		retrieved, err := m.GetPostByID(id)
		if err != nil {
			t.Fatalf("GetPostByID failed: %v", err)
		}
		if retrieved == nil {
			t.Errorf("Post %s should be stored", id)
		}
	}
}

func TestBatchCommit_WithSearchRecords(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	record := &SearchRecord{
		Title:           "Test Post",
		NormalizedTitle: "test post",
		Tokens:          []string{"test", "post"},
		BM25Data:        map[string]int{"test": 1},
		DocLen:          10,
		Content:         "Test content",
		NormalizedTags:  []string{"test"},
	}

	records := map[string]*SearchRecord{
		post.PostID: record,
	}

	if err := m.BatchCommit([]*PostMeta{post}, records, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify search record was stored
	retrieved, err := m.GetSearchRecord(post.PostID)
	if err != nil {
		t.Fatalf("GetSearchRecord failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Search record should be stored")
	}

	if retrieved.Title != record.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, record.Title)
	}
}

func TestBatchCommit_WithDependencies(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	post.PostID = "deps-test-post"

	deps := &Dependencies{
		Templates: []string{"layouts/post.html", "partials/header.html"},
		Tags:      []string{"go", "tutorial", "advanced"},
		Includes:  []string{"partials/footer.html", "partials/analytics.html"},
	}

	depsMap := map[string]*Dependencies{
		post.PostID: deps,
	}

	if err := m.BatchCommit([]*PostMeta{post}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify tags were indexed (indirectly verifying dependencies were processed)
	tagPosts, err := m.GetPostsByTag("go")
	if err != nil {
		t.Fatalf("GetPostsByTag failed: %v", err)
	}

	found := false
	for _, id := range tagPosts {
		if id == post.PostID {
			found = true
			break
		}
	}

	if !found {
		t.Error("Post should be indexed by tag 'go'")
	}
}

func TestBatchCommit_Complete(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	post.PostID = "complete-test"

	record := &SearchRecord{
		Title:  "Complete Test",
		Tokens: []string{"complete", "test"},
	}

	deps := &Dependencies{
		Templates: []string{"layouts/post.html"},
		Tags:      []string{"test"},
		Includes:  []string{"partials/footer.html"},
	}

	if err := m.BatchCommit(
		[]*PostMeta{post},
		map[string]*SearchRecord{post.PostID: record},
		map[string]*Dependencies{post.PostID: deps},
	); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify all data was stored
	retrievedPost, _ := m.GetPostByID(post.PostID)
	if retrievedPost == nil {
		t.Error("Post should be stored")
	}

	retrievedRecord, _ := m.GetSearchRecord(post.PostID)
	if retrievedRecord == nil {
		t.Error("Search record should be stored")
	}

	// Dependencies verification removed as GetDependencies was deleted
	// But we can verify side effects like tag indexing if needed
	tagPosts, _ := m.GetPostsByTag("test")
	if len(tagPosts) == 0 {
		t.Error("Post should be indexed by tag 'test'")
	}
}

func TestBatchCommit_UpdatesBuildCount(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Get initial build count
	stats, _ := m.Stats()
	initialCount := stats.BuildCount

	// Commit some posts
	post := createSamplePostMeta()
	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify build count was incremented
	stats, _ = m.Stats()
	if stats.BuildCount != initialCount+1 {
		t.Errorf("BuildCount = %d, want %d", stats.BuildCount, initialCount+1)
	}
}

func TestBatchCommit_UpdatesWriteStats(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Stats verification removed as runtime stats tracking was removed
	// Only persistence stats (BuildCount) are now verified in other tests
}

func TestStoreHTML(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	content := []byte("<html><body>Test HTML</body></html>")
	hash, err := m.StoreHTML(content)
	if err != nil {
		t.Fatalf("StoreHTML failed: %v", err)
	}

	if hash == "" {
		t.Error("StoreHTML should return a hash")
	}

	// Hash should be deterministic for same content
	hash2, err := m.StoreHTML(content)
	if err != nil {
		t.Fatalf("StoreHTML second call failed: %v", err)
	}

	if hash != hash2 {
		t.Error("StoreHTML should return same hash for same content")
	}

	// Different content should produce different hash
	differentContent := []byte("<html><body>Different content</body></html>")
	hash3, err := m.StoreHTML(differentContent)
	if err != nil {
		t.Fatalf("StoreHTML third call failed: %v", err)
	}

	if hash == hash3 {
		t.Error("Different content should produce different hash")
	}
}

func TestStoreHTMLForPost_Inline(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	smallContent := []byte("<p>Small content</p>")

	if err := m.StoreHTMLForPost(post, smallContent); err != nil {
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

func TestStoreHTMLForPost_Large(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	// Create content larger than 32KB
	largeContent := make([]byte, 35000)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	if err := m.StoreHTMLForPost(post, largeContent); err != nil {
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

func TestStoreHTMLForPost_Retrieve(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	content := []byte("<p>Test content to store</p>")

	if err := m.StoreHTMLForPost(post, content); err != nil {
		t.Fatalf("StoreHTMLForPost failed: %v", err)
	}

	// Commit the post so we can retrieve it
	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Retrieve the post and verify HTML content
	retrieved, err := m.GetPostByID(post.PostID)
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}

	htmlContent, err := m.GetHTMLContent(retrieved)
	if err != nil {
		t.Fatalf("GetHTMLContent failed: %v", err)
	}

	if string(htmlContent) != string(content) {
		t.Errorf("HTML content = %q, want %q", htmlContent, content)
	}
}

func TestStoreSSR(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	ssrType := "d2"
	inputHash := "abc123"
	content := []byte("d2 diagram content")

	artifact, err := m.StoreSSR(ssrType, inputHash, content)
	if err != nil {
		t.Fatalf("StoreSSR failed: %v", err)
	}

	if artifact == nil {
		t.Fatal("StoreSSR should return artifact")
	}

	if artifact.Type != ssrType {
		t.Errorf("Type = %q, want %q", artifact.Type, ssrType)
	}

	if artifact.InputHash != inputHash {
		t.Errorf("InputHash = %q, want %q", artifact.InputHash, inputHash)
	}

	if artifact.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", artifact.Size, len(content))
	}

	if artifact.OutputHash == "" {
		t.Error("OutputHash should be set")
	}

	if artifact.CreatedAt == 0 {
		t.Error("CreatedAt should be set")
	}
}

func TestStoreSSR_Retrieve(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	ssrType := "katex"
	inputHash := "math-123"
	content := []byte("\\frac{1}{2}")

	artifact, err := m.StoreSSR(ssrType, inputHash, content)
	if err != nil {
		t.Fatalf("StoreSSR failed: %v", err)
	}

	// Retrieve the artifact
	retrieved, err := m.GetSSRArtifact(ssrType, inputHash)
	if err != nil {
		t.Fatalf("GetSSRArtifact failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Should retrieve SSR artifact")
	}

	if retrieved.Type != artifact.Type {
		t.Errorf("Type = %q, want %q", retrieved.Type, artifact.Type)
	}

	if retrieved.InputHash != artifact.InputHash {
		t.Errorf("InputHash = %q, want %q", retrieved.InputHash, artifact.InputHash)
	}
}

func TestDeletePost(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create a post
	post := createSamplePostMeta()
	post.Tags = []string{"test", "delete"}

	deps := &Dependencies{
		Tags: []string{"test", "delete"},
	}

	if err := m.BatchCommit([]*PostMeta{post}, nil, map[string]*Dependencies{post.PostID: deps}); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify post exists
	retrieved, _ := m.GetPostByID(post.PostID)
	if retrieved == nil {
		t.Fatal("Post should exist before deletion")
	}

	// Delete the post
	if err := m.DeletePost(post.PostID); err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}

	// Verify post is deleted
	retrieved, _ = m.GetPostByID(post.PostID)
	if retrieved != nil {
		t.Error("Post should be deleted")
	}

	// Verify tags are removed
	tagPosts, _ := m.GetPostsByTag("test")
	for _, id := range tagPosts {
		if id == post.PostID {
			t.Error("Post should not be indexed by tag after deletion")
		}
	}
}

func TestDeletePost_NotFound(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Delete non-existent post should not error
	if err := m.DeletePost("non-existent-post"); err != nil {
		t.Fatalf("DeletePost should not error for non-existent post: %v", err)
	}
}

func TestDeletePost_WithSearchRecord(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	post := createSamplePostMeta()
	record := &SearchRecord{Title: "Test"}

	if err := m.BatchCommit(
		[]*PostMeta{post},
		map[string]*SearchRecord{post.PostID: record},
		nil,
	); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Delete the post
	if err := m.DeletePost(post.PostID); err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}

	// Verify search record is deleted
	retrieved, _ := m.GetSearchRecord(post.PostID)
	if retrieved != nil {
		t.Error("Search record should be deleted")
	}
}
