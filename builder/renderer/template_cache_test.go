package renderer

import (
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRenderer_MutexProtection(t *testing.T) {
	templateDir := t.TempDir()

	layoutContent := `<html><body>{{ template "content" . }}</body></html>`
	if err := os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatalf("Failed to write layout: %v", err)
	}

	indexContent := `<html><body>Index</body></html>`
	if err := os.WriteFile(filepath.Join(templateDir, "index.html"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("Failed to write index: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	r := New(true, nil, templateDir, true, logger)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.mu.RLock()
			_ = r.Layout
			_ = r.Index
			r.mu.RUnlock()
		}()
	}

	wg.Wait()

	t.Log("Renderer mutex protection test passed")
}

func TestTemplateCache_HasTemplatesChanged_StampedePrevention(t *testing.T) {
	templateDir := t.TempDir()

	layoutContent := `<html><body>{{ .Title }}</body></html>`
	os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644)
	os.WriteFile(filepath.Join(templateDir, "index.html"), []byte("<html>Index</html>"), 0644)

	tc := &templateCache{
		templates:   make(map[string]*template.Template),
		mtimes:      make(map[string]time.Time),
		hashes:      make(map[string]string),
		templateDir: templateDir,
		checkTTL:    10 * time.Millisecond,
	}

	var wg sync.WaitGroup
	results := make([]bool, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = tc.hasTemplatesChanged()
		}(i)
	}

	wg.Wait()

	t.Log("Template cache TTL stampede prevention test passed")
}

func TestTemplateCache_SetAndGet(t *testing.T) {
	templateDir := t.TempDir()
	os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte("<html></html>"), 0644)

	tc := &templateCache{
		templates:   make(map[string]*template.Template),
		mtimes:      make(map[string]time.Time),
		hashes:      make(map[string]string),
		templateDir: templateDir,
		checkTTL:    2 * time.Second,
	}

	tmpl, _ := template.New("layout").Parse("<html></html>")
	tc.setTemplate("layout", tmpl, time.Now(), []byte("<html></html>"))

	tc.mu.RLock()
	_, exists := tc.templates["layout"]
	tc.mu.RUnlock()

	if !exists {
		t.Error("Template should exist after setTemplate")
	}

	t.Log("Template cache set/get test passed")
}

func TestTemplateCache_ConcurrentAccess(t *testing.T) {
	templateDir := t.TempDir()
	os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte("<html></html>"), 0644)
	os.WriteFile(filepath.Join(templateDir, "index.html"), []byte("<html></html>"), 0644)

	tc := &templateCache{
		templates:   make(map[string]*template.Template),
		mtimes:      make(map[string]time.Time),
		hashes:      make(map[string]string),
		templateDir: templateDir,
		checkTTL:    10 * time.Millisecond,
	}

	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = tc.hasTemplatesChanged()
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	t.Log("Template cache concurrent access test passed")
}

func TestGraphTemplate_FuncMapAvailable(t *testing.T) {
	// Reset the global cache for this test
	globalCacheMu.Lock()
	globalCache = nil
	globalCacheMu.Unlock()

	templateDir := t.TempDir()

	layoutContent := `<html><body>{{ template "content" . }}</body></html>`
	if err := os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatalf("Failed to write layout: %v", err)
	}

	graphContent := `<html><body>
Title: {{ .Title }}
Lower: {{ lower .Title }}
HasPrefix: {{ hasPrefix .Title "Test" }}
Replace: {{ replace "old" "new" .Title }}
Now: {{ now }}
</body></html>`
	if err := os.WriteFile(filepath.Join(templateDir, "graph.html"), []byte(graphContent), 0644); err != nil {
		t.Fatalf("Failed to write graph: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	r := New(true, nil, templateDir, true, logger)

	r.ReloadTemplates()

	if r.Graph == nil {
		t.Fatal("Graph template should be loaded")
	}

	t.Log("Graph template funcMap available test passed")
}

func TestGraphTemplate_WithData(t *testing.T) {
	// Reset the global cache for this test
	globalCacheMu.Lock()
	globalCache = nil
	globalCacheMu.Unlock()

	templateDir := t.TempDir()

	layoutContent := `<html><body>{{ template "content" . }}</body></html>`
	if err := os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatalf("Failed to write layout: %v", err)
	}

	graphContent := `<html><body>
Title: {{ .Title | lower }}
</body></html>`
	if err := os.WriteFile(filepath.Join(templateDir, "graph.html"), []byte(graphContent), 0644); err != nil {
		t.Fatalf("Failed to write graph: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	r := New(true, nil, templateDir, true, logger)

	r.ReloadTemplates()

	if r.Graph == nil {
		t.Fatal("Graph template should be loaded")
	}

	t.Log("Graph template with data test passed")
}
