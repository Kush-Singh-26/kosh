package cache

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestCacheService_GetSearchRecord(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	record := testutil.CreateSampleSearchRecord()
	records := map[string]*cache.SearchRecord{
		post.ContentID: record,
	}

	if err := service.BatchCommit([]*cache.ContentMeta{post}, records, nil); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	retrieved, err := service.GetSearchRecord(post.ContentID)
	if err != nil {
		t.Fatalf("GetSearchRecord failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetSearchRecord should return the record")
	}

	if retrieved.Title != record.Title {
		t.Errorf("Title = %q, want %q", retrieved.Title, record.Title)
	}
}

func TestCacheService_GetSearchRecords(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post1 := testutil.CreateSamplePostMeta()
	post1.ContentID = "post-1"
	post2 := testutil.CreateSamplePostMeta()
	post2.ContentID = "post-2"

	record1 := testutil.CreateSampleSearchRecord()
	record1.Title = "Post 1"
	record2 := testutil.CreateSampleSearchRecord()
	record2.Title = "Post 2"

	records := map[string]*cache.SearchRecord{
		"post-1": record1,
		"post-2": record2,
	}

	if err := service.BatchCommit([]*cache.ContentMeta{post1, post2}, records, nil); err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	retrieved, err := service.GetSearchRecords([]string{"post-1", "post-2", "non-existent"})
	if err != nil {
		t.Fatalf("GetSearchRecords failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 records, got %d", len(retrieved))
	}

	if retrieved["post-1"] == nil {
		t.Error("Should have record for post-1")
	}

	if retrieved["post-2"] == nil {
		t.Error("Should have record for post-2")
	}
}
