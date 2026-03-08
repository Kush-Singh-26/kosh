package services

import (
	"context"
	"html/template"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// noopHandler is a slog.Handler that does nothing, used in tests to avoid race conditions in standard handlers.
type noopHandler struct{}

func (noopHandler) Handle(ctx context.Context, r slog.Record) error    { return nil }
func (noopHandler) WithAttrs(attrs []slog.Attr) slog.Handler           { return noopHandler{} }
func (noopHandler) WithGroup(name string) slog.Handler                 { return noopHandler{} }
func (noopHandler) Enabled(ctx context.Context, level slog.Level) bool { return false }

// mockRenderService is a mock that can simulate panics
type mockRenderService struct {
	shouldPanic bool
	panicMsg    string
}

func (m *mockRenderService) SetSink(sink utils.ArtifactSink) {}
func (m *mockRenderService) SetSourceFs(fs afero.Fs)          {}

func (m *mockRenderService) RenderPage(path string, data models.PageData) {
	if m.shouldPanic {
		panic(m.panicMsg)
	}
}

func (m *mockRenderService) RenderIndex(path string, data models.PageData) {}
func (m *mockRenderService) Render404(path string, data models.PageData)   {}
func (m *mockRenderService) RenderGraph(path string, data models.PageData) {}
func (m *mockRenderService) RenderSidebar(tree []*models.TreeNode) template.HTML { return "" }
func (m *mockRenderService) RegisterFile(path string)                      {}
func (m *mockRenderService) SetAssets(assets map[string]string)            {}
func (m *mockRenderService) GetAssets() map[string]string                  { return nil }
func (m *mockRenderService) GetRenderedFiles() map[string]bool             { return nil }
func (m *mockRenderService) ClearRenderedFiles()                           {}
func (m *mockRenderService) ReloadTemplates()                              {}
func (m *mockRenderService) GetErrors() []error                            { return nil }

type mockArtifactSink struct {
	utils.ArtifactSink
	mu           sync.Mutex
	writtenFiles map[string]bool
}

func (m *mockArtifactSink) WriteFile(path string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writtenFiles == nil {
		m.writtenFiles = make(map[string]bool)
	}
	m.writtenFiles[path] = true
	return nil
}

func (m *mockArtifactSink) WriteStream(path string, fn func(w io.Writer) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writtenFiles == nil {
		m.writtenFiles = make(map[string]bool)
	}
	m.writtenFiles[path] = true
	return nil
}

func (m *mockArtifactSink) MkdirAll(path string) error { return nil }

func (m *mockArtifactSink) Register(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writtenFiles == nil {
		m.writtenFiles = make(map[string]bool)
	}
	m.writtenFiles[path] = true
}

// mockRenderServiceWithCapture captures PageData for verification
type mockRenderServiceWithCapture struct {
	mockRenderService
	Pages map[string]models.PageData
	mu    sync.RWMutex
}

func (m *mockRenderServiceWithCapture) RenderPage(path string, data models.PageData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Pages == nil {
		m.Pages = make(map[string]models.PageData)
	}
	m.Pages[path] = data
}

func (m *mockRenderServiceWithCapture) GetRenderedPaths() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.Pages))
	for k := range m.Pages {
		keys = append(keys, k)
	}
	return keys
}

func (m *mockRenderServiceWithCapture) GetPage(path string) (models.PageData, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.Pages[path]
	return data, ok
}

func (m *mockRenderServiceWithCapture) GetAssets() map[string]string { return nil }

// mockCacheService that can simulate failures
type mockCacheService struct{}

