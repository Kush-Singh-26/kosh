package orchestration

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/Kush-Singh-26/kosh/builder/models"
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
		Ctx:     buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Manager: cacheManager,
		Logger:  logger,
	})

	postPath := "content/posts/hello.md"
	cacheSvc.MarkDirty(postPath)

	isDirty := cacheSvc.IsDirty(postPath)
	if !isDirty {
		t.Error("Expected Content to be marked as dirty")
	}

	cacheSvc.ClearDirty()

	isDirty = cacheSvc.IsDirty(postPath)
	if isDirty {
		t.Error("Expected Content to not be dirty after ClearDirty")
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
		Ctx:     buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Manager: cacheManager,
		Logger:  logger,
	})

	posts := []*models.ContentMeta{
		{
			ContentID:   "Content-1",
			Title:       "Content 1",
			Path:        "content/posts/post1.md",
			BodyHash:    "hash1",
			WordCount:   100,
			ReadingTime: 1,
		},
		{
			ContentID:   "Content-2",
			Title:       "Content 2",
			Path:        "content/posts/post2.md",
			BodyHash:    "hash2",
			WordCount:   200,
			ReadingTime: 2,
		},
		{
			ContentID:   "Content-3",
			Title:       "Content 3",
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
		meta, err := cacheManager.GetItemByID(expected.ContentID)
		if err != nil {
			t.Errorf("Failed to get Content %d: %v", i, err)
			continue
		}
		if meta == nil {
			t.Errorf("Content %d not found in cache", i)
			continue
		}
		if meta.Title != expected.Title {
			t.Errorf("Content %d title mismatch: %s vs %s", i, meta.Title, expected.Title)
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
		Ctx:     buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
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
