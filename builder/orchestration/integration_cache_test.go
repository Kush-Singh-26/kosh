package orchestration

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestCacheService_DirtyTrackingIntegration(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	cacheDir := t.TempDir()

	logger := InitLogger()
	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx:     buildCtx.NewBuildContext(buildCtx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Manager: cacheManager,
		Logger:  logger,
	})

	postPath := "content/posts/hello.md"
	cacheSvc.MarkDirty(postPath)

	isDirty := cacheSvc.IsDirty(postPath)
	if !isDirty {
		t.Error("Expected post to be marked as dirty")
	}

	cacheSvc.ClearDirty()

	isDirty = cacheSvc.IsDirty(postPath)
	if isDirty {
		t.Error("Expected post to not be dirty after ClearDirty")
	}
}

func TestCacheService_BatchCommitIntegration(t *testing.T) {
	cacheDir := t.TempDir()

	logger := InitLogger()
	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx:     buildCtx.NewBuildContext(buildCtx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Manager: cacheManager,
		Logger:  logger,
	})

	posts := []*cache.PostMeta{
		{
			PostID:      "post-1",
			Title:       "Post 1",
			Path:        "content/posts/post1.md",
			BodyHash:    "hash1",
			WordCount:   100,
			ReadingTime: 1,
		},
		{
			PostID:      "post-2",
			Title:       "Post 2",
			Path:        "content/posts/post2.md",
			BodyHash:    "hash2",
			WordCount:   200,
			ReadingTime: 2,
		},
		{
			PostID:      "post-3",
			Title:       "Post 3",
			Path:        "content/posts/post3.md",
			BodyHash:    "hash3",
			WordCount:   300,
			ReadingTime: 3,
		},
	}

	err = cacheSvc.BatchCommit(posts, nil, nil)
	if err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	for i, expected := range posts {
		meta, err := cacheManager.GetPostByID(expected.PostID)
		if err != nil {
			t.Errorf("Failed to get post %d: %v", i, err)
			continue
		}
		if meta == nil {
			t.Errorf("Post %d not found in cache", i)
			continue
		}
		if meta.Title != expected.Title {
			t.Errorf("Post %d title mismatch: %s vs %s", i, meta.Title, expected.Title)
		}
	}
}

func TestCacheService_SocialCardHashPersistence(t *testing.T) {
	cacheDir := t.TempDir()

	logger := InitLogger()
	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx:     buildCtx.NewBuildContext(buildCtx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Manager: cacheManager,
		Logger:  logger,
	})

	hashes := map[string]string{
		"posts/hello.md": "abc123",
		"posts/world.md": "def456",
		"pages/about.md": "ghi789",
	}

	err = cacheSvc.BatchSetSocialCardHashes(hashes)
	if err != nil {
		t.Fatalf("BatchSetSocialCardHashes failed: %v", err)
	}

	for path, expectedHash := range hashes {
		hash, err := cacheManager.GetSocialCardHash(path)
		if err != nil {
			t.Errorf("Failed to get hash for %s: %v", path, err)
			continue
		}
		if hash != expectedHash {
			t.Errorf("Hash mismatch for %s: %s vs %s", path, hash, expectedHash)
		}
	}
}
