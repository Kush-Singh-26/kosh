package store

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/retry"
)

func TestRenameWithRetry_Succeeds(t *testing.T) {
	d := t.TempDir()
	tmp := filepath.Join(d, "a.tmp")
	final := filepath.Join(d, "a.raw")
	if err := os.WriteFile(tmp, []byte("x"), 0644); err != nil {
		t.Fatalf("write tmp failed: %v", err)
	}

	if err := retry.RenameWithRetry(retry.RenameOptions{
		Ctx:        context.Background(),
		OldPath:    tmp,
		NewPath:    final,
		MaxRetries: 3,
		BaseDelay:  1*time.Millisecond,
	}); err != nil {
		t.Fatalf("RenameWithRetry should succeed: %v", err)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("expected final file to exist: %v", err)
	}
}

func TestRenameWithRetry_FailsWhenMissing(t *testing.T) {
	d := t.TempDir()
	tmp := filepath.Join(d, "missing.tmp")
	final := filepath.Join(d, "a.raw")

	err := retry.RenameWithRetry(retry.RenameOptions{
		Ctx:        context.Background(),
		OldPath:    tmp,
		NewPath:    final,
		MaxRetries: 2,
		BaseDelay:  1*time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected RenameWithRetry to fail for missing temp file")
	}
}

func TestCleanOrphans(t *testing.T) {
	basePath, err := os.MkdirTemp("", "kosh-store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(basePath) }()

	store, err := New(basePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	cat := "html"
	hashLive, _, _ := store.Put(cat, []byte("live content"))

	hashNewOrphan, _, _ := store.Put(cat, []byte("new orphan"))

	hashOldOrphan, _, _ := store.Put(cat, []byte("old orphan"))

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	oldPathRaw := filepath.Join(basePath, cat, hashOldOrphan[0:2], hashOldOrphan+".raw")
	oldPathZst := filepath.Join(basePath, cat, hashOldOrphan[0:2], hashOldOrphan+".zst")
	_ = os.Chtimes(oldPathRaw, oldTime, oldTime)
	_ = os.Chtimes(oldPathZst, oldTime, oldTime)

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

func TestStorePut_ConcurrentSameContent(t *testing.T) {
	basePath := t.TempDir()
	store, err := New(basePath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := store.Put("ssr/d2", []byte("same content"))
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("expected concurrent Put success, got error: %v", err)
		}
	}
}

var _ = core.SchemaVersion
