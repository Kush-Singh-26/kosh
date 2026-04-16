package cache

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestCacheService_StoreHTMLAndRetrieve(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	content := []byte("<html><body>Test content</body></html>")
	hash, err := service.StoreHTML(content)
	if err != nil {
		t.Fatalf("StoreHTML failed: %v", err)
	}

	if hash == "" {
		t.Error("StoreHTML should return a hash")
	}

	post := testutil.CreateSamplePostMeta()
	post.HTMLHash = hash
	post.InlineHTML = nil

	if err := service.BatchCommit([]*cache.ContentMeta{post}, nil, nil); err != nil {
		t.Fatalf("Failed to commit post: %v", err)
	}

	retrieved, err := service.GetHTMLContent(post)
	if err != nil {
		t.Fatalf("GetHTMLContent failed: %v", err)
	}

	if string(retrieved) != string(content) {
		t.Errorf("Content mismatch: got %q, want %q", retrieved, content)
	}
}

func TestCacheService_StoreHTMLForItem_Inline(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	smallContent := []byte("<p>Small content</p>")

	if err := service.StoreHTMLForItem(post, smallContent); err != nil {
		t.Fatalf("StoreHTMLForItem failed: %v", err)
	}

	if post.InlineHTML == nil {
		t.Error("Small content should be inlined")
	}

	if string(post.InlineHTML) != string(smallContent) {
		t.Errorf("InlineHTML = %q, want %q", post.InlineHTML, smallContent)
	}

	if post.HTMLHash != "" {
		t.Error("HTMLHash should be empty for inlined content")
	}
}

func TestCacheService_StoreHTMLForItem_Large(t *testing.T) {
	service, _, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	post := testutil.CreateSamplePostMeta()
	largeContent := testutil.CreateLargeHTML()

	if err := service.StoreHTMLForItem(post, largeContent); err != nil {
		t.Fatalf("StoreHTMLForItem failed: %v", err)
	}

	if post.InlineHTML != nil {
		t.Error("Large content should not be inlined")
	}

	if post.HTMLHash == "" {
		t.Error("HTMLHash should be set for large content")
	}
}
