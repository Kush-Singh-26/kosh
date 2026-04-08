package search

import (
	"context"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
)

type mockHealthRegistry struct {
	docs int64
	size int64
}

func (m *mockHealthRegistry) RecordSearchStats(docs int64, size int64) {
	m.docs = docs
	m.size = size
}

func TestManager_UpdateIndexedPostCache(t *testing.T) {
	m := NewManager(ManagerDependencies{
		Cfg: &config.Config{},
	})

	idxPost := models.IndexedPost{
		SourcePath: "test.md",
		Record: models.PostRecord{
			Title: "Test",
		},
	}
	m.SetIndexedPosts([]models.IndexedPost{idxPost})

	// Update existing
	newParseRes := &post.ParsedMarkdownResult{
		SearchRecord: models.PostRecord{Title: "Updated"},
	}
	m.UpdateIndexedPostCache("test.md", newParseRes)

	posts := m.GetIndexedPosts()
	if len(posts) != 1 || posts[0].Record.Title != "Updated" {
		t.Errorf("UpdateIndexedPostCache failed to update existing post, got title: %s", posts[0].Record.Title)
	}

	// Add new
	m.UpdateIndexedPostCache("new.md", newParseRes)
	posts = m.GetIndexedPosts()
	if len(posts) != 2 {
		t.Errorf("UpdateIndexedPostCache failed to add new post, got len: %d", len(posts))
	}
}

func TestManager_PruneDeletedPost(t *testing.T) {
	m := NewManager(ManagerDependencies{})
	m.SetIndexedPosts([]models.IndexedPost{
		{SourcePath: "a.md"},
		{SourcePath: "b.md"},
	})

	m.PruneDeletedPost("a.md")
	posts := m.GetIndexedPosts()
	if len(posts) != 1 || posts[0].SourcePath != "b.md" {
		t.Error("PruneDeletedPost failed")
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
