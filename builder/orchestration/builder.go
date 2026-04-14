package orchestration

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/minify"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/assets"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/incremental"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/search"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/asset"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/services/wasm"
	"github.com/Kush-Singh-26/kosh/builder/shortcodes"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/spf13/afero"
)

func init() {
	minify.InitHTMLMinifier()
	debug.SetGCPercent(gcPercent)
}

const (
	gcPercent        = 200
	minNativeWorkers = 4
)

type buildSetup struct {
	sourceFs       afero.Fs
	config         *config.Config
	logger         *slog.Logger
	ctx            *buildctx.BuildContext
	isCleanBuild   bool
	buildMetrics   *metrics.BuildMetrics
	cacheSvc       svcCache.Service
	nativeRenderer *native.Renderer
	mdPool         *sync.Pool
	renderSvc      render.Service
	assetSvc       asset.Service
	postSvc        post.Service
	wasmSvc        wasm.Service
	metaScanner    scanner.Scanner
	diagramAdapter *cache.DiagramCacheAdapter
	reporter       ui.Reporter
}

func (setup *buildSetup) initLoggerAndContext(config *config.Config, reporter ui.Reporter) {
	setup.config = config
	setup.reporter = reporter
	setup.logger = InitLogger(reporter)
	isTesting := fspkg.DetectTestingMode()

	outputExists, _ := afero.Exists(setup.sourceFs, config.OutputDir)
	setup.isCleanBuild = !outputExists
	buildScheduler := scheduler.NewBuildScheduler()
	setup.ctx = buildctx.NewBuildContext(buildctx.ContextOptions{
		IsTesting:    isTesting,
		IsDev:        config.IsDev,
		IsCleanBuild: setup.isCleanBuild,
		Scheduler:    buildScheduler,
		Logger:       setup.logger,
	})
	VerifyThemeFs(setup.sourceFs, config, setup.logger, isTesting)

	// Ensure all packages use the configured repository root
	fspkg.SetRepoRoot(config.KoshSourceRoot)
}

func (setup *buildSetup) initDiagnostics() {
	setup.buildMetrics = metrics.NewBuildMetrics()
}

func (setup *buildSetup) initCache() {
	SetupCacheDirectoriesFs(setup.sourceFs, setup.config, setup.logger, fspkg.DetectTestingMode())
	cacheManager, diagramAdapter, err := SetupCacheManager(setup.config, setup.logger)
	if err != nil {
		setup.logger.Warn("Failed to open cache database, using in-memory cache", "error", err)
	}
	if cacheManager != nil {
		setup.cacheSvc = svcCache.NewService(svcCache.Dependencies{
			Ctx:     setup.ctx,
			Manager: cacheManager,
			Logger:  setup.logger,
		})
	}
	setup.diagramAdapter = diagramAdapter
}

func (setup *buildSetup) initNativeRenderer() {
	nativeWorkers := max(runtime.NumCPU(), minNativeWorkers)
	workers := nativeWorkers
	if setup.config.ParserWorkers > 0 {
		workers = setup.config.ParserWorkers
	}
	buildScheduler := setup.ctx.Scheduler
	setup.nativeRenderer = native.New(native.WithWorkers(workers), native.WithScheduler(buildScheduler))
	async.FireAndForget(context.Background(), setup.logger, "native renderer warmup", func() error {
		setup.nativeRenderer.EnsureInitialized(context.Background())
		return nil
	})

	var ssrMap parser.SSRMap
	if setup.diagramAdapter != nil {
		ssrMap = setup.diagramAdapter
	} else {
		ssrMap = parser.NewMemorySSRMap()
	}

	d2Group := setup.nativeRenderer.GetD2Singleflight()
	setup.mdPool = &sync.Pool{
		// mdPool stores *parser.Parser instances for markdown parsing.
		New: func() any {
			return parser.New(setup.config,
				parser.WithRenderer(setup.nativeRenderer),
				parser.WithDiagramCache(ssrMap),
				parser.WithD2Group(d2Group),
			)
		},
	}
}

