package incremental

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestManager_ResolveContentPaths(t *testing.T) {
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: "content",
		},
	}
	m := NewManager(ManagerDependencies{
		Cfg: cfg,
	})

	rel, html, clean, err := m.ResolveContentPaths("content/blog/test.md")
	if err != nil {
		t.Fatalf("ResolveContentPaths failed: %v", err)
	}

	if rel != "blog/test.md" {
		t.Errorf("expected rel blog/test.md, got %s", rel)
	}
	if html != "blog/test.html" {
		t.Errorf("expected html blog/test.html, got %s", html)
	}
	if clean != "blog/test.html" {
		t.Errorf("expected clean blog/test.html, got %s", clean)
	}
}

func TestManager_DeterminePostChange(t *testing.T) {
	mockCache := &mocks.MockCacheService{}
	m := NewManager(ManagerDependencies{
		Deps: Dependencies{
			Cache: mockCache,
		},
	})

	relPath := "test.md"

	t.Run("New Post", func(t *testing.T) {
		mockCache.GetPostByPathFn = func(_ string) (*models.PostMeta, error) {
			return nil, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeNew {
			t.Errorf("expected PostChangeNew, got %v", change)
		}
	})

	t.Run("Frontmatter Change", func(t *testing.T) {
		mockCache.GetPostByPathFn = func(_ string) (*models.PostMeta, error) {
			return &models.PostMeta{ContentHash: "f0", BodyHash: "b1"}, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeFrontmatter {
			t.Errorf("expected PostChangeFrontmatter, got %v", change)
		}
	})

	t.Run("Body Change", func(t *testing.T) {
		mockCache.GetPostByPathFn = func(_ string) (*models.PostMeta, error) {
			return &models.PostMeta{ContentHash: "f1", BodyHash: "b0"}, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeBody {
			t.Errorf("expected PostChangeBody, got %v", change)
		}
	})

	t.Run("No Change", func(t *testing.T) {
		mockCache.GetPostByPathFn = func(_ string) (*models.PostMeta, error) {
			return &models.PostMeta{ContentHash: "f1", BodyHash: "b1"}, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeNone {
			t.Errorf("expected PostChangeNone, got %v", change)
		}
	})
}
