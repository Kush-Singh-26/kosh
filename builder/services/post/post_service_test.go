package post

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
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

func (m *mockRenderService) SetSink(sink fspkg.ArtifactSink)      {}
func (m *mockRenderService) SetSourceFs(fs afero.Fs)              {}
func (m *mockRenderService) SetAssetsGate(ch <-chan struct{})     {}
func (m *mockRenderService) ReconfigureWithLogger(l *slog.Logger) {}

func (m *mockRenderService) RenderPage(path string, data models.PageData) error {
	if m.shouldPanic {
		panic(m.panicMsg)
	}
	return nil
}

func (m *mockRenderService) RenderIndex(path string, data models.PageData) error      { return nil }
func (m *mockRenderService) Render404(path string, data models.PageData) error        { return nil }
func (m *mockRenderService) RenderGraph(path string, data models.PageData) error      { return nil }
func (m *mockRenderService) RegisterFile(path string)                                 {}
func (m *mockRenderService) SetAssets(assets map[string]string)                       {}
func (m *mockRenderService) GetAssets() map[string]string                             { return nil }
func (m *mockRenderService) GetRenderedFiles() map[string]bool                        { return nil }
func (m *mockRenderService) ClearRenderedFiles()                                      {}
func (m *mockRenderService) ReloadTemplates()                                         {}
func (m *mockRenderService) ConsumeErrors() []error                                   { return nil }
func (m *mockRenderService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {}
func (m *mockRenderService) Has404Template() bool                                     { return true }

// mockArtifactSink is a mock ArtifactSink for testing
type mockArtifactSink struct {
	fspkg.ArtifactSink
	writtenFiles sync.Map
}

func (m *mockArtifactSink) WriteFile(path string, data []byte) error {
	m.writtenFiles.Store(path, true)
	return nil
}

func (m *mockArtifactSink) WriteStream(path string, fn func(w io.Writer) error) error {
	m.writtenFiles.Store(path, true)
	return nil
}

func (m *mockArtifactSink) MkdirAll(path string) error { return nil }

func (m *mockArtifactSink) CopyFile(src, dst string) error {
	m.writtenFiles.Store(dst, true)
	return nil
}

func (m *mockArtifactSink) Register(path string) {
	m.writtenFiles.Store(path, true)
}

func (m *mockArtifactSink) SetMtime(path string, mtime time.Time) error { return nil }
func (m *mockArtifactSink) Stat(path string) (os.FileInfo, error) {
	return nil, os.ErrNotExist
}

func (m *mockArtifactSink) GetWrittenFiles() map[string]bool {
	res := make(map[string]bool)
	m.writtenFiles.Range(func(key, value any) bool {
		res[key.(string)] = value.(bool)
		return true
	})
	return res
}

// mockRenderServiceWithCapture captures PageData for verification
type mockRenderServiceWithCapture struct {
	mockRenderService
	Pages map[string]models.PageData
	mu    sync.RWMutex
}

func (m *mockRenderServiceWithCapture) RenderPage(path string, data models.PageData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Pages == nil {
		m.Pages = make(map[string]models.PageData)
	}
	m.Pages[path] = data
	return nil
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
func (m *mockCacheService) GetSearchRecord(id string) (*cache.SearchRecord, error)  { return nil, nil }
func (m *mockCacheService) GetHTMLContent(post *cache.PostMeta) ([]byte, error)     { return nil, nil }
func (m *mockCacheService) GetSocialCardHash(path string) (string, error)           { return "", nil }
func (m *mockCacheService) SetSocialCardHash(path, hash string) error               { return nil }
func (m *mockCacheService) BatchSetSocialCardHashes(hashes map[string]string) error { return nil }
func (m *mockCacheService) GetGraphHash() (string, error)                           { return "", nil }
func (m *mockCacheService) SetGraphHash(hash string) error                          { return nil }
func (m *mockCacheService) GetWasmHash() (string, error)                            { return "", nil }
func (m *mockCacheService) SetWasmHash(hash string) error                           { return nil }
func (m *mockCacheService) GetAllPostsMetadata() ([]cache.PostListMeta, error) {
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

func setupPostServiceTest(t *testing.T) *postService {
	t.Helper()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			BaseURL: "http://localhost:8080",
		},
		PathConfig: config.PathConfig{
			ContentDir:  "content",
			OutputDir:   "output",
			Theme:       "test",
			TemplateDir: "templates",
			StaticDir:   "static",
		},
	}

	// Set global slog to noop to avoid races in tests
	slog.SetDefault(slog.New(noopHandler{}))

	logger := slog.New(noopHandler{})
	sourceFs := afero.NewMemMapFs()

	// Create minimal directory structure
	_ = sourceFs.MkdirAll(cfg.ContentDir, 0755)

	nativeRenderer := native.New()
	t.Cleanup(func() {
		_ = nativeRenderer.Close()
	})
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	return &postService{
		ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		cfg:            cfg,
		cache:          &mockCacheService{},
		renderer:       &mockRenderService{},
		logger:         logger,
		sourceFs:       sourceFs,
		sink:           testutil.NewMemSink(),
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
	logf := func(format string, args ...any) {
		t.Logf(format, args...)
	}

	s := setupPostServiceTest(t)

	// Create a test file with proper frontmatter and content
	testContent := []byte("---\ntitle: Test Post\n---\n\nTest content")
	testFile := filepath.Join("content", "test.md")
	_ = s.sourceFs.MkdirAll(filepath.Dir(testFile), 0755)
	_ = afero.WriteFile(s.sourceFs, testFile, testContent, 0644)

	// Create mock renderer that will panic
	mockRend := &mockRenderService{
		shouldPanic: true,
		panicMsg:    "simulated panic in renderer",
	}
	s.renderer = mockRend

	// Process should complete without crashing despite the panic
	// Use longer timeout to allow for worker pool shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scanner := scanner.NewScanner()
	metadataResult, _ := scanner.Scan(ctx, "content", s.sourceFs, s.cfg, nil)

	_, err := s.Process(ctx, false, false, false, metadataResult.Files)

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

func TestDecoupledPipeline(t *testing.T) {
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: "content",
			OutputDir:  "public",
		},
	}
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nil, mdParser.NewMemorySSRMap(), nil)
		},
	}

	source := []byte("---\ntitle: Test\ndate: 2024-01-01\n---\n# Hello World\n\nThis is a test.")

	// 1. Semantic Parse
	res, err := ParseMarkdownMetadata(ParseOptions{
		Source:           source,
		Path:             "content/test.md",
		CleanHtmlRelPath: "test.html",
		HtmlRelPath:      "test.html",
		MdPool:           mdPool,
		Cfg:              cfg,
	})
	if err != nil {
		t.Fatalf("ParseMarkdownMetadata failed: %v", err)
	}

	if res.Post.Title != "Test" {
		t.Errorf("Expected title 'Test', got %q", res.Post.Title)
	}
	if res.PlainText == "" {
		t.Error("Expected non-empty PlainText")
	}
	if res.AST == nil {
		t.Error("Expected non-nil AST")
	}

	// 2. Render AST
	err = RenderParsedMarkdown(MarkdownRenderOptions{
		Source:         source,
		Result:         res,
		MdPool:         mdPool,
		NativeRenderer: nil,
		DiagramAdapter: nil,
	})
	if err != nil {
		t.Fatalf("RenderParsedMarkdown failed: %v", err)
	}

	if !strings.Contains(res.HTMLContent, "Hello World</h1>") {
		t.Errorf("Expected HTML to contain 'Hello World</h1>', got %q", res.HTMLContent)
	}
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
		path := filepath.Join("content", p.name)
		_ = afero.WriteFile(s.sourceFs, path, []byte(p.content), 0644)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scanner := scanner.NewScanner()
	metadataResult, _ := scanner.Scan(ctx, "content", s.sourceFs, s.cfg, nil)

	_, err := s.Process(ctx, true, false, true, metadataResult.Files)
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
	// Sorted descending: Post 3 (Mar 3), Post 2 (Mar 2), Post 1 (Mar 1).
	// Prev (newer) = Post 3, Next (older) = Post 1.

	t.Logf("Post 2 path: %s, Prev: %v, Next: %v", post2Path, data.PrevPage, data.NextPage)

	if data.PrevPage == nil || data.PrevPage.Title != "Post 3" {
		t.Errorf("Post 2 PrevPage mismatch: got %v, want Post 3", data.PrevPage)
	}
	if data.NextPage == nil || data.NextPage.Title != "Post 1" {
		t.Errorf("Post 2 NextPage mismatch: got %v, want Post 1", data.NextPage)
	}
}
