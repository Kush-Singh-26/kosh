// Package orchestration provides integration tests for the full build pipeline.
// These tests verify end-to-end build correctness, cache utilization,
// and transaction rollback behavior.
package orchestration

import (
	"context"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	svcContent "github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

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
			ItemsPerPage: 10,
		},
		Features: models.FeaturesConfig{
			Generators: models.GeneratorsConfig{
				IsSitemapEnabled: true,
				IsRSSEnabled:     true,
				Search:           models.SearchOptionsConfig{IsEnabled: true},
				Graph:            models.GraphConfig{IsEnabled: true, ShowsTaxonomies: true},
			},
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := getSharedRenderer()
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

	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(renderer.Options{
		SourceFs:    fs,
		Compress:    false,
		Sink:        nil,
		TemplateDir: cfg.TemplateDir,
		DevMode:     true,
		Logger:      logger,
	})
	renderSvc := render.NewService(render.Dependencies{
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	metadataScanner := scanner.NewScanner()
	sink := testutil.NewMemSink()
	contentSvc := svcContent.NewService(svcContent.Dependencies{
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		Fragments:      nil,
		SourceFs:       fs,
		Sink:           sink,
	})
	tx := testutil.NewMockTransaction("public")

	b := NewEngine(WithDeps(EngineDependencies{
		Config:         cfg,
		Render:         renderSvc,
		Asset:          assetSvc,
		Content:        contentSvc,
		Scanner:        metadataScanner,
		Wasm:           wasmSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		Fragments:      nil,
		Cache:          nil,
		Diagrams:       nil,
	}))
	defer b.Close()
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()
	err = b.Build(ctx)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

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

	stats, err := cacheSvc.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TotalItems == 0 {
		t.Error("Expected posts in cache after build")
	}
}

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
			ItemsPerPage: 10,
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := getSharedRenderer()
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

	cacheManager, err := cache.Open(cacheDir, false)
	if err != nil {
		t.Fatalf("failed to open cache: %v", err)
	}
	t.Cleanup(func() { _ = cacheManager.Close() })

	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx:     buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Manager: cacheManager,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(renderer.Options{
		SourceFs:    fs,
		Compress:    false,
		Sink:        nil,
		TemplateDir: cfg.TemplateDir,
		DevMode:     true,
		Logger:      logger,
	})
	renderSvc := render.NewService(render.Dependencies{
		Ctx:      buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	metadataScanner := scanner.NewScanner()
	sink := testutil.NewMemSink()
	contentSvc := svcContent.NewService(svcContent.Dependencies{
		Ctx:            buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		Fragments:      nil,
		SourceFs:       fs,
		Sink:           sink,
	})
	tx := testutil.NewMockTransaction("public")

	b := NewEngine(WithDeps(EngineDependencies{
		Config:         cfg,
		Render:         renderSvc,
		Asset:          assetSvc,
		Content:        contentSvc,
		Scanner:        metadataScanner,
		Wasm:           wasmSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		Fragments:      nil,
		Cache:          nil,
		Diagrams:       nil,
	}))
	defer b.Close()
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()

	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}
	b.SaveCaches()

	initialStats, err := cacheSvc.Stats()
	if err != nil {
		t.Fatalf("initial Stats failed: %v", err)
	}

	sink.Files = make(map[string][]byte)
	if err := b.Build(ctx); err != nil {
		t.Fatalf("incremental build failed: %v", err)
	}

	afterStats, err := cacheSvc.Stats()
	if err != nil {
		t.Fatalf("after Stats failed: %v", err)
	}
	if afterStats.TotalItems != initialStats.TotalItems {
		t.Errorf("Expected same Content count, got %d vs %d", afterStats.TotalItems, initialStats.TotalItems)
	}
}
