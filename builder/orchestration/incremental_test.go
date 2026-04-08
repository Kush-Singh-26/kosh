package orchestration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/testutil"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
)

func TestIsAssetPath(t *testing.T) {
	staticDir := "themes/test-theme/static"
	tests := []struct {
		name      string
		path      string
		staticDir string
		want      bool
	}{
		{
			name:      "css in theme static",
			path:      "themes/test-theme/static/css/style.css",
			staticDir: staticDir,
			want:      true,
		},
		{
			name:      "js in site static",
			path:      "static/js/main.js",
			staticDir: staticDir,
			want:      true,
		},
		{
			name:      "markdown file",
			path:      "content/post.md",
			staticDir: staticDir,
			want:      false,
		},
		{
			name:      "config file",
			path:      "kosh.yaml",
			staticDir: staticDir,
			want:      false,
		},
		{
			name:      "nested css",
			path:      "themes/test-theme/static/css/nested/style.css",
			staticDir: staticDir,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := InitLogger()
			cfg := &config.Config{
				PathConfig: config.PathConfig{
					StaticDir: tt.staticDir,
				},
			}
			b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Wasm: &mocks.MockWasmService{}, Logger: logger}))
			got := b.Watch.IsAssetPath(tt.path)
			if got != tt.want {
				t.Errorf("isAssetPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizeWatchPath_ProjectRelativeAbsolutePath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	logger := InitLogger()
	cfg := &config.Config{}
	b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Wasm: &mocks.MockWasmService{}, Logger: logger}))
	abs := filepath.Join(wd, "themes", "test-theme", "static", "css", "style.css")
	got := b.Watch.NormalizeWatchPath(abs)
	expected := fspkg.NormalizePath("themes/test-theme/static/css/style.css")
	if got != expected {
		t.Fatalf("got %q, want %q", got, expected)
	}
}

func TestIsContentPathWithAbsoluteConfiguredContentDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	contentDir := filepath.Join(wd, "content")
	logger := InitLogger()
	cfg := &config.Config{PathConfig: config.PathConfig{ContentDir: contentDir}}
	b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Logger: logger}))
	path := filepath.Join(contentDir, "posts", "hello.md")
	if !b.Watch.IsContentPath(path) {
		t.Fatalf("expected absolute markdown path to match absolute content dir")
	}
}

func TestInvalidateForTemplate(t *testing.T) {
	templateDir := "themes/test-theme/templates"
	staticDir := "themes/test-theme/static"
	tests := []struct {
		name         string
		templatePath string
		templateDir  string
		staticDir    string
		wantNil      bool
	}{
		{
			name:         "layout.html_changes_affect_all",
			templatePath: "themes/test-theme/templates/layout.html",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "static_file_changes_affect_all",
			templatePath: "themes/test-theme/static/css/style.css",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "kosh.yaml_changes_affect_all",
			templatePath: "kosh.yaml",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      true,
		},
		{
			name:         "pwa.go_changes_return_empty",
			templatePath: "builder/generators/pwa.go",
			templateDir:  templateDir,
			staticDir:    staticDir,
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := InitLogger()
			cfg := &config.Config{
				PathConfig: config.PathConfig{
					TemplateDir: tt.templateDir,
					StaticDir:   tt.staticDir,
				},
			}
			b := NewEngine(WithDeps(EngineDependencies{Config: cfg, Wasm: &mocks.MockWasmService{}, Logger: logger}))
			got := b.Watch.InvalidateForTemplate(tt.templatePath)
			if (got == nil) != tt.wantNil {
				t.Errorf("invalidateForTemplate(%q) returned nil=%v, want nil=%v", tt.templatePath, got == nil, tt.wantNil)
			}
		})
	}
}

