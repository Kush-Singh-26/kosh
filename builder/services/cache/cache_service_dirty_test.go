package cache

import (
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestCacheService_DirtyTracking(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ContentID := "test-post-123"

	if service.IsDirty(ContentID) {
		t.Error("Post should not be dirty initially")
	}

	service.MarkDirty(ContentID)

	if !service.IsDirty(ContentID) {
		t.Error("Post should be dirty after MarkDirty")
	}

	service.MarkDirty(ContentID)

	if !service.IsDirty(ContentID) {
		t.Error("Post should still be dirty")
	}
}

func TestCacheService_ClearDirty_RangeDelete(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	postIDs := []string{"post-1", "post-2", "post-3", "post-4", "post-5"}

	for _, id := range postIDs {
		service.MarkDirty(id)
	}

	for _, id := range postIDs {
		if !service.IsDirty(id) {
			t.Errorf("Post %s should be dirty", id)
		}
	}

	service.ClearDirty()

	for _, id := range postIDs {
		if service.IsDirty(id) {
			t.Errorf("Post %s should not be dirty after ClearDirty", id)
		}
	}
}

func TestCacheService_ClearDirty_Concurrent(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	var wg sync.WaitGroup

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			service.MarkDirty("post-" + string(rune('0'+id)))
		}(i)
	}

	wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		service.ClearDirty()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			service.MarkDirty("concurrent-post")
		}
	}()

	wg.Wait()
}

func TestCacheService_EmptyBodyHash_Invalidation(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	post.ContentID = "test-empty-body"
	post.BodyHash = ""

	if err := service.BatchCommit([]*cache.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	retrieved, err := service.GetItemByID("test-empty-body")
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Post should be retrievable")
	}

	if retrieved.BodyHash != "" {
		t.Logf("Empty BodyHash stored as: %q", retrieved.BodyHash)
	}
}

func TestCacheService_ConcurrentDirtyTracking(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	const numGoroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				ContentID := string(rune(base%26+'a')) + string(rune(i%26+'a'))
				service.MarkDirty(ContentID)
				_ = service.IsDirty(ContentID)
			}
		}(g)
	}

	wg.Wait()

	service.ClearDirty()

	for i := 0; i < 10; i++ {
		ContentID := string(rune(i%26+'a')) + "0"
		if service.IsDirty(ContentID) {
			t.Errorf("Post %s should be clean after ClearDirty", ContentID)
		}
	}
}

func TestCacheService_ConcurrentMarkAndCheck(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	const numPosts = 100
	var wg sync.WaitGroup

	for i := 0; i < numPosts; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ContentID := string(rune(id%26 + 'a'))
			service.MarkDirty(ContentID)
			_ = service.IsDirty(ContentID)
		}(i)
	}

	wg.Wait()

	service.ClearDirty()
}

func TestCacheService_DirtyTrackingIdempotency(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ContentID := "test-idempotent"

	for i := 0; i < 10; i++ {
		service.MarkDirty(ContentID)
	}

	if !service.IsDirty(ContentID) {
		t.Error("Post should be dirty after multiple MarkDirty calls")
	}

	service.ClearDirty()
	if service.IsDirty(ContentID) {
		t.Error("Post should be clean after ClearDirty")
	}
}