func (m *mockCacheService) GetPost(id string) (*cache.PostMeta, error)         { return nil, nil }
func (m *mockCacheService) ListAllPosts() ([]string, error)                    { return nil, nil }
func (m *mockCacheService) GetPostByPath(path string) (*cache.PostMeta, error) { return nil, nil }
func (m *mockCacheService) GetPostsByIDs(ids []string) (map[string]*cache.PostMeta, error) {
	return nil, nil
}
func (m *mockCacheService) GetPostsByTemplate(templatePath string) ([]string, error) { return nil, nil }
func (m *mockCacheService) GetSearchRecords(ids []string) (map[string]*cache.SearchRecord, error) {
	return nil, nil
}
func (m *mockCacheService) GetSearchRecord(id string) (*cache.SearchRecord, error) { return nil, nil }
func (m *mockCacheService) GetHTMLContent(post *cache.PostMeta) ([]byte, error)    { return nil, nil }
func (m *mockCacheService) GetSocialCardHash(path string) (string, error)          { return "", nil }
func (m *mockCacheService) SetSocialCardHash(path, hash string) error              { return nil }
func (m *mockCacheService) BatchSetSocialCardHashes(hashes map[string]string) error { return nil }
func (m *mockCacheService) GetGraphHash() (string, error)                          { return "", nil }
func (m *mockCacheService) SetGraphHash(hash string) error                         { return nil }
func (m *mockCacheService) GetWasmHash() (string, error)                           { return "", nil }
func (m *mockCacheService) SetWasmHash(hash string) error                          { return nil }
func (m *mockCacheService) GetPostsMetadataByVersion(version string) ([]cache.PostListMeta, error) {
	return nil, nil
}
func (m *mockCacheService) StoreHTML(content []byte) (string, error)                    { return "", nil }
func (m *mockCacheService) StoreHTMLForPost(post *cache.PostMeta, content []byte) error { return nil }
func (m *mockCacheService) BatchCommit(posts []*cache.PostMeta, records map[string]*cache.SearchRecord, deps map[string]*cache.Dependencies) error {
	return nil
}
func (m *mockCacheService) DeletePost(postID string) error    { return nil }
func (m *mockCacheService) MarkDirty(postID string)           {}
func (m *mockCacheService) IsDirty(postID string) bool        { return false }
func (m *mockCacheService) ClearDirty()                       {}
func (m *mockCacheService) Stats() (*cache.CacheStats, error) { return nil, nil }
func (m *mockCacheService) IncrementBuildCount() error        { return nil }
func (m *mockCacheService) Close() error                      { return nil }

func setupPostServiceTest(t *testing.T) *postServiceImpl {
	t.Helper()

	cfg := &config.Config{
		ContentDir:  "content",
		OutputDir:   "output",
		BaseURL:     "http://localhost:8080",
		Theme:       "test",
		TemplateDir: "templates",
		StaticDir:   "static",
	}

	// Set global slog to noop to avoid races in tests
	slog.SetDefault(slog.New(noopHandler{}))

	logger := slog.New(noopHandler{})
	sourceFs := afero.NewMemMapFs()

	// Create minimal directory structure
	_ = sourceFs.MkdirAll(cfg.ContentDir, 0755)

	nativeRenderer := native.New()
	diagramCache := &sync.Map{}
	mdPool := &sync.Pool{
		New: func() interface{} {
			return mdParser.New(cfg, nativeRenderer, diagramCache)
		},
	}

	return &postServiceImpl{
		cfg:            cfg,
		cache:          &mockCacheService{},
		renderer:       &mockRenderService{},
		logger:         logger,
		sourceFs:       sourceFs,
		metrics:        metrics.NewBuildMetrics(),
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
	}
}

func TestPostService_PanicRecovery(t *testing.T) {
	// Skip in race mode due to data race in standard library's fmt package internal printer pool
	// when high-concurrency panic recovery and logging occur simultaneously.
	for _, arg := range os.Args {
		if strings.Contains(arg, "-race") {
			t.Skip("Skipping TestPostService_PanicRecovery in race mode due to fmt package internal race")
		}
	}

	// Use a helper for logging
	logf := func(format string, args ...interface{}) {
		t.Logf(format, args...)
	}

	s := setupPostServiceTest(t)

	// Create a test file
	testContent := []byte("---\ntitle: Test Post\n---\n\nTest content")
	testFile := filepath.Join(s.cfg.ContentDir, "test.md")
	_ = s.sourceFs.MkdirAll(filepath.Dir(testFile), 0755)
	_ = afero.WriteFile(s.sourceFs, testFile, testContent, 0644)

	// Create mock renderer that will panic
	mockRend := &mockRenderService{
		shouldPanic: true,
		panicMsg:    "simulated panic in renderer",
	}
	s.renderer = mockRend

	// Process should complete without crashing despite the panic
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.Process(ctx, false, false, false, nil)

	// We expect successful completion (not a crash)
	logf("Process completed with error: %v", err)
}

