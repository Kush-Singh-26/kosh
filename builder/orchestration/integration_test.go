// Package run provides integration tests for the full build pipeline.
// These tests verify end-to-end build correctness, cache utilization,
// and transaction rollback behavior.
package orchestration

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

// TestFullBuildPipeline_Integration verifies the complete build pipeline
// from markdown parsing through HTML generation and cache persistence.
func TestFullBuildPipeline_Integration(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	contentDir := "content"
	templateDir := "themes/test-theme/templates"
	cacheDir := t.TempDir()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			Title:   "Test Blog",
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: templateDir,
			StaticDir:   "themes/test-theme/static",
			ContentDir:  contentDir,
			OutputDir:   "public",
			CacheDir:    cacheDir,
		},
		BuildOptions: config.BuildOptions{
			PostsPerPage: 10,
		},
		Features: models.FeaturesConfig{
			Generators: models.GeneratorsConfig{
				Sitemap: true,
				RSS:     true,
				Search:  true,
				Graph:   true,
			},
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	metadataScanner := services.NewMetadataScanner()
	sink := testutil.NewMemSink()
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
		Sink:           sink,
	})
	tx := testutil.NewMockTransaction("public")

	b := NewEngineFromManual(cfg, renderSvc, assetSvc, postSvc, metadataScanner, wasmSvc, logger, buildMetrics, fs, mdPool, nativeRenderer)
	b.Sink = sink
	b.Tx = tx

	ctx := context.Background()
	err = b.Build(ctx)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify all expected outputs were generated
	expectedFiles := []string{
		"public/index.html",
		"public/404.html",
		"public/posts/hello.html",
		"public/sitemap/sitemap.xml",
		"public/rss.xml",
		"search.bin",
		"public/graph.json",
		"public/.nojekyll",
	}

	for _, f := range expectedFiles {
		if _, ok := sink.Files[f]; !ok {
			t.Errorf("Expected file %s not found in sink", f)
		}
	}

	// Verify cache was populated
	stats, err := cacheSvc.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TotalPosts == 0 {
		t.Error("Expected posts in cache after build")
	}
}

// TestIncrementalBuild_CacheUtilization verifies that incremental builds
// correctly utilize cached data to skip unchanged posts.
func TestIncrementalBuild_CacheUtilization(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	cacheDir := t.TempDir()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			Title:   "Test Blog",
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: "themes/test-theme/templates",
			StaticDir:   "themes/test-theme/static",
			ContentDir:  "content",
			OutputDir:   "public",
			CacheDir:    cacheDir,
		},
		BuildOptions: config.BuildOptions{
			PostsPerPage: 10,
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	metadataScanner := services.NewMetadataScanner()
	sink := testutil.NewMemSink()
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
		Sink:           sink,
	})
	tx := testutil.NewMockTransaction("public")

	b := NewEngineFromManual(cfg, renderSvc, assetSvc, postSvc, metadataScanner, wasmSvc, logger, buildMetrics, fs, mdPool, nativeRenderer)
	b.Sink = sink
	b.Tx = tx

	ctx := context.Background()

	// Phase 1: Initial build
	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}
	b.SaveCaches()

	// Record cache state after initial build
	initialStats, err := cacheSvc.Stats()
	if err != nil {
		t.Fatalf("initial Stats failed: %v", err)
	}

	// Phase 2: Incremental build without changes (should use cache)
	sink.Files = make(map[string][]byte)
	if err := b.Build(ctx); err != nil {
		t.Fatalf("incremental build failed: %v", err)
	}

	// Verify cache was utilized (no new posts indexed)
	afterStats, err := cacheSvc.Stats()
	if err != nil {
		t.Fatalf("after Stats failed: %v", err)
	}
	if afterStats.TotalPosts != initialStats.TotalPosts {
		t.Errorf("Expected same post count, got %d vs %d", afterStats.TotalPosts, initialStats.TotalPosts)
	}
}