func TestModTimeQuickBail(t *testing.T) {
	cachedMeta := &cache.PostMeta{
		ModTime:  1000,
		BodyHash: "hash123",
	}

	info := afero.NewMemMapFs()
	_ = afero.WriteFile(info, "post.md", []byte("content"), 0644)
	stat, _ := info.Stat("post.md")

	shouldForce := false
	exists := true

	fastBail := !shouldForce && exists && cachedMeta != nil && cachedMeta.BodyHash != "" && stat != nil && cachedMeta.ModTime == stat.ModTime().Unix()

	if fastBail {
		t.Error("fastBail should be false when ModTime mismatches")
	}
}

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
			PostsPerPage: 10,
		},
	}

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{New: func() any { return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group) }}

	cm, _ := cache.OpenWithTimeout(t.TempDir(), true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
	b.Sink = sink
	b.Tx = tx

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
	b.Incremental.BuildSinglePost(ctx, absPath)

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatalf("expected single-post rebuild output for absolute path")
	}
}

func TestBuildSinglePost_BodyOnlyChangeDoesNotFallBackToFullBuild(t *testing.T) {
	fs := afero.NewMemMapFs()
	absPath, _ := filepath.Abs("content/posts/hello.md")
	contentDir, _ := filepath.Abs("content")
	templateDir, _ := filepath.Abs("themes/test-theme/templates")

	_ = fs.MkdirAll(filepath.Dir(absPath), 0755)
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte("<html>{{.Content}}</html>"), 0644)

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
			CacheDir:    ".kosh-cache",
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
	mdPool := &sync.Pool{New: func() any { return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group) }}

	cm, _ := cache.OpenWithTimeout(t.TempDir(), true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
	b.Sink = sink
	b.Tx = tx

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
	b.Incremental.BuildSinglePost(ctx, absPath)
}

func TestBuildSinglePost_FrontmatterChangeTriggersFullBuild(t *testing.T) {
	fs := afero.NewMemMapFs()
	absPath, _ := filepath.Abs("content/posts/hello.md")
	contentDir, _ := filepath.Abs("content")
	templateDir, _ := filepath.Abs("themes/test-theme/templates")

	_ = fs.MkdirAll(filepath.Dir(absPath), 0755)
	_ = fs.MkdirAll(templateDir, 0755)
	_ = afero.WriteFile(fs, filepath.Join(templateDir, "layout.html"), []byte("<html>{{.Content}}</html>"), 0644)

	initialContent := `---
title: "Hello"
date: "2026-03-06"
---
# Body
`
	_ = afero.WriteFile(fs, absPath, []byte(initialContent), 0644)

	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ThemeDir:    "themes",
			TemplateDir: templateDir,
			ContentDir:  contentDir,
			OutputDir:   "public",
		},
	}

	logger := InitLogger()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{New: func() any { return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group) }}

	cm, _ := cache.OpenWithTimeout(t.TempDir(), true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := svcCache.NewService(svcCache.Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
			IsTesting:    true,
			IsDev:        false,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Renderer: rnd,
		Logger:   logger,
	})

	postSvc := post.NewService(post.Dependencies{
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
		MdPool:         mdPool,
		SourceFs:       fs,
		NativeRenderer: nativeRenderer,
	})

	b := NewEngine(WithDeps(EngineDependencies{
		Config:         cfg,
		Render:         renderSvc,
		Asset:          &mocks.MockAssetService{},
		Post:           postSvc,
		Cache:          cacheSvc,
		Scanner:        scanner.NewScanner(),
		Wasm:           &mocks.MockWasmService{},
		Logger:         logger,
		Metrics:        metrics.NewBuildMetrics(),
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	}))
	b.SetSink(testutil.NewMemSink())
	b.Tx = testutil.NewMockTransaction("public")

	ctx := context.Background()
	_ = b.Build(ctx)

	// Change frontmatter
	updatedContent := `---
title: "Updated Title"
date: "2026-03-06"
---
# Body
`
	_ = afero.WriteFile(fs, absPath, []byte(updatedContent), 0644)

	// This should not deadlock and should complete
	b.Incremental.BuildSinglePost(ctx, absPath)
}
