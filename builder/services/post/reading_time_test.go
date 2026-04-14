package post

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/spf13/afero"
)

func TestPostService_ReadingTimeReuse_Explicit(t *testing.T) {
	s := setupPostServiceTest(t)
	mockRend := &mockRenderServiceWithCapture{}
	s.renderer = mockRend

	// Use a real cache for this test
	tempCacheDir := t.TempDir()
	c, err := cache.Open(tempCacheDir, false)
	if err != nil {
		t.Fatalf("Failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s.cache = c

	// 1. Initial build to populate cache
	relativePath := "test.md"
	body := "\n\nBody with some words to count."
	content := []byte("---\ntitle: Original\n---" + body)
	path := filepath.Join("content", relativePath)
	_ = afero.WriteFile(s.sourceFs, path, content, 0644)

	ctx := context.Background()
	scanSvc := scanner.NewScanner()
	res, _ := scanSvc.Scan(scanner.ScanOptions{
		Ctx:        ctx,
		ContentDir: "content",
		SrcFs:      s.sourceFs,
		Cfg:        s.cfg,
	})

	_, err = s.Process(ProcessOptions{
		Ctx:         ctx,
		ShouldForce: false,
		Files:       res.Files,
	})
	if err != nil {
		t.Fatalf("Initial process failed: %v", err)
	}

	// Capture initial reading time
	destPath := filepath.Join(s.cfg.OutputDir, "test.html")
	data, ok := mockRend.GetPage(destPath)
	if !ok {
		t.Fatal("test.html not rendered in initial build")
	}
	initialReadingTime := data.ReadingTime
	if initialReadingTime == 0 {
		t.Error("Initial reading time should be > 0")
	}

	// 2. Modify frontmatter only (title changed, body same)
	newContent := []byte("---\ntitle: Updated Title\n---" + body)
	_ = afero.WriteFile(s.sourceFs, path, newContent, 0644)

	// Clear mock renderer state for second pass
	mockRend.mu.Lock()
	delete(mockRend.Pages, destPath)
	mockRend.mu.Unlock()

	res2, _ := scanSvc.Scan(scanner.ScanOptions{
		Ctx:        ctx,
		ContentDir: "content",
		SrcFs:      s.sourceFs,
		Cfg:        s.cfg,
	})

	_, err = s.Process(ProcessOptions{
		Ctx:         ctx,
		ShouldForce: false,
		Files:       res2.Files,
	})
	if err != nil {
		t.Fatalf("Second process failed: %v", err)
	}

	data2, ok := mockRend.GetPage(destPath)
	if !ok {
		t.Fatal("test.html not rendered in second build")
	}

	if data2.ReadingTime != initialReadingTime {
		t.Errorf("Reading time mismatch: got %d, want %d (reused)", data2.ReadingTime, initialReadingTime)
	}

	if data2.Title != "Updated Title" {
		t.Errorf("Title mismatch: got %q, want %q", data2.Title, "Updated Title")
	}
}
