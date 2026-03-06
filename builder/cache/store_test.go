package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanOrphans(t *testing.T) {
	basePath, err := os.MkdirTemp("", "kosh-store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(basePath) }()

	store, err := NewStore(basePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	cat := "html"
	// 1. Create a "live" blob
	hashLive, _, _ := store.Put(cat, []byte("live content"))

	// 2. Create an "orphaned" blob but it's new (inside TTL)
	hashNewOrphan, _, _ := store.Put(cat, []byte("new orphan"))

	// 3. Create an "orphaned" blob that is old (outside TTL)
	hashOldOrphan, _, _ := store.Put(cat, []byte("old orphan"))

	// Manually force the old orphan to have an old modtime
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	oldPathRaw := filepath.Join(basePath, cat, hashOldOrphan[0:2], hashOldOrphan[2:4], hashOldOrphan+".raw")
	oldPathZst := filepath.Join(basePath, cat, hashOldOrphan[0:2], hashOldOrphan[2:4], hashOldOrphan+".zst")
	_ = os.Chtimes(oldPathRaw, oldTime, oldTime)
	_ = os.Chtimes(oldPathZst, oldTime, oldTime)

	// Clean orphans with a 7-day TTL
	liveHashes := map[string]bool{
		hashLive: true,
	}

	deleted, _, err := store.CleanOrphans(cat, liveHashes, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOrphans failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("Expected 1 blob to be deleted, got %d", deleted)
	}

	if !store.Exists(cat, hashLive) {
		t.Error("Live blob should not have been deleted")
	}
	if !store.Exists(cat, hashNewOrphan) {
		t.Error("New orphaned blob (inside TTL) should not have been deleted")
	}
	if store.Exists(cat, hashOldOrphan) {
		t.Error("Old orphaned blob (outside TTL) should have been deleted")
	}
}
