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
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestPostService_ConcurrentPostProcessing(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

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
	rnd := renderer.NewWithFs(renderer.RendererOptions{
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
	sink := testutil.NewMemSink()
	postSvc := post.NewService(post.Dependencies{
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

	postSvc.ReconfigureForBuild(sink, fs)
	renderSvc.ReconfigureForBuild(sink, fs)

	ctx := context.Background()

	var files []models.ScannedResource
	for path := range postContents {
		info, _ := fs.Stat(path)
		source, _ := afero.ReadFile(fs, path)
		files = append(files, models.ScannedResource{Path: path, Info: info, SourceLoader: func() ([]byte, error) { return source, nil }})
	}

	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := postSvc.Process(post.ProcessOptions{
			Ctx:                ctx,
			ShouldForce:        false,
			ForceSocialRebuild: false,
			OutputMissing:      false,
			Files:              files,
		})
		if err != nil {
			errChan <- err
		}
	}()

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			t.Errorf("Concurrent post processing failed: %v", err)
		}
	}
}
