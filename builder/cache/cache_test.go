package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestCache creates a temporary cache for testing
func createTestCache(t *testing.T) (*Manager, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	m, err := Open(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to open cache: %v", err)
	}
	return m, func() {
		_ = m.Close()
	}
}

// createSamplePostMeta creates a sample PostMeta for testing
func createSamplePostMeta() *PostMeta {
	return &PostMeta{
		PostID:      "test-post",
		Title:       "Test Post",
		Path:        "content/posts/test-post.md",
		Date:        time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		Tags:        []string{"test", "go", "tutorial"},
		Description: "A test post for testing purposes",
		Draft:       false,
		Weight:      10,
		WordCount:   150,
		ReadingTime: 1,
		Meta:        make(map[string]interface{}),
	}
}

func TestOpen_NewCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	m, err := Open(cacheDir, false)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	}()

	if m == nil {
		t.Fatal("Open() returned nil Manager")
	}

	// Verify cache directory was created
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("Cache directory should be created")
	}

	// Verify database file exists
	dbPath := filepath.Join(cacheDir, "meta.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file should be created")
	}

	// Note: Store directory may be created lazily on first use
	// The important thing is that the Manager has a valid store
	if m.store == nil {
		t.Error("Manager should have a store")
	}
}

func TestOpen_DevMode(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	m, err := Open(cacheDir, true)
	if err != nil {
		t.Fatalf("Open() in dev mode failed: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	}()

	if m == nil {
		t.Fatal("Open() returned nil Manager")
	}
}

func TestOpen_ExistingCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create cache first time
	m1, err := Open(cacheDir, false)
	if err != nil {
		t.Fatalf("First Open() failed: %v", err)
	}

	// Set a cache ID
	if err := m1.SetCacheID("test-id-123"); err != nil {
		t.Fatalf("SetCacheID failed: %v", err)
	}
	if err := m1.Close(); err != nil {
		t.Errorf("First Close() failed: %v", err)
	}

	// Re-open existing cache
	m2, err := Open(cacheDir, false)
	if err != nil {
		t.Fatalf("Second Open() failed: %v", err)
	}
	defer func() {
		if err := m2.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}
	}()

	// Verify cache ID persisted
	needsRebuild, err := m2.VerifyCacheID("test-id-123")
	if err != nil {
		t.Fatalf("VerifyCacheID failed: %v", err)
	}

	if needsRebuild {
		t.Error("Cache ID should match, no rebuild needed")
	}
}

func TestManager_Close(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	m, err := Open(cacheDir, false)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}

	// Close should not error
	if err := m.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Double close should not panic (may return error)
	_ = m.Close()
}

func TestManager_VerifyCacheID_New(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// New cache should need rebuild
	needsRebuild, err := m.VerifyCacheID("new-id")
	if err != nil {
		t.Fatalf("VerifyCacheID failed: %v", err)
	}

	if !needsRebuild {
		t.Error("New cache should need rebuild")
	}
}

func TestManager_VerifyCacheID_Match(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Set cache ID
	if err := m.SetCacheID("test-id"); err != nil {
		t.Fatalf("SetCacheID failed: %v", err)
	}

	// Verify same ID
	needsRebuild, err := m.VerifyCacheID("test-id")
	if err != nil {
		t.Fatalf("VerifyCacheID failed: %v", err)
	}

	if needsRebuild {
		t.Error("Matching ID should not need rebuild")
	}
}

func TestManager_VerifyCacheID_Mismatch(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Set cache ID
	if err := m.SetCacheID("old-id"); err != nil {
		t.Fatalf("SetCacheID failed: %v", err)
	}

	// Verify different ID
	needsRebuild, err := m.VerifyCacheID("new-id")
	if err != nil {
		t.Fatalf("VerifyCacheID failed: %v", err)
	}

	if !needsRebuild {
		t.Error("Different ID should need rebuild")
	}
}

func TestManager_SetCacheID(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	testID := "test-cache-id-12345"

	if err := m.SetCacheID(testID); err != nil {
		t.Fatalf("SetCacheID failed: %v", err)
	}

	// Verify it was set
	needsRebuild, err := m.VerifyCacheID(testID)
	if err != nil {
		t.Fatalf("VerifyCacheID failed: %v", err)
	}

	if needsRebuild {
		t.Error("Cache ID should be set correctly")
	}
}

func TestManager_Store(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	store := m.Store()
	if store == nil {
		t.Error("Store() should return non-nil store")
	}
}

