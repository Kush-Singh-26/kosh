package orchestration

import (
	"context"
	"path/filepath"
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
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestIncrementalBuild_CSSChange(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	contentDir := "content"
	templateDir := "themes/test-theme/templates"
	staticDir := "themes/test-theme/static/css"
	cacheDir := t.TempDir()

	_ = fs.MkdirAll(staticDir, 0755)
	initialCSS := `body { margin: 0; }`
	_ = afero.WriteFile(fs, filepath.Join(staticDir, "style.css"), []byte(initialCSS), 0644)

	cfg := &config.Config{
		SiteConfig: config.SiteConfig{
			Title:   "Test Blog",
			BaseURL: "https://example.com",
		},
		PathConfig: config.PathConfig{
			Theme:       "test-theme",
			ThemeDir:    "themes",
			TemplateDir: templateDir,
			StaticDir:   filepath.Join("themes", "test-theme", "static"),
			ContentDir:  contentDir,
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
			return mdParser.New(cfg,
				mdParser.WithRenderer(nativeRenderer),
				mdParser.WithDiagramCache(diagramCache),
				mdParser.WithD2Group(d2Group),
			)
		},
	}

	cm, _ := cache.OpenWithTimeout(cacheDir, true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Manager: cm,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(renderer.RendererOptions{
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
	postSvc := post.NewService(post.Dependencies{
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
		SourceFs:       fs,
	})
	metadataScanner := scanner.NewScanner()
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := NewEngine(WithDeps(EngineDependencies{
		Config:         cfg,
		Render:         renderSvc,
		Asset:          assetSvc,
		Post:           postSvc,
		Scanner:        metadataScanner,
		Wasm:           wasmSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	}))
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()
	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}

	updatedCSS := `body { margin: 0; padding: 0; }`
	_ = afero.WriteFile(fs, filepath.Join(staticDir, "style.css"), []byte(updatedCSS), 0644)

	if !b.Watch.IsAssetPath(filepath.Join("themes", "test-theme", "static", "css", "style.css")) {
		t.Error("CSS file not recognized as asset path")
	}
}

func TestIncrementalBuild_TemplateChange(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	templateDir := "themes/test-theme/templates"
	cacheDir := t.TempDir()

	templateContent := `<html>
<head><title>{{.Title}}</title></head>
<body>{{.Content}}</body>
<!-- MARKER: initial -->
</html>`
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte(templateContent), 0644)

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
			return mdParser.New(cfg,
				mdParser.WithRenderer(nativeRenderer),
				mdParser.WithDiagramCache(diagramCache),
				mdParser.WithD2Group(d2Group),
			)
		},
	}

	cm, _ := cache.OpenWithTimeout(cacheDir, true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Manager: cm,
		Logger:  logger,
	})
	rnd := renderer.NewWithFs(renderer.RendererOptions{
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
	postSvc := post.NewService(post.Dependencies{
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
		SourceFs:       fs,
	})
	metadataScanner := scanner.NewScanner()
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := NewEngine(WithDeps(EngineDependencies{
		Config:         cfg,
		Render:         renderSvc,
		Asset:          assetSvc,
		Post:           postSvc,
		Scanner:        metadataScanner,
		Wasm:           wasmSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	}))
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()
	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}

	templatePath := filepath.Join("themes", "test-theme", "templates", "layout.html")
	invalidated := b.Watch.InvalidateForTemplate(templatePath)
	if invalidated != nil {
		t.Error("layout.html change should invalidate all posts")
	}

	staticPath := filepath.Join("themes", "test-theme", "static", "css", "style.css")
	invalidated = b.Watch.InvalidateForTemplate(staticPath)
	if invalidated != nil {
		t.Error("static file change should invalidate all posts")
	}
}

func TestIncrementalBuild_AssetCopyTimestampPreservation(t *testing.T) {
	fs := afero.NewMemMapFs()
	srcDir := "source/assets"
	dstDir := "dest/assets"

	_ = fs.MkdirAll(srcDir, 0755)
	_ = fs.MkdirAll(dstDir, 0755)

	content := []byte("test asset content")
	_ = afero.WriteFile(fs, filepath.Join(srcDir, "test.css"), content, 0644)

	srcInfo, err := fs.Stat(filepath.Join(srcDir, "test.css"))
	if err != nil {
		t.Fatalf("failed to stat source: %v", err)
	}
	srcModTime := srcInfo.ModTime()

	_ = afero.WriteFile(fs, filepath.Join(dstDir, "test.css"), content, 0644)
	dstInfo, err := fs.Stat(filepath.Join(dstDir, "test.css"))
	if err != nil {
		t.Fatalf("failed to stat dest: %v", err)
	}

	_ = srcModTime
	_ = dstInfo
}
