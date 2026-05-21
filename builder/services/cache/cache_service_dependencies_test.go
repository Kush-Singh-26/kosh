package cache

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestCacheService_BatchCommit_WithDependencies(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	deps := &models.Dependencies{
		Templates:  []string{"layouts/post.html", "partials/header.html"},
		Taxonomies: map[string][]string{"tags": {"go", "tutorial"}},
		Includes:   []string{"partials/footer.html"},
	}

	depsMap := map[string]*models.Dependencies{
		post.ContentID: deps,
	}

	if err := service.BatchCommit([]*models.ContentMeta{post}, nil, depsMap); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	retrieved, err := service.GetItemByID(post.ContentID)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}

	if retrieved == nil {
		t.Error("Post should exist after commit")
	}
}

func TestCacheService_GetItemsByTemplate(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	posts, err := service.GetItemsByTemplate("layouts/post.html")
	if err != nil {
		t.Fatalf("GetItemsByTemplate failed: %v", err)
	}

	if len(posts) > 0 {
		t.Error("GetItemsByTemplate should return empty results for non-existent template")
	}
}
