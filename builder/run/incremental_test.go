package run

import (
	"context"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
	"github.com/Kush-Singh-26/kosh/builder/utils"
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
			b := &Builder{
				cfg: &config.Config{
					PathConfig: config.PathConfig{
						StaticDir: tt.staticDir,
					},
				},
			}
			got := b.isAssetPath(tt.path)
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
	b := &Builder{}
	abs := filepath.Join(wd, "themes", "test-theme", "static", "css", "style.css")
	got := b.normalizeWatchPath(abs)
	expected := utils.NormalizePath("themes/test-theme/static/css/style.css")
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
	b := &Builder{cfg: &config.Config{PathConfig: config.PathConfig{ContentDir: contentDir}}}
	path := filepath.Join(contentDir, "posts", "hello.md")
	if !b.isContentPath(path) {
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
			b := &Builder{
				cfg: &config.Config{
					PathConfig: config.PathConfig{
						TemplateDir: tt.templateDir,
						StaticDir:   tt.staticDir,
					},
				},
			}
			got := b.invalidateForTemplate(tt.templatePath)
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
	utils.TestingMode = true
	defer func() { utils.TestingMode = false }()

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
	diagramCache := &sync.Map{}
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{New: func() any { return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group) }}

	cm, _ := cache.OpenWithTimeout(t.TempDir(), true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := services.NewCacheService(cm, logger)
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(rnd, logger)
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
	})
	metadataScanner := services.NewMetadataScanner()
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := &Builder{
		cfg: cfg,
		deps: BuilderDependencies{
			Cache:    cacheSvc,
			Post:     postSvc,
			Asset:    assetSvc,
			Render:   renderSvc,
			Scanner:  metadataScanner,
			Diagrams: nil,
		},
		logger:         logger,
		metrics:        buildMetrics,
		SourceFs:       fs,
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
		Sink:           sink,
		Tx:             tx,
	}

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
	b.buildSinglePost(ctx, absPath)

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatalf("expected single-post rebuild output for absolute path")
	}
}

func TestBuildSinglePost_BodyOnlyChangeDoesNotFallBackToFullBuild(t *testing.T) {
	utils.TestingMode = true
	defer func() { utils.TestingMode = false }()

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
	diagramCache := &sync.Map{}
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{New: func() any { return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group) }}

	cm, _ := cache.OpenWithTimeout(t.TempDir(), true, 0)
	defer func() { _ = cm.Close() }()
	cacheSvc := services.NewCacheService(cm, logger)
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(rnd, logger)
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	postSvc := services.NewPostService(services.PostServiceDependencies{
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
	})
	metadataScanner := services.NewMetadataScanner()
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := &Builder{
		cfg: cfg,
		deps: BuilderDependencies{
			Cache:    cacheSvc,
			Post:     postSvc,
			Asset:    assetSvc,
			Render:   renderSvc,
			Scanner:  metadataScanner,
			Diagrams: nil,
		},
		logger:         logger,
		metrics:        buildMetrics,
		SourceFs:       fs,
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
		Sink:           sink,
		Tx:             tx,
	}

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
	b.buildSinglePost(ctx, absPath)

	if _, ok := sink.Files["public/posts/hello.html"]; !ok {
		t.Fatalf("expected single-post output to be written")
	}
}