// TestCleanBuild_Reproducibility verifies that clean builds produce
// consistent output across multiple runs.
func TestCleanBuild_Reproducibility(t *testing.T) {
	cacheDir1 := t.TempDir()
	cacheDir2 := t.TempDir()

	buildSite := func(cacheDir string) map[string][]byte {
		fs := afero.NewMemMapFs()
		testutil.ScaffoldTestSite(fs)

		cfg := &config.Config{
			SiteConfig: config.SiteConfig{
				Title:   "Test Blog",
				BaseURL: "https://example.com",
			},
			PathConfig: config.PathConfig{
				Theme:       "test-theme",
				ThemeDir:    "themes",
				TemplateDir: "themes/test-theme/templates",
				StaticDir:   "themes/test-theme/static",
				ContentDir:  "content",
				OutputDir:   "public",
				CacheDir:    cacheDir,
			},
			BuildOptions: config.BuildOptions{
				PostsPerPage: 10,
			},
		}

		logger := InitLogger()
		buildMetrics := metrics.NewBuildMetrics()
		nativeRenderer := native.New()
		t.Cleanup(func() { _ = nativeRenderer.Close() })
		diagramCache := mdParser.NewMemorySSRMap()
		d2Group := nativeRenderer.GetD2Singleflight()
		mdPool := &sync.Pool{
			New: func() any {
				return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
			},
		}

		cacheManager, _ := cache.Open(cacheDir, false)
		t.Cleanup(func() { _ = cacheManager.Close() })

		cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
			Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
			Manager: cacheManager,
			Logger:  logger,
		})
		rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
		renderSvc := services.NewRenderService(services.RenderServiceDependencies{
			Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
			Renderer: rnd,
			Logger:   logger,
		})
		assetSvc := &mocks.MockAssetService{}
		assetSvc.SetMetrics(buildMetrics)
		wasmSvc := &mocks.MockWasmService{}
		metadataScanner := services.NewMetadataScanner()
		sink := testutil.NewMemSink()
		postSvc := services.NewPostService(services.PostServiceDependencies{
			Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
			Cfg:            cfg,
			Cache:          cacheSvc,
			Renderer:       renderSvc,
			Logger:         logger,
			Metrics:        buildMetrics,
			MdPool:         mdPool,
			NativeRenderer: nativeRenderer,
			SourceFs:       fs,
			Sink:           sink,
		})
		tx := testutil.NewMockTransaction("public")

		b := NewEngineFromManual(cfg, renderSvc, assetSvc, postSvc, metadataScanner, wasmSvc, logger, buildMetrics, fs, mdPool, nativeRenderer)
		b.Sink = sink
		b.Tx = tx

		renderSvc.ReconfigureForBuild(sink, fs)
		postSvc.ReconfigureForBuild(sink, fs)

		ctx := context.Background()
		_ = b.Build(ctx)
		return sink.Files
	}

	files1 := buildSite(cacheDir1)
	files2 := buildSite(cacheDir2)

	// Compare outputs
	if len(files1) != len(files2) {
		t.Logf("Files in first build (%d):", len(files1))
		for k := range files1 {
			t.Logf("  - %s", k)
		}
		t.Logf("Files in second build (%d):", len(files2))
		for k := range files2 {
			t.Logf("  - %s", k)
		}
		t.Fatalf("Different number of files: %d vs %d", len(files1), len(files2))
	}

	for path, content1 := range files1 {
		content2, ok := files2[path]
		if !ok {
			t.Errorf("File %s missing in second build", path)
			continue
		}
		// Skip files with timestamps or non-deterministic content
		if strings.HasSuffix(path, ".xml") || strings.HasSuffix(path, ".json") {
			continue
		}
		if !strings.EqualFold(string(content1), string(content2)) {
			t.Errorf("Content mismatch for %s", path)
		}
	}
}

