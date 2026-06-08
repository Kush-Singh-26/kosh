// Package orchestration provides integration tests for incremental build behavior.
// These tests verify that the incremental rebuild system correctly identifies
// and processes only the changed content without full rebuilds.
package orchestration

import (
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
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestIncrementalBuild_BodyOnlyChange(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	absPath := "content/posts/hello.md"
	contentDir := "content"
	templateDir := "themes/test-theme/templates"
	cacheDir := t.TempDir()

	initialContent := `---
title: "Hello World"
date: "2026-03-15"
tags: ["test"]
---
# Hello World
This is the initial body.
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

	ctx, cancel := testCtx()
	defer cancel()
	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}
	b.SaveCaches()

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatal("initial Content output not found")
	}
	initialOutput := string(sink.Files["public/posts/hello.html"])
	if !strings.Contains(initialOutput, "This is the initial body") {
		t.Logf("initial output: %s", initialOutput)
		t.Error("initial output doesn't contain expected initial content")
	}

	updatedContent := `---
title: "Hello World"
date: "2026-03-15"
tags: ["test"]
---
# Hello World
This is the updated body.
`
	_ = afero.WriteFile(fs, absPath, []byte(updatedContent), 0644)

	sink.Files = make(map[string][]byte)

	b.Incremental.BuildSingleItem(ctx, absPath)

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatal("expected single-Content rebuild output not found")
	}

	updatedOutput := string(sink.Files["public/posts/hello.html"])
	if !strings.Contains(updatedOutput, "This is the updated body") {
		t.Logf("updated output: %s", updatedOutput)
		t.Error("incremental output doesn't contain updated content")
	}
	if strings.Contains(updatedOutput, "This is the initial body") {
		t.Error("incremental output still contains stale initial content")
	}
}

func TestIncrementalBuild_FrontmatterChange(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	absPath := "content/posts/hello.md"
	contentDir := "content"
	templateDir := "themes/test-theme/templates"
	cacheDir := t.TempDir()

	initialContent := `---
title: "Hello World"
date: "2026-03-15"
tags: ["test"]
---
# Hello World
Body content.
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

	ctx, cancel := testCtx()
	defer cancel()
	if err := b.Build(ctx); err != nil {
		t.Fatalf("initial build failed: %v", err)
	}
	b.SaveCaches()

	updatedContent := `---
title: "Updated Title"
date: "2026-03-15"
tags: ["test", "updated"]
---
# Hello World
Body content.
`
	_ = afero.WriteFile(fs, absPath, []byte(updatedContent), 0644)

	sink.Files = make(map[string][]byte)

	b.Incremental.BuildSingleItem(ctx, absPath)

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatal("expected Content rebuild output not found")
	}

	output := string(sink.Files["public/posts/hello.html"])
	if !strings.Contains(output, "Updated Title") {
		t.Error("output doesn't contain updated title")
	}
}
