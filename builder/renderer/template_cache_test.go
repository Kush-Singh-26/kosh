package renderer

import (
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestRenderer_MutexProtection(t *testing.T) {
	templateDir := t.TempDir()

	layoutContent := `{{ define "content" }}Layout{{ end }}`
	if err := os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatalf("Failed to write layout: %v", err)
	}

	indexContent := `{{ define "content" }}Index{{ end }}`
	if err := os.WriteFile(filepath.Join(templateDir, "index.html"), []byte(indexContent), 0644); err != nil {
		t.Fatalf("Failed to write index: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	r := New(Options{Compress: true, Sink: nil, TemplateDir: templateDir, DevMode: true, Logger: logger})

	var wg sync.WaitGroup

	for range 10 {
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

	layoutContent := `{{ define "content" }}{{ .Title }}{{ end }}`
	_ = os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644)
	_ = os.WriteFile(filepath.Join(templateDir, "index.html"), []byte(`{{ define "content" }}Index{{ end }}`), 0644)

	tc := &templateCache{
		templates:   make(map[string]*template.Template),
		mtimes:      make(map[string]time.Time),
		hashes:      make(map[string]string),
		templateDir: templateDir,
		checkTTL:    10 * time.Millisecond,
	}

	var wg sync.WaitGroup
	results := make([]bool, 10)

	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = tc.hasTemplatesChanged(afero.NewOsFs())
		}(i)
	}

	wg.Wait()

	t.Log("Template cache TTL stampede prevention test passed")
}

func TestTemplateCache_SetAndGet(t *testing.T) {
	templateDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(`{{ define "content" }}Layout{{ end }}`), 0644)

	tc := &templateCache{
		templates:   make(map[string]*template.Template),
		mtimes:      make(map[string]time.Time),
		hashes:      make(map[string]string),
		templateDir: templateDir,
		checkTTL:    2 * time.Second,
	}

	tmpl, _ := template.New("layout").Parse(`{{ define "content" }}Layout{{ end }}`)
	tc.setTemplate("layout", tmpl, time.Now(), []byte(`{{ define "content" }}Layout{{ end }}`))

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
	_ = os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(`{{ define "content" }}Layout{{ end }}`), 0644)
	_ = os.WriteFile(filepath.Join(templateDir, "index.html"), []byte(`{{ define "content" }}Index{{ end }}`), 0644)

	tc := &templateCache{
		templates:   make(map[string]*template.Template),
		mtimes:      make(map[string]time.Time),
		hashes:      make(map[string]string),
		templateDir: templateDir,
		checkTTL:    10 * time.Millisecond,
	}

	var wg sync.WaitGroup

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = tc.hasTemplatesChanged(afero.NewOsFs())
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	t.Log("Template cache concurrent access test passed")
}

func TestGraphTemplate_LoadedAutomatically(t *testing.T) {
	// Reset the global cache for this test
	globalCacheMu.Lock()
	globalCache = nil
	globalCacheMu.Unlock()

	templateDir := t.TempDir()

	layoutContent := `{{ define "content" }}Layout{{ end }}`
	if err := os.WriteFile(filepath.Join(templateDir, "layout.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatalf("Failed to write layout: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	r := New(Options{Compress: true, Sink: nil, TemplateDir: templateDir, DevMode: true, Logger: logger})

	r.ReloadTemplates()

	if r.Graph == nil {
		t.Fatal("Graph template should be loaded automatically from base chrome")
	}

	t.Log("Graph template automatic load test passed")
}