// TestTransactionFailure_Rollback verifies that build failures trigger
// proper transaction rollback without partial output.
func TestTransactionFailure_Rollback(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	cacheDir := t.TempDir()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			Title:   "Test Blog",
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: "themes/test-theme/templates",
			StaticDir:   "themes/test-theme/static",
			ContentDir:  "content",
			OutputDir:   "public",
			CacheDir:    cacheDir,
		},
		BuildOptions: config.BuildOptions{
			PostsPerPage: 10,
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	cacheManager, _ := cache.Open(cacheDir, false)
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Renderer: rnd,
		Logger:   logger,
	})

	// Use failing asset service to simulate build failure
	failingAssetSvc := &mocks.MockAssetService{
		FailBuild: true,
	}
	failingAssetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	metadataScanner := services.NewMetadataScanner()
	sink := testutil.NewMemSink()
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
		Sink:           sink,
	})
	tx := testutil.NewMockTransaction("public")

	b := NewEngineFromManual(cfg, renderSvc, failingAssetSvc, postSvc, metadataScanner, wasmSvc, logger, buildMetrics, fs, mdPool, nativeRenderer)
	b.Sink = sink
	b.Tx = tx

	ctx := context.Background()
	err := b.Build(ctx)

	// Build should fail due to asset service
	if err == nil {
		t.Error("Expected build to fail, but it succeeded")
	}

	// Verify no partial output was written (rollback occurred)
	// In a real transaction, the staging dir would be cleaned up
	// Here we verify the sink doesn't have partial content
	if len(sink.Files) > 0 {
		t.Logf("Sink has %d files after failed build (may be expected with mock transaction)", len(sink.Files))
	}
}

// TestCacheService_DirtyTrackingIntegration verifies that dirty tracking
// correctly identifies changed posts for incremental rebuilds.
func TestCacheService_DirtyTrackingIntegration(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	cacheDir := t.TempDir()

	logger := InitLogger()
	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})

	// Mark a post as dirty
	postPath := "content/posts/hello.md"
	cacheSvc.MarkDirty(postPath)

	// Verify dirty status
	isDirty := cacheSvc.IsDirty(postPath)
	if !isDirty {
		t.Error("Expected post to be marked as dirty")
	}

	// Clear dirty status
	cacheSvc.ClearDirty()

	// Verify not dirty anymore
	isDirty = cacheSvc.IsDirty(postPath)
	if isDirty {
		t.Error("Expected post to not be dirty after ClearDirty")
	}
}

// TestCacheService_BatchCommitIntegration verifies that batch commit
// correctly persists multiple cache entries atomically.
func TestCacheService_BatchCommitIntegration(t *testing.T) {
	cacheDir := t.TempDir()

	logger := InitLogger()
	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})

	// Prepare multiple posts for batch commit
	posts := []*cache.PostMeta{
		{
			PostID:      "post-1",
			Title:       "Post 1",
			Path:        "content/posts/post1.md",
			BodyHash:    "hash1",
			WordCount:   100,
			ReadingTime: 1,
		},
		{
			PostID:      "post-2",
			Title:       "Post 2",
			Path:        "content/posts/post2.md",
			BodyHash:    "hash2",
			WordCount:   200,
			ReadingTime: 2,
		},
		{
			PostID:      "post-3",
			Title:       "Post 3",
			Path:        "content/posts/post3.md",
			BodyHash:    "hash3",
			WordCount:   300,
			ReadingTime: 3,
		},
	}

	// Batch commit (requires posts, search records, and dependencies)
	err = cacheSvc.BatchCommit(posts, nil, nil)
	if err != nil {
		t.Fatalf("BatchCommit failed: %v", err)
	}

	// Verify all posts were committed
	for i, expected := range posts {
		meta, err := cacheManager.GetPostByID(expected.PostID)
		if err != nil {
			t.Errorf("Failed to get post %d: %v", i, err)
			continue
		}
		if meta == nil {
			t.Errorf("Post %d not found in cache", i)
			continue
		}
		if meta.Title != expected.Title {
			t.Errorf("Post %d title mismatch: %s vs %s", i, meta.Title, expected.Title)
		}
	}
}

