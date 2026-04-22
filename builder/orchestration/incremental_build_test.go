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
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestIncrementalBuild(t *testing.T) {
	fs := afero.NewMemMapFs()

	absPath, _ := filepath.Abs("content/posts/hello.md")
	contentDir, _ := filepath.Abs("content")
	templateDir, _ := filepath.Abs("themes/test-theme/templates")
	cacheDir, _ := filepath.Abs(".kosh-cache")

	_ = fs.MkdirAll(filepath.Dir(absPath), 0755)
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte("<html>{{.Content}}</html>"), 0644)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "index.html"), []byte("<html>{{range .Posts}}{{.Title}}{{end}}</html>"), 0644)

	initialContent := `---
title: "Hello"
date: "2026-03-06"
tags: ["test"]
---
# Hello
Initial body.
`
	_ = afero.WriteFile(fs, absPath, []byte(initialContent), 0644)

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

	cm, _ := cache.OpenWithTimeout(t.TempDir(), true, 0)
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
	contentSvc := content.NewService(content.Dependencies{
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
	})
	metadataScanner := scanner.NewScanner()
	sink := testutil.NewMemSink()
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
	}))
	defer b.Close()
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()
	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}
	b.SaveCaches()

	updatedContent := `---
title: "Hello"
date: "2026-03-06"
tags: ["test"]
---
# Hello
Updated body.
`
	_ = afero.WriteFile(fs, absPath, []byte(updatedContent), 0644)

	sink.Files = make(map[string][]byte)
	b.Incremental.BuildSingleItem(ctx, absPath)

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatalf("expected single-Content rebuild output for absolute path")
	}
}
