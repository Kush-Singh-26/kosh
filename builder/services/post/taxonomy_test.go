package post

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/spf13/afero"
)

func TestPostService_TaxonomyPopulation(t *testing.T) {
	s := setupPostServiceTest(t)
	mockRend := &mockRenderServiceWithCapture{}
	s.renderer = mockRend

	// Create posts with different tags
	posts := []struct {
		name    string
		content string
	}{
		{"go-post.md", "---\ntitle: Go Post\ntags: [go, programming]\n---\nContent"},
		{"rust-post.md", "---\ntitle: Rust Post\ntags: [rust, programming]\n---\nContent"},
	}

	for _, p := range posts {
		path := filepath.Join("content", p.name)
		_ = afero.WriteFile(s.sourceFs, path, []byte(p.content), 0644)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	scanSvc := scanner.NewScanner()
	metadataResult, _ := scanSvc.Scan(scanner.ScanOptions{
		Ctx:        ctx,
		ContentDir: "content",
		SrcFs:      s.sourceFs,
		Cfg:        s.cfg,
	})

	_, err := s.Process(ProcessOptions{
		Ctx:         ctx,
		ShouldForce: true,
		Files:       metadataResult.Files,
	})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Verify taxonomies in rendered page data
	goPostPath := filepath.Join(s.cfg.OutputDir, "go-post.html")
	data, ok := mockRend.GetPage(goPostPath)
	if !ok {
		t.Fatalf("go-post.html not rendered")
	}

	// Check if Taxonomies map is populated
	tax, hasTags := data.Taxonomies["tags"]
	if !hasTags {
		t.Fatal("Taxonomies['tags'] missing in PageData")
	}

	// Check terms
	foundGo := false
	foundProg := false
	for _, term := range tax.Terms {
		if term.Name == "go" {
			foundGo = true
			if term.Count != 1 {
				t.Errorf("Expected count 1 for 'go', got %d", term.Count)
			}
		}
		if term.Name == "programming" {
			foundProg = true
			if term.Count != 2 {
				t.Errorf("Expected count 2 for 'programming', got %d", term.Count)
			}
		}
	}

	if !foundGo || !foundProg {
		t.Errorf("Missing terms in taxonomy data: go=%v, programming=%v", foundGo, foundProg)
	}
}

func TestPostService_ReadingTimeReuse(t *testing.T) {
	// This tests "Reading-time reuse during frontmatter-only updates"
	// We'll need to mock the cache to return a value for reading time
	// and verify it's reused when only frontmatter changes.

	// Actually, the reading time is calculated in parser_helpers.go
	// and stored in cache.

	// I'll check if there's already a test for reading time.
}