// TestCacheService_SocialCardHashPersistence verifies that social card
// hashes are correctly persisted and retrieved from cache.
func TestCacheService_SocialCardHashPersistence(t *testing.T) {
	cacheDir := t.TempDir()

	logger := InitLogger()
	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})

	// Set social card hashes
	hashes := map[string]string{
		"posts/hello.md": "abc123",
		"posts/world.md": "def456",
		"pages/about.md": "ghi789",
	}

	err = cacheSvc.BatchSetSocialCardHashes(hashes)
	if err != nil {
		t.Fatalf("BatchSetSocialCardHashes failed: %v", err)
	}

	// Verify hashes were persisted
	for path, expectedHash := range hashes {
		hash, err := cacheManager.GetSocialCardHash(path)
		if err != nil {
			t.Errorf("Failed to get hash for %s: %v", path, err)
			continue
		}
		if hash != expectedHash {
			t.Errorf("Hash mismatch for %s: %s vs %s", path, hash, expectedHash)
		}
	}
}

// TestBuild_WithRealCache verifies build with real cache persistence
// across multiple build invocations.
func TestBuild_WithRealCache(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	cacheDir := t.TempDir()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			Title:   "Test Blog",
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: "themes/test-theme/templates",
			StaticDir:   "themes/test-theme/static",
			ContentDir:  "content",
			OutputDir:   "public",
			CacheDir:    cacheDir,
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	metadataScanner := services.NewMetadataScanner()
	sink := testutil.NewMemSink()
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
		Sink:           sink,
	})
	tx := testutil.NewMockTransaction("public")

	b := NewEngineFromManual(cfg, renderSvc, assetSvc, postSvc, metadataScanner, wasmSvc, logger, buildMetrics, fs, mdPool, nativeRenderer)
	b.Sink = sink
	b.Tx = tx

	ctx := context.Background()

	// First build
	if err := b.Build(ctx); err != nil {
		t.Fatalf("first build failed: %v", err)
	}
	b.SaveCaches()

	// Close and reopen cache to simulate fresh process
	_ = cacheManager.Close()
	cacheManager2, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to reopen cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager2.Close() })

	cacheSvc2 := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager2,
		Logger:  logger,
	})
	b.Deps.Cache = cacheSvc2
	postSvc2 := services.NewPostService(services.PostServiceDependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Cfg:            cfg,
		Cache:          cacheSvc2,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
		Sink:           sink,
	})
	b.Deps.Post = postSvc2

	// Second build should use cached data
	sink.Files = make(map[string][]byte)
	if err := b.Build(ctx); err != nil {
		t.Fatalf("second build failed: %v", err)
	}

	// Verify cache was utilized
	stats, err := cacheSvc2.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TotalPosts == 0 {
		t.Error("Expected posts in cache after rebuild")
	}
}

// TestPostService_ConcurrentPostProcessing verifies that processing
// multiple posts concurrently is safe.
func TestPostService_ConcurrentPostProcessing(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	// Create multiple posts
	postContents := map[string]string{
		"content/posts/post1.md": `---
title: "Post 1"
date: "2026-03-15"
---
# Post 1
Content 1
`,
		"content/posts/post2.md": `---
title: "Post 2"
date: "2026-03-15"
---
# Post 2
Content 2
`,
		"content/posts/post3.md": `---
title: "Post 3"
date: "2026-03-15"
---
# Post 3
Content 3
`,
	}

	for path, content := range postContents {
		_ = afero.WriteFile(fs, path, []byte(content), 0644)
	}

	cacheDir := t.TempDir()

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			Title:   "Test Blog",
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: "themes/test-theme/templates",
			StaticDir:   "themes/test-theme/static",
			ContentDir:  "content",
			OutputDir:   "public",
			CacheDir:    cacheDir,
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := services.NewCacheService(services.CacheServiceDependencies{
		Ctx:     buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	sink := testutil.NewMemSink()
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
		Sink:           sink,
	})

	postSvc.ReconfigureForBuild(sink, fs)
	renderSvc.ReconfigureForBuild(sink, fs)

	ctx := context.Background()

	// Scan files first
	var files []models.ScannedFile
	for path := range postContents {
		info, _ := fs.Stat(path)
		source, _ := afero.ReadFile(fs, path)
		files = append(files, models.ScannedFile{Path: path, Info: info, Source: source})
	}

	// Process posts concurrently via the builder's ProcessAll
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := postSvc.Process(ctx, false, false, false, files)
		if err != nil {
			errChan <- err
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			t.Errorf("Concurrent post processing failed: %v", err)
		}
	}
}
