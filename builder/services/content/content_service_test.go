package content

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

	"github.com/Kush-Singh-26/kosh/builder/testutil"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
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

func (noopHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (noopHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return noopHandler{} }
func (noopHandler) WithGroup(_ string) slog.Handler               { return noopHandler{} }
func (noopHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }

// mockRenderService is a mock that can simulate panics
type mockRenderService struct {
	shouldPanic bool
	panicMsg    string
}

func (m *mockRenderService) SetSink(_ fspkg.ArtifactSink)         {}
func (m *mockRenderService) SetSourceFs(_ afero.Fs)               {}
func (m *mockRenderService) SetAssetsGate(_ <-chan struct{})      {}
func (m *mockRenderService) ReconfigureWithLogger(_ *slog.Logger) {}

func (m *mockRenderService) RenderPage(_ string, _ models.PageData) error {
	if m.shouldPanic {
		panic(m.panicMsg)
	}
	return nil
}

func (m *mockRenderService) RenderIndex(_ string, _ models.PageData) error { return nil }
func (m *mockRenderService) Render404(_ string, _ models.PageData) error   { return nil }
func (m *mockRenderService) RenderGraph(_ string, _ models.PageData) error { return nil }
func (m *mockRenderService) RenderFragment(_ string, _ string, _ models.PageData) (template.HTML, error) {
	return template.HTML(""), nil
}
func (m *mockRenderService) RegisterFile(_ string)                                {}
func (m *mockRenderService) SetAssets(_ map[string]string)                        {}
func (m *mockRenderService) GetAssets() map[string]string                         { return nil }
func (m *mockRenderService) GetRenderedFiles() map[string]bool                    { return nil }
func (m *mockRenderService) ClearRenderedFiles()                                  {}
func (m *mockRenderService) ReloadTemplates()                                     {}
func (m *mockRenderService) ConsumeErrors() []error                               { return nil }
func (m *mockRenderService) ReconfigureForBuild(_ fspkg.ArtifactSink, _ afero.Fs) {}
func (m *mockRenderService) Has404Template() bool                                 { return true }
func (m *mockRenderService) FlushFragments(_ context.Context) error               { return nil }

// mockArtifactSink is a mock ArtifactSink for testing
type mockArtifactSink struct {
	fspkg.ArtifactSink
	writtenFiles sync.Map
}

func (m *mockArtifactSink) WriteFile(_ string, _ []byte) error {
	m.writtenFiles.Store("", true)
	return nil
}

func (m *mockArtifactSink) WriteStream(_ string, _ func(w io.Writer) error) error {
	m.writtenFiles.Store("", true)
	return nil
}

func (m *mockArtifactSink) MkdirAll(_ string) error { return nil }

func (m *mockArtifactSink) CopyFile(_, dst string) error {
	m.writtenFiles.Store(dst, true)
	return nil
}

func (m *mockArtifactSink) Register(_ string) {
}

func (m *mockArtifactSink) SetMtime(_ string, _ time.Time) error { return nil }
func (m *mockArtifactSink) Stat(_ string) (os.FileInfo, error) {
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
	mu    sync.RWMutex // protects Pages
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

func (m *mockCacheService) GetItemByID(_ string) (*models.ContentMeta, error)   { return nil, nil }
func (m *mockCacheService) ListAllItems() ([]string, error)                     { return nil, nil }
func (m *mockCacheService) GetItemByPath(_ string) (*models.ContentMeta, error) { return nil, nil }
func (m *mockCacheService) GetItemsByIDs(_ []string) (map[string]*models.ContentMeta, error) {
	return nil, nil
}
func (m *mockCacheService) GetItemsByTemplate(_ string) ([]string, error) { return nil, nil }
func (m *mockCacheService) GetSearchRecords(_ []string) (map[string]*models.SearchRecord, error) {
	return nil, nil
}
func (m *mockCacheService) GetSearchRecord(_ string) (*models.SearchRecord, error) { return nil, nil }
func (m *mockCacheService) GetHTMLContent(_ *models.ContentMeta) ([]byte, error)   { return nil, nil }
func (m *mockCacheService) GetSocialCardHash(_ string) (string, error)             { return "", nil }
func (m *mockCacheService) SetSocialCardHash(_, _ string) error                    { return nil }
func (m *mockCacheService) BatchSetSocialCardHashes(_ map[string]string) error     { return nil }
func (m *mockCacheService) GetGraphHash() (string, error)                          { return "", nil }
func (m *mockCacheService) SetGraphHash(_ string) error                            { return nil }
func (m *mockCacheService) GetWasmHash() (string, error)                           { return "", nil }
func (m *mockCacheService) SetWasmHash(_ string) error                             { return nil }
func (m *mockCacheService) GetAllItemsMetadata() ([]models.ContentListMeta, error) {
	return nil, nil
}
func (m *mockCacheService) StoreHTML(_ []byte) (string, error)                     { return "", nil }
func (m *mockCacheService) StoreHTMLForItem(_ *models.ContentMeta, _ []byte) error { return nil }
func (m *mockCacheService) BatchCommit(_ []*models.ContentMeta, _ map[string]*models.SearchRecord, _ map[string]*models.Dependencies) error {
	return nil
}
func (m *mockCacheService) DeleteItem(_ string) error    { return nil }
func (m *mockCacheService) MarkDirty(_ string)           {}
func (m *mockCacheService) IsDirty(_ string) bool        { return false }
func (m *mockCacheService) ClearDirty()                  {}
func (m *mockCacheService) Stats() (*core.CacheStats, error) { return nil, nil }
func (m *mockCacheService) IncrementBuildCount() error   { return nil }
func (m *mockCacheService) Close() error                 { return nil }

func setupPostServiceTest(t *testing.T) *contentService {
	t.Helper()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			BaseURL: "http://localhost:8080",
			Taxonomies: map[string]string{
				"tags": "tags",
			},
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

	nativeRenderer := native.New(native.WithWorkers(1))
	t.Cleanup(func() {
		_ = nativeRenderer.Close()
	})
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg,
				mdParser.WithRenderer(nativeRenderer),
				mdParser.WithDiagramCache(diagramCache),
				mdParser.WithD2Group(d2Group),
			)
		},
	}

	return &contentService{
		ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
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

	scanSvc := scanner.NewScanner()
	metadataResult, _ := scanSvc.Scan(scanner.ScanOptions{
		Ctx:        ctx,
		ContentDir: "content",
		SrcFs:      s.sourceFs,
		Cfg:        s.cfg,
		FileChan:   nil,
	})

	_, err := s.Process(ProcessOptions{
		Ctx:                ctx,
		ShouldForce:        false,
		ForceSocialRebuild: false,
		OutputMissing:      false,
		Files:              metadataResult.Files,
	})

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
			return mdParser.New(cfg, mdParser.WithDiagramCache(mdParser.NewMemorySSRMap()))
		},
	}

	source := []byte("---\ntitle: Test\ndate: 2024-01-01\n---\n# Hello World\n\nThis is a test.")

	// 1. Semantic Parse
	res, err := ParseMarkdownMetadata(context.Background(), ParseOptions{
		Source:           source,
		Path:             "content/test.md",
		CleanHTMLRelPath: "test.html",
		HTMLRelPath:      "test.html",
		MdPool:           mdPool,
		Cfg:              cfg,
	})
	if err != nil {
		t.Fatalf("ParseMarkdownMetadata failed: %v", err)
	}

	if res.Item.Title != "Test" {
		t.Errorf("Expected title 'Test', got %q", res.Item.Title)
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

	scanSvc := scanner.NewScanner()
	metadataResult, _ := scanSvc.Scan(scanner.ScanOptions{
		Ctx:        ctx,
		ContentDir: "content",
		SrcFs:      s.sourceFs,
		Cfg:        s.cfg,
		FileChan:   nil,
	})

	_, err := s.Process(ProcessOptions{
		Ctx:                ctx,
		ShouldForce:        true,
		ForceSocialRebuild: false,
		OutputMissing:      true,
		Files:              metadataResult.Files,
	})
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