func TestManager_DB(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	db := m.DB()
	if db == nil {
		t.Error("DB() should return non-nil database")
	}
}

func TestManager_DirtyTracking(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	postID := "test-post-123"

	// Initially not dirty
	if m.IsDirty(postID) {
		t.Error("Post should not be dirty initially")
	}

	// Mark as dirty
	m.MarkDirty(postID)

	// Should now be dirty
	if !m.IsDirty(postID) {
		t.Error("Post should be dirty after MarkDirty")
	}

	// Check internal dirty map
	m.mu.RLock()
	dirty := m.dirty[postID]
	m.mu.RUnlock()

	if !dirty {
		t.Error("Internal dirty map should have the post")
	}
}

func TestManager_ClearDirty(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	postID := "test-post-123"

	// Mark as dirty
	m.MarkDirty(postID)

	if !m.IsDirty(postID) {
		t.Fatal("Post should be dirty before clearing")
	}

	// Clear dirty (not exported, test via internal access)
	m.mu.Lock()
	delete(m.dirty, postID)
	m.mu.Unlock()

	// Should not be dirty anymore
	if m.IsDirty(postID) {
		t.Error("Post should not be dirty after clearing")
	}
}

func TestManager_MultipleDirtyPosts(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	postIDs := []string{"post-1", "post-2", "post-3"}

	// Mark all as dirty
	for _, id := range postIDs {
		m.MarkDirty(id)
	}

	// Verify all are dirty
	for _, id := range postIDs {
		if !m.IsDirty(id) {
			t.Errorf("Post %s should be dirty", id)
		}
	}

	// Verify non-existent post is not dirty
	if m.IsDirty("non-existent") {
		t.Error("Non-existent post should not be dirty")
	}
}

func TestEncodedPost_Pool(t *testing.T) {
	// Get from pool
	item := encodedPostPool.Get().([]EncodedPost)
	if item == nil {
		t.Fatal("Pool should return non-nil slice")
	}

	// Reset and return to pool
	for i := range item {
		item[i] = EncodedPost{}
	}
	//nolint:staticcheck // SA6002 - slices are reference types
	encodedPostPool.Put(item)

	// Get again - should be the same or similar item
	item2 := encodedPostPool.Get().([]EncodedPost)
	if item2 == nil {
		t.Fatal("Pool should return non-nil slice on second get")
	}

	// Return to pool
	//nolint:staticcheck // SA6002 - slices are reference types
	encodedPostPool.Put(item2)
}

func TestWriteOps(t *testing.T) {
	// This is tested indirectly through BatchCommit and other write operations
	// The writeOps function is internal and tested through higher-level functions
}

func TestManager_Stats(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	stats, err := m.Stats()
	if err != nil {
		t.Fatalf("Stats() failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Stats() should return non-nil stats")
	}

	// New cache should have 0 posts
	if stats.TotalPosts != 0 {
		t.Errorf("TotalPosts = %d, want 0", stats.TotalPosts)
	}
}

func TestManager_IncrementBuildCount(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Get initial stats
	stats, _ := m.Stats()
	initialCount := stats.BuildCount

	// Increment
	if err := m.IncrementBuildCount(); err != nil {
		t.Fatalf("IncrementBuildCount() failed: %v", err)
	}

	// Check stats
	stats, _ = m.Stats()
	if stats.BuildCount != initialCount+1 {
		t.Errorf("BuildCount = %d, want %d", stats.BuildCount, initialCount+1)
	}

	// Increment again
	if err := m.IncrementBuildCount(); err != nil {
		t.Fatalf("IncrementBuildCount() second call failed: %v", err)
	}

	stats, _ = m.Stats()
	if stats.BuildCount != initialCount+2 {
		t.Errorf("BuildCount = %d, want %d", stats.BuildCount, initialCount+2)
	}
}

func TestManager_ListAllPosts(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Initially empty
	posts, err := m.ListAllPosts()
	if err != nil {
		t.Fatalf("ListAllPosts() failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts, got %d", len(posts))
	}

	// Add some posts
	post1 := createSamplePostMeta()
	post1.PostID = "post-1"
	post2 := createSamplePostMeta()
	post2.PostID = "post-2"

	if err := m.BatchCommit([]*PostMeta{post1, post2}, nil, nil); err != nil {
		t.Fatalf("BatchCommit() failed: %v", err)
	}

	// Should now have 2 posts
	posts, err = m.ListAllPosts()
	if err != nil {
		t.Fatalf("ListAllPosts() failed: %v", err)
	}

	if len(posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(posts))
	}
}
