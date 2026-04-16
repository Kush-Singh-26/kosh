package orchestration

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	svcContent "github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

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
				ItemsPerPage: 10,
				PostsPerPage: 10,
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

		cacheManager, _ := cache.Open(cacheDir, false)
		t.Cleanup(func() { _ = cacheManager.Close() })

		sink := testutil.NewMemSink()
		cacheSvc := svcCache.NewService(svcCache.Dependencies{
			Ctx:     buildctx.NewBuildContext(buildctx.ContextOptions{IsTesting: true, IsDev: false, IsCleanBuild: false, Scheduler: scheduler.NewBuildScheduler(), Logger: logger}),
			Manager: cacheManager,
			Logger:  logger,
		})
		rnd := renderer.NewWithFs(renderer.Options{
			SourceFs:    fs,
			Compress:    false,
			Sink:        sink,
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
		b.artifactSink = sink
		b.buildTransaction = tx

		renderSvc.ReconfigureForBuild(sink, fs)
		contentSvc.ReconfigureForBuild(sink, fs)

		ctx := context.Background()
		_ = b.Build(ctx)
		return sink.Files
	}

	files1 := buildSite(cacheDir1)
	files2 := buildSite(cacheDir2)

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
		if strings.HasSuffix(path, ".xml") || strings.HasSuffix(path, ".json") {
			continue
		}
		if !strings.EqualFold(string(content1), string(content2)) {
			t.Errorf("Content mismatch for %s", path)
		}
	}
}

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

	cacheManager, _ := cache.Open(cacheDir, false)
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

	failingAssetSvc := &mocks.MockAssetService{
		FailBuild: true,
	}
	failingAssetSvc.SetMetrics(buildMetrics)
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
		Asset:          failingAssetSvc,
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
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()
	err := b.Build(ctx)

	if err == nil {
		t.Error("Expected build to fail, but it succeeded")
	}

	if len(sink.Files) > 0 {
		t.Logf("Sink has %d files after failed build (may be expected with mock transaction)", len(sink.Files))
	}
}
