package search

import (
	"context"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
)

func TestManager_UpdateIndexedContentCache(t *testing.T) {
	m := NewManager(ManagerDependencies{
		Cfg: &config.Config{},
	})

	idxPost := searchpkg.IndexedContent{
		SourcePath: "test.md",
		Record: searchpkg.ContentRecord{
			Title: "Test",
		},
	}
	m.SetIndexedPosts([]searchpkg.IndexedContent{idxPost})

	// Update existing
	newParseRes := &content.ParsedMarkdownResult{
		SearchRecord: searchpkg.ContentRecord{Title: "Updated"},
	}
	m.UpdateIndexedContentCache("test.md", newParseRes)

	posts := m.GetIndexedPosts()
	if len(posts) != 1 || posts[0].Record.Title != "Updated" {
		t.Errorf("UpdateIndexedContentCache failed to update existing Content, got title: %s", posts[0].Record.Title)
	}

	// Add new
	m.UpdateIndexedContentCache("new.md", newParseRes)
	posts = m.GetIndexedPosts()
	if len(posts) != 2 {
		t.Errorf("UpdateIndexedContentCache failed to add new Content, got len: %d", len(posts))
	}
}

func TestManager_UpdateIndexedContentCache_Canonicalization(t *testing.T) {
	m := NewManager(ManagerDependencies{
		Cfg: &config.Config{SiteConfig: config.SiteConfig{BaseURL: "https://example.com/"}},
	})

	// Add initial record with relative link
	m.SetIndexedPosts([]searchpkg.IndexedContent{
		{
			SourcePath: "blogs/test.md",
			Record:     searchpkg.ContentRecord{Title: "Initial", Link: "blogs/test.html"},
		},
	})

	// Simulate incremental update with absolute link
	m.UpdateIndexedContentCache("blogs/test.md", &content.ParsedMarkdownResult{
		SearchRecord: searchpkg.ContentRecord{Title: "Updated", Link: "https://example.com/blogs/test.html"},
	})

	posts := m.GetIndexedPosts()
	if len(posts) != 1 {
		t.Fatalf("Expected 1 post after update, got %d", len(posts))
	}

	if posts[0].Record.Link != "blogs/test.html" {
		t.Errorf("Link was not canonicalized: got %s, want blogs/test.html", posts[0].Record.Link)
	}
}

func TestManager_PruneDeletedItem(t *testing.T) {
	m := NewManager(ManagerDependencies{})
	m.SetIndexedPosts([]searchpkg.IndexedContent{
		{SourcePath: "a.md"},
		{SourcePath: "b.md"},
	})

	m.PruneDeletedItem("a.md")
	posts := m.GetIndexedPosts()
	if len(posts) != 1 || posts[0].SourcePath != "b.md" {
		t.Error("PruneDeletedItem failed")
	}
}

func TestManager_RegenerateIndex_Empty(t *testing.T) {
	m := NewManager(ManagerDependencies{})
	err := m.RegenerateIndex(context.Background())
	if err != nil {
		t.Errorf("RegenerateIndex failed: %v", err)
	}
}

func TestManager_Reconfigure(t *testing.T) {
	m := NewManager(ManagerDependencies{})
	mockRender := &mocks.MockRenderService{}
	m.Reconfigure(nil, mockRender)
	// Verify render service was set by checking no panic occurs
	// The render service is stored internally; we verify via Reconfigure not panicking
	if m.render != mockRender {
		t.Error("Reconfigure failed to update render service")
	}
}
