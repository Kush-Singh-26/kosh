package cache

import (
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

// TestDeletePost_BoltDBError tests that DeletePost returns error when database is closed
func TestDeletePost_BoltDBError(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// First, commit a post to ensure it exists
	post := createSamplePostMeta()
	post.PostID = "error-test-post"
	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Close the database to simulate it being unavailable
	if err := m.Close(); err != nil {
		t.Fatalf("Failed to close database: %v", err)
	}

	// Attempt deletion should fail because database is closed
	err := m.DeletePost(post.PostID)
	if err == nil {
		t.Error("DeletePost should return error after database is closed")
	}
}

// TestDeletePost_Concurrent tests concurrent deletion operations
func TestDeletePost_Concurrent(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Create multiple posts
	numPosts := 10
	postIDs := make([]string, numPosts)
	for i := 0; i < numPosts; i++ {
		post := createSamplePostMeta()
		post.PostID = "concurrent-post-" + string(rune(i))
		postIDs[i] = post.PostID
		if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
			t.Fatalf("BatchCommit failed for post %d: %v", i, err)
		}
	}

	// Delete concurrently
	errors := make(chan error, numPosts)
	for _, id := range postIDs {
		go func(postID string) {
			errors <- m.DeletePost(postID)
		}(id)
	}

	// Collect results
	for i := 0; i < numPosts; i++ {
		err := <-errors
		if err != nil {
			t.Errorf("Concurrent delete failed: %v", err)
		}
	}

	// Verify all posts deleted
	for _, id := range postIDs {
		retrieved, _ := m.GetPostByID(id)
		if retrieved != nil {
			t.Errorf("Post %s should be deleted", id)
		}
	}
}

// TestClearAll_FilesystemError tests ClearAll handling of filesystem permission errors
func TestClearAll_FilesystemError(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Add some data first
	post := createSamplePostMeta()
	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Make the store directory read-only to cause permission errors during clearFilesystemStore
	storeDir := filepath.Join(m.basePath, "store")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatalf("Failed to create store dir: %v", err)
	}
	if err := os.Chmod(storeDir, 0555); err != nil {
		t.Skipf("Cannot set directory permissions: %v", err)
	}
	defer func() { _ = os.Chmod(storeDir, 0755) }() // Cleanup

	// ClearAll should log warnings but still succeed in clearing database
	err := m.ClearAll()
	if err != nil {
		t.Errorf("ClearAll returned error: %v", err)
	}

	// Verify database is cleared (buckets deleted)
	retrieved, _ := m.GetPostByID(post.PostID)
	if retrieved != nil {
		t.Error("Post should be deleted from database after ClearAll")
	}
}

// TestClear_FullResetError tests full cache reset error handling
func TestClear_FullResetError(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Add some data
	post := createSamplePostMeta()
	if err := m.BatchCommit([]*PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Make the database file read-only to cause error on Close
	dbPath := filepath.Join(m.basePath, "meta.db")
	if err := os.Chmod(dbPath, 0444); err != nil {
		t.Skipf("Cannot set file permissions: %v", err)
	}

	// Clear should handle error gracefully
	err := m.Clear()
	if err != nil {
		t.Errorf("Clear should not return error, got: %v", err)
	}

	// Verify cache is reset (new database should be created)
	newPost := createSamplePostMeta()
	newPost.PostID = "new-post"
	err = m.BatchCommit([]*PostMeta{newPost}, nil, nil)
	if err != nil {
		t.Fatalf("BatchCommit after Clear failed: %v", err)
	}

	retrieved, _ := m.GetPostByID(newPost.PostID)
	if retrieved == nil {
		t.Error("New post should be stored after Clear")
	}
}

// TestStoreDelete_BestEffort tests that Store.Delete is best-effort and never returns error
func TestStoreDelete_BestEffort(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	store := m.store

	// Test Delete on non-existent file should not error
	err := store.Delete("html", "nonexistent-hash")
	if err != nil {
		t.Errorf("Store.Delete should return nil for non-existent file, got: %v", err)
	}

	// Create a file via Put
	hash, _, err := store.Put("html", []byte("test content"))
	if err != nil {
		t.Fatalf("Store.Put failed: %v", err)
	}

	// Normal delete should succeed
	err = store.Delete("html", hash)
	if err != nil {
		t.Errorf("Store.Delete should not error: %v", err)
	}
}

// TestDeletePost_CorruptedData tests deletion when PostMeta data is corrupted
func TestDeletePost_CorruptedData(t *testing.T) {
	m, cleanup := createTestCache(t)
	defer cleanup()

	// Directly insert corrupted data into the database
	postID := "corrupted-post"
	path := "content/corrupted.md"

	// Use a transaction to insert invalid data (malformed JSON)
	err := m.db.Update(func(tx *bolt.Tx) error {
		postsBucket := tx.Bucket([]byte(BucketPosts))
		if postsBucket == nil {
			// Create bucket if it doesn't exist (shouldn't happen in normal operation)
			_, err := tx.CreateBucket([]byte(BucketPosts))
			if err != nil {
				return err
			}
			postsBucket = tx.Bucket([]byte(BucketPosts))
		}
		// Create a malformed JSON that will fail unmarshaling
		corruptedData := []byte("{invalid json")
		if err := postsBucket.Put([]byte(postID), corruptedData); err != nil {
			return err
		}

		// Also add to path index with corrupted data
		pathBucket := tx.Bucket([]byte(BucketPaths))
		if pathBucket == nil {
			_, err := tx.CreateBucket([]byte(BucketPaths))
			if err != nil {
				return err
			}
			pathBucket = tx.Bucket([]byte(BucketPaths))
		}
		_ = pathBucket.Put([]byte(path), []byte(postID))

		return nil
	})
	if err != nil {
		t.Fatalf("Failed to insert corrupted data: %v", err)
	}

	// Deletion should succeed even with corrupted data (best-effort)
	err = m.DeletePost(postID)
	if err != nil {
		t.Errorf("DeletePost should handle corrupted data gracefully: %v", err)
	}

	// Verify post is removed
	retrieved, _ := m.GetPostByID(postID)
	if retrieved != nil {
		t.Error("Corrupted post should be deleted")
	}
}
