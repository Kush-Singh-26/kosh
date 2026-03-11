package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestRunGC(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := Open(tmpDir, true)
	if err != nil {
		t.Fatalf("Failed to open cache: %v", err)
	}
	defer m.Close()

	// 1. Store a post with an HTML artifact
	postID := "test-post"
	htmlContent := []byte("<html><body>Hello</body></html>")
	var htmlHash string

	// Manually store to have full control
	err = m.db.Update(func(tx *bolt.Tx) error {
		var err error
		htmlHash, _, err = m.store.Put("html", htmlContent)
		return err
	})
	if err != nil {
		t.Fatalf("Failed to setup test data: %v", err)
	}

	err = m.db.Update(func(tx *bolt.Tx) error {
		post := &PostMeta{
			PostID:   postID,
			Path:     "test.md",
			HTMLHash: htmlHash,
		}
		data, _ := Encode(post)
		return tx.Bucket([]byte(BucketPosts)).Put([]byte(postID), data)
	})
	if err != nil {
		t.Fatalf("Failed to setup post: %v", err)
	}

	// Verify artifact exists
	if !m.store.Exists("html", htmlHash) {
		t.Fatal("HTML artifact was not written")
	}

	// 2. Run GC - should NOT delete because it's live
	res, err := m.RunGC(GCConfig{MaxAge: 0})
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if res.DeletedBlobs > 0 {
		t.Errorf("GC deleted live blob: %d", res.DeletedBlobs)
	}

	// 3. Delete the post from BoltDB (simulating source file deletion)
	err = m.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketPosts)).Delete([]byte(postID))
	})
	if err != nil {
		t.Fatalf("Failed to delete post: %v", err)
	}

	// 4. Run GC - should delete the orphaned artifact
	// Backdate all files in the html store to ensure they are older than MaxAge
	htmlDir := filepath.Join(tmpDir, "store", "html")
	_ = filepath.WalkDir(htmlDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			oldTime := time.Now().Add(-24 * time.Hour)
			_ = os.Chtimes(path, oldTime, oldTime)
		}
		return nil
	})

	res2, err := m.RunGC(GCConfig{MaxAge: 1 * time.Hour})
	if err != nil {
		t.Fatalf("GC 2 failed: %v", err)
	}

	if res2.DeletedBlobs == 0 {
		t.Error("GC did not delete orphaned blob")
	}

	// Verify artifact is gone
	if m.store.Exists("html", htmlHash) {
		t.Error("HTML artifact still exists after GC")
	}
}
