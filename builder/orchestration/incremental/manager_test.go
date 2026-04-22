package incremental

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
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

	t.Run("New Content", func(t *testing.T) {
		mockCache.GetItemByPathFn = func(_ string) (*models.ContentMeta, error) {
			return nil, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeNew {
			t.Errorf("expected PostChangeNew, got %v", change)
		}
	})

	t.Run("Frontmatter Change", func(t *testing.T) {
		mockCache.GetItemByPathFn = func(_ string) (*models.ContentMeta, error) {
			return &models.ContentMeta{ContentHash: "f0", BodyHash: "b1"}, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeFrontmatter {
			t.Errorf("expected PostChangeFrontmatter, got %v", change)
		}
	})

	t.Run("Body Change", func(t *testing.T) {
		mockCache.GetItemByPathFn = func(_ string) (*models.ContentMeta, error) {
			return &models.ContentMeta{ContentHash: "f1", BodyHash: "b0"}, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeBody {
			t.Errorf("expected PostChangeBody, got %v", change)
		}
	})

	t.Run("No Change", func(t *testing.T) {
		mockCache.GetItemByPathFn = func(_ string) (*models.ContentMeta, error) {
			return &models.ContentMeta{ContentHash: "f1", BodyHash: "b1"}, nil
		}
		change := m.DeterminePostChange(relPath, "f1", "b1")
		if change != PostChangeNone {
			t.Errorf("expected PostChangeNone, got %v", change)
		}
	})
}

func TestManager_RemoveDeletedOutputs_RemovesGeneratedArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "public")
	cacheDir := filepath.Join(tempDir, ".kosh-cache")
	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			OutputDir: outputDir,
			CacheDir:  cacheDir,
		},
		Features: models.FeaturesConfig{
			UseRawMarkdown: true,
		},
	}

	cacheService := mocks.NewMockCacheService()
	const (
		relPath     = "blogs/sample.md"
		htmlRelPath = "blogs/sample.html"
		cardHash    = "card-hash-123"
	)
	cacheService.SocialCardHashes[relPath] = cardHash

	m := NewManager(ManagerDependencies{
		Cfg: cfg,
		Deps: Dependencies{
			Cache: cacheService,
		},
	})

	htmlPath := filepath.Join(outputDir, htmlRelPath)
	rawPath := filepath.Join(outputDir, "blogs", "sample.md")
	_, cardOutHashed, _ := navigation.CardPaths(cfg.BaseURL, cfg.OutputDir, htmlRelPath, cardHash)
	_, cardOutPlain, _ := navigation.CardPaths(cfg.BaseURL, cfg.OutputDir, htmlRelPath, "")
	cacheCardPath := filepath.Join(cacheDir, "social-cards", cardHash+".webp")

	paths := []string{htmlPath, rawPath, cardOutHashed, cardOutPlain, cacheCardPath}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	m.removeDeletedOutputs(relPath, htmlRelPath)

	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
}
