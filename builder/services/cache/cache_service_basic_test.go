package cache

import (
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestNewService(t *testing.T) {
	mgr, cleanup := testutil.CreateTestCache(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewService(Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
	posts := []*cache.PostMeta{post}
	if err := service.BatchCommit(posts, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

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

	retrieved, err := service.GetPost("non-existent-post")
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

func TestCacheService_ListAllPosts(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	posts, err := service.ListAllPosts()
	if err != nil {
		t.Fatalf("ListAllPosts failed: %v", err)
	}

	if len(posts) != 0 {
		t.Errorf("Expected 0 posts initially, got %d", len(posts))
	}

	post1 := testutil.CreateSamplePostMeta()
	post1.PostID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.PostID = "post-2"

	if err := service.BatchCommit([]*cache.PostMeta{post1, post2}, nil, nil); err != nil {
		t.Fatalf("Failed to commit posts: %v", err)
	}

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

	post := testutil.CreateSamplePostMeta()
	post.Path = "content/posts/my-post.md"

	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

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

func TestCacheService_GetPostByPath_NotFound(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	retrieved, err := service.GetPostByPath("non-existent.md")
	if err == nil {
		t.Fatal("GetPostByPath should error for missing path")
	}

	if !IsCacheMiss(err) {
		t.Fatalf("Expected cache miss error, got %v", err)
	}

	if retrieved != nil {
		t.Error("GetPostByPath should return nil for missing path")
	}
}

func TestCacheService_GetPostsByIDs(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post1 := testutil.CreateSamplePostMeta()
	post1.PostID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.PostID = "post-2"
	post3 := testutil.CreateSamplePostMeta()
	post3.PostID = "post-3"

	if err := service.BatchCommit([]*cache.PostMeta{post1, post2, post3}, nil, nil); err != nil {
		t.Fatalf("Failed to commit posts: %v", err)
	}

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

func TestCacheService_DeletePost(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	retrieved, err := service.GetPost(post.PostID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Post should exist before deletion")
	}

	if err := service.DeletePost(post.PostID); err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}

	retrieved, err = service.GetPost(post.PostID)
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