func (setup *buildSetup) initServices() {
	// assetsReady is created per-build and closed by AssetService when assets are ready.
	// RenderService receives it via SetAssetsGate and waits before rendering pages.
	// This is a one-way synchronization channel, not a bidirectional dependency.
	// AssetService owns the channel lifecycle; RenderService only waits on it.
	rendererInstance := renderer.NewWithFs(renderer.RendererOptions{
		SourceFs:    setup.sourceFs,
		Compress:    setup.config.ShouldCompressImages,
		Sink:        nil,
		TemplateDir: setup.config.TemplateDir,
		DevMode:     setup.config.IsDev,
		Logger:      setup.logger,
		Cache:       setup.cacheSvc,
	})

	assetsReady := make(chan struct{})

	setup.renderSvc = render.NewService(render.Dependencies{
		Ctx:      setup.ctx,
		Renderer: rendererInstance,
		Logger:   setup.logger,
	})
	setup.renderSvc.SetAssetsGate(assetsReady)

	setup.assetSvc = asset.NewService(asset.Dependencies{
		Ctx:      setup.ctx,
		SourceFs: setup.sourceFs,
		Sink:     nil,
		Cfg:      setup.config,
		Renderer: setup.renderSvc,
		Logger:   setup.logger,
		Metrics:  setup.buildMetrics,
		Reporter: setup.reporter,
	}, asset.WithAssetsReadySignal(assetsReady))

	themeShortcodesDir := filepath.Join(setup.config.TemplateDir, "shortcodes")
	shortcodeProc, err := shortcodes.New(setup.sourceFs, themeShortcodesDir)
	if err != nil {
		setup.logger.Warn("Failed to initialize shortcodes", "error", err)
	}

	setup.postSvc = post.NewService(post.Dependencies{
		Ctx:            setup.ctx,
		Cfg:            setup.config,
		Cache:          setup.cacheSvc,
		Renderer:       setup.renderSvc,
		Logger:         setup.logger,
		Metrics:        setup.buildMetrics,
		MdPool:         setup.mdPool,
		NativeRenderer: setup.nativeRenderer,
		SourceFs:       setup.sourceFs,
		DiagramAdapter: setup.diagramAdapter,
		Reporter:       setup.reporter,
		Shortcodes:     shortcodeProc,
	})
	setup.metaScanner = scanner.NewScanner()

	setup.wasmSvc = wasm.NewService(wasm.Dependencies{
		Ctx:      setup.ctx,
		Cfg:      setup.config,
		Logger:   setup.logger,
		SourceFs: setup.sourceFs,
	})
}

func newEngineWithConfigFs(sourceFs afero.Fs, cfg *config.Config, reporter ui.Reporter) *Engine {
	setup := &buildSetup{sourceFs: sourceFs, reporter: reporter}

	setup.initLoggerAndContext(cfg, reporter)
	setup.initDiagnostics()
	setup.initCache()
	setup.initNativeRenderer()
	setup.initServices()

	engineInstance := &Engine{
		Cfg: cfg,
		Ctx: setup.ctx,
		Deps: EngineDependencies{
			Cache:          setup.cacheSvc,
			Post:           setup.postSvc,
			Asset:          setup.assetSvc,
			Render:         setup.renderSvc,
			Wasm:           setup.wasmSvc,
			Scanner:        setup.metaScanner,
			Diagrams:       setup.diagramAdapter,
			SourceFs:       sourceFs,
			Logger:         setup.logger,
			Metrics:        setup.buildMetrics,
			MdPool:         setup.mdPool,
			NativeRenderer: setup.nativeRenderer,
			Reporter:       reporter,
		},
		State: EngineState{
			IsCleanBuild: setup.isCleanBuild,
		},
		Health: NewBuildHealthRegistry(),
	}

	engineInstance.Assets = assets.NewManager(assets.ManagerDependencies{
		Cfg:      cfg,
		Asset:    setup.assetSvc,
		Render:   setup.renderSvc,
		Logger:   setup.logger,
		Metrics:  setup.buildMetrics,
		SourceFs: sourceFs,
	})

	engineInstance.Search = search.NewManager(search.ManagerDependencies{
		Cfg:    cfg,
		Cache:  setup.cacheSvc,
		Logger: setup.logger,
		Health: engineInstance.Health,
	})

	engineInstance.State.ForceGenerators.Store(true)
	engineInstance.Incremental = incremental.NewManager(incremental.ManagerDependencies{
		Cfg:      cfg,
		Logger:   setup.logger,
		SourceFs: sourceFs,
		Deps: incremental.IncrementalDependencies{
			Cache:    setup.cacheSvc,
			Post:     setup.postSvc,
			Render:   setup.renderSvc,
			Diagrams: setup.diagramAdapter,
		},
		Builder:        engineInstance,
		Search:         engineInstance.Search,
		MdPool:         setup.mdPool,
		NativeRenderer: setup.nativeRenderer,
	})

	engineInstance.Watch = watch.New(watch.CoordinatorDependencies{
		Cfg:           cfg,
		BuildMu:       &engineInstance.State.BuildMu,
		Cache:         setup.cacheSvc,
		OnChange:      engineInstance.handleWatchChange,
		OnSearchRegen: engineInstance.handleSearchRegen,
	})
	engineInstance.Watch.Start()

	return engineInstance
}
