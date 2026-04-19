package cache

import (
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestNewService(t *testing.T) {
	mgr, cleanup := testutil.CreateTestCache(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewService(Dependencies{
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Manager: mgr,
		Logger:  logger,
	})

	if service == nil {
		t.Fatal("NewService should not return nil")
	}

	if _, ok := service.(*cacheService); !ok {
		t.Error("NewService should return *cacheService")
	}
}

func TestCacheService_GetPost(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	posts := []*models.ContentMeta{post}
	if err := service.BatchCommit(posts, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	retrieved, err := service.GetItemByID(post.ContentID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetPost should return the post")
	}

	if retrieved.ContentID != post.ContentID {
		t.Errorf("ContentID = %q, want %q", retrieved.ContentID, post.ContentID)
	}
}

func TestCacheService_GetPost_NotFound(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	retrieved, err := service.GetItemByID("non-existent-post")
	if err == nil {
		t.Fatal("GetPost should error for missing post")
	}

	if !IsCacheMiss(err) {
		t.Fatalf("Expected cache miss error, got %v", err)
	}

	if retrieved != nil {
		t.Error("GetPost should return nil for non-existent post")
	}
}

func TestCacheService_ListAllItems(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	posts, err := service.ListAllItems()
	if err != nil {
		t.Fatalf("ListAllItems failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts initially, got %d", len(posts))
	}

	post1 := testutil.CreateSamplePostMeta()
	post1.ContentID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.ContentID = "post-2"

	if err := service.BatchCommit([]*models.ContentMeta{post1, post2}, nil, nil); err != nil {
		t.Fatalf("Failed to commit posts: %v", err)
	}

	posts, err = service.ListAllItems()
	if err != nil {
		t.Fatalf("ListAllItems failed: %v", err)
	}

	if len(posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(posts))
	}
}

func TestCacheService_GetItemByPath(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	post.Path = "content/posts/my-post.md"

	if err := service.BatchCommit([]*models.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	retrieved, err := service.GetItemByPath("content/posts/my-post.md")
	if err != nil {
		t.Fatalf("GetItemByPath failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetItemByPath should return the post")
	}

	if retrieved.Path != post.Path {
		t.Errorf("Path = %q, want %q", retrieved.Path, post.Path)
	}
}

func TestCacheService_GetItemByPath_NotFound(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	retrieved, err := service.GetItemByPath("non-existent.md")
	if err == nil {
		t.Fatal("GetItemByPath should error for missing path")
	}

	if !IsCacheMiss(err) {
		t.Fatalf("Expected cache miss error, got %v", err)
	}

	if retrieved != nil {
		t.Error("GetItemByPath should return nil for missing path")
	}
}

func TestCacheService_GetItemsByIDs(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post1 := testutil.CreateSamplePostMeta()
	post1.ContentID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.ContentID = "post-2"
	post3 := testutil.CreateSamplePostMeta()
	post3.ContentID = "post-3"

	if err := service.BatchCommit([]*models.ContentMeta{post1, post2, post3}, nil, nil); err != nil {
		t.Fatalf("Failed to commit posts: %v", err)
	}

	posts, err := service.GetItemsByIDs([]string{"post-1", "post-3", "non-existent"})
	if err != nil {
		t.Fatalf("GetItemsByIDs failed: %v", err)
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

func TestCacheService_DeleteItem(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	if err := service.BatchCommit([]*models.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	retrieved, err := service.GetItemByID(post.ContentID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Post should exist before deletion")
	}

	if err := service.DeleteItem(post.ContentID); err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}

	retrieved, err = service.GetItemByID(post.ContentID)
	if err == nil {
		t.Fatal("GetPost should error after delete")
	}
	if !IsCacheMiss(err) {
		t.Fatalf("Expected cache miss error, got %v", err)
	}
	if retrieved != nil {
		t.Error("Post should not exist after deletion")
	}
}

func TestCacheService_Close(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)

	if err := service.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	cleanup()
}

func TestCacheService_Save(t *testing.T) {
	_, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()
}

func TestCacheService_Manager(t *testing.T) {
	service, mgr, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	returnedMgr := service.Manager()

	if returnedMgr != mgr {
		t.Error("Manager() should return the underlying manager")
	}
}
