package gc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/store"
	"go.etcd.io/bbolt"
)

func TestRunGC(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "meta.db")
	db, err := bbolt.Open(dbPath, 0644, nil)
	if err != nil {
		t.Fatalf("Failed to open BoltDB: %v", err)
	}
	defer db.Close()

	storePath := filepath.Join(tmpDir, "store")
	s, err := store.New(storePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()

	// Init buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, name := range core.AllBuckets() {
			_, _ = tx.CreateBucketIfNotExists([]byte(name))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to init buckets: %v", err)
	}

	refCount := NewRefCountManager(db)

	// 1. Store a post with an HTML artifact
	postID := "test-post"
	htmlContent := []byte("<html><body>Hello</body></html>")
	var htmlHash string

	// Manually store to have full control
	err = db.Update(func(tx *bbolt.Tx) error {
		var err error
		htmlHash, _, err = s.Put("html", htmlContent)
		return err
	})
	if err != nil {
		t.Fatalf("Failed to setup test data: %v", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		post := &core.PostMeta{
			PostID:   postID,
			Path:     "test.md",
			HTMLHash: htmlHash,
		}
		data, _ := core.Encode(post)
		return tx.Bucket([]byte(core.BucketPosts)).Put([]byte(postID), data)
	})
	if err != nil {
		t.Fatalf("Failed to setup post: %v", err)
	}

	// Verify artifact exists
	if !s.Exists("html", htmlHash) {
		t.Fatal("HTML artifact was not written")
	}

	// 2. Run GC - should NOT delete because it's live
	res, err := RunGC(db, s, refCount, GCConfig{MaxAge: 0})
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if res.DeletedBlobs > 0 {
		t.Errorf("GC deleted live blob: %d", res.DeletedBlobs)
	}

	// 3. Delete the post from BoltDB (simulating source file deletion)
	err = db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(core.BucketPosts)).Delete([]byte(postID))
	})
	if err != nil {
		t.Fatalf("Failed to delete post: %v", err)
	}

	// 4. Run GC - should delete the orphaned artifact
	// Backdate all files in the html store to ensure they are older than MaxAge
	htmlDir := filepath.Join(storePath, "html")
	_ = filepath.WalkDir(htmlDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			oldTime := time.Now().Add(-24 * time.Hour)
			_ = os.Chtimes(path, oldTime, oldTime)
		}
		return nil
	})

	res2, err := RunGC(db, s, refCount, GCConfig{MaxAge: 1 * time.Hour})
	if err != nil {
		t.Fatalf("GC 2 failed: %v", err)
	}

	if res2.DeletedBlobs == 0 {
		t.Error("GC did not delete orphaned blob")
	}

	// Verify artifact is gone
	if s.Exists("html", htmlHash) {
		t.Error("HTML artifact still exists after GC")
	}
}
