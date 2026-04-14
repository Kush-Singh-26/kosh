package cache

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestCacheService_BatchCommit_WithDependencies(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	deps := &cache.Dependencies{
		Templates: []string{"layouts/post.html", "partials/header.html"},
		Taxonomies:      map[string][]string{"tags": {"go", "tutorial"}},
		Includes:  []string{"partials/footer.html"},
	}

	depsMap := map[string]*cache.Dependencies{
		post.PostID: deps,
	}

	if err := service.BatchCommit([]*cache.PostMeta{post}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	retrieved, err := service.GetPostByID(post.PostID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if retrieved == nil {
		t.Error("Post should exist after commit")
	}
}

func TestCacheService_GetPostsByTemplate(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	posts, err := service.GetPostsByTemplate("layouts/post.html")
	if err != nil {
		t.Fatalf("GetPostsByTemplate failed: %v", err)
	}

	if len(posts) > 0 {
		t.Error("GetPostsByTemplate should return empty results for non-existent template")
	}
}