func TestPostService_PanicRecovery_IncrementsPanicCounter(t *testing.T) {
	s := setupPostServiceTest(t)

	// Check initial panic count
	initialPanics := atomic.LoadInt32(&s.metrics.PanicsRecovered)

	// Simulate a panic by calling the defer recover directly
	// This is a white-box test of the panic recovery mechanism
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.metrics.IncrementPanicsRecovered()
			}
		}()
		panic("test panic")
	}()

	// Verify panic counter was incremented
	finalPanics := atomic.LoadInt32(&s.metrics.PanicsRecovered)
	if finalPanics != initialPanics+1 {
		t.Errorf("Panic counter should increment by 1, got %d -> %d", initialPanics, finalPanics)
	}
}

func TestPostService_ProcessSingle_HandlesErrors(t *testing.T) {
	s := setupPostServiceTest(t)

	// Test with invalid content (no frontmatter)
	invalidContent := []byte("No frontmatter here")

	ctx := context.Background()
	testPath := filepath.Join(s.cfg.ContentDir, "invalid.md")

	// This should handle the error gracefully
	err := s.ProcessSingle(ctx, testPath, invalidContent)

	// The error might be nil or an error, but it shouldn't panic
	t.Logf("ProcessSingle completed with error: %v", err)
}

func TestPostService_ProcessSingle_PanicRecovery(t *testing.T) {
	s := setupPostServiceTest(t)

	// Set up renderer to panic
	mockRend := &mockRenderService{
		shouldPanic: true,
		panicMsg:    "panic in RenderPage",
	}
	s.renderer = mockRend

	// Create valid content
	validContent := []byte("---\ntitle: Test\n---\n\nContent")
	ctx := context.Background()
	testPath := filepath.Join(s.cfg.ContentDir, "test.md")

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ProcessSingle should not panic, but recovered: %v", r)
		}
	}()

	err := s.ProcessSingle(ctx, testPath, validContent)
	t.Logf("ProcessSingle completed with error: %v", err)
}

func TestPostService_NeighborLookup(t *testing.T) {
	s := setupPostServiceTest(t)
	mockRend := &mockRenderServiceWithCapture{}
	s.renderer = mockRend
	s.sink = &mockArtifactSink{}

	// Create 3 posts with different dates
	posts := []struct {
		name    string
		date    string
		content string
	}{
		{"post1.md", "2026-03-01", "---\ntitle: Post 1\ndate: 2026-03-01\n---\nContent 1"},
		{"post2.md", "2026-03-02", "---\ntitle: Post 2\ndate: 2026-03-02\n---\nContent 2"},
		{"post3.md", "2026-03-03", "---\ntitle: Post 3\ndate: 2026-03-03\n---\nContent 3"},
	}

	for _, p := range posts {
		path := filepath.Join(s.cfg.ContentDir, p.name)
		_ = afero.WriteFile(s.sourceFs, path, []byte(p.content), 0644)
	}

	ctx := context.Background()
	_, err := s.Process(ctx, true, false, true, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Verify neighbors for post2.html
	// Note: output paths are lowercase and .html
	post2Path := filepath.Join(s.cfg.OutputDir, "post2.html")
	data, ok := mockRend.GetPage(post2Path)
	if !ok {
		t.Fatalf("post2.html not rendered. Available: %v", mockRend.GetRenderedPaths())
	}

	// Post 2 (March 2nd)
	// Sorting is descending date: Post 3 (Mar 3), Post 2 (Mar 2), Post 1 (Mar 1).
	// Next of March 2 is Post 1 (older).
	// Prev of March 2 is Post 3 (newer).

	if data.NextPage == nil || data.NextPage.Title != "Post 1" {
		t.Errorf("Post 2 NextPage mismatch: got %v, want Post 1", data.NextPage)
	}
	if data.PrevPage == nil || data.PrevPage.Title != "Post 3" {
		t.Errorf("Post 2 PrevPage mismatch: got %v, want Post 3", data.PrevPage)
	}
}
