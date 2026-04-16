package search

import (
	"context"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
)

func TestManager_UpdateIndexedContentCache(t *testing.T) {
	m := NewManager(ManagerDependencies{
		Cfg: &config.Config{},
	})

	idxPost := models.IndexedContent{
		SourcePath: "test.md",
		Record: models.ContentRecord{
			Title: "Test",
		},
	}
	m.SetIndexedPosts([]models.IndexedContent{idxPost})

	// Update existing
	newParseRes := &content.ParsedMarkdownResult{
		SearchRecord: models.ContentRecord{Title: "Updated"},
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

func TestManager_PruneDeletedItem(t *testing.T) {
	m := NewManager(ManagerDependencies{})
	m.SetIndexedPosts([]models.IndexedContent{
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
