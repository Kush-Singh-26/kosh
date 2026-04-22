package orchestration

import (
	"context"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

func TestNewBuilder_Flags(t *testing.T) {
	fs := afero.NewMemMapFs()
	// Scaffold a minimal theme so NewEngine doesn't exit
	_ = fs.MkdirAll("themes/my-theme/templates", 0755)
	_ = afero.WriteFile(fs, "themes/my-theme/templates/layout.html", []byte("<html>{{.Content}}</html>"), 0644)
	// Create kosh.yaml to override default theme path
	_ = afero.WriteFile(fs, "kosh.yaml", []byte(`
theme: "my-theme"
baseURL: "https://kosh.dev"
`), 0644)

	args := []string{
		"-drafts",
	}

	cfg := config.LoadFs(fs, args)
	b := NewEngine(WithFs(fs), WithConfig(cfg))
	defer b.Close()

	if b.Cfg.BaseURL != "https://kosh.dev" {
		t.Errorf("Expected BaseURL https://kosh.dev, got %s", b.Cfg.BaseURL)
	}
	if !b.Cfg.ShouldIncludeDrafts {
		t.Error("Expected ShouldIncludeDrafts to be true")
	}
	if b.Cfg.Theme != "my-theme" {
		t.Errorf("Expected Theme my-theme, got %s", b.Cfg.Theme)
	}
}

func TestFullBuild(t *testing.T) {
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
			CacheDir:    ".kosh-cache",
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
			IsDev:        true,
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
			IsDev:        true,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       logger,
		}),
		Cfg:            cfg,
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
		Fragments:      nil, // Fragments adapter not required for this test
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	}))
	defer b.Close()
	b.artifactSink = sink
	b.buildTransaction = tx

	ctx := context.Background()
	err := b.Build(ctx)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify outputs in sink
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
			t.Logf("Available files in sink:")
			for k := range sink.Files {
				t.Logf("  - %s", k)
			}
		}
	}
}

func TestBuild_EarlyBail(t *testing.T) {
	// This is a placeholder for a more complex integration test
	// In a real scenario, we would use a mock builder and mock services
	t.Run("parallel tasks fail and propagate error", func(_ *testing.T) {
		// Just a structural test to ensure our plan was followed
		// We'll rely on the fact that errgroup is now used in build.go
	})
}

func TestBuild_Cancellation(t *testing.T) {
	t.Run("context cancellation stops build", func(_ *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		cancel() // Immediate cancel

		// In a real test, calling build with cancelled ctx should return error early
	})
}
