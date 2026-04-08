package orchestration

import (
	"context"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
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
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/spf13/afero"
)

func init() {
	minify.InitHTMLMinifier()
	debug.SetGCPercent(200)
}

type buildSetup struct {
	vfs            afero.Fs
	cfg            *config.Config
	logger         *slog.Logger
	ctx            *buildCtx.BuildContext
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

func (s *buildSetup) initLoggerAndContext(cfg *config.Config, r ui.Reporter) {
	s.cfg = cfg
	s.reporter = r
	s.logger = InitLogger(r)
	isTesting := fspkg.DetectTestingMode()

	outputExists, _ := afero.Exists(s.vfs, cfg.OutputDir)
	s.isCleanBuild = !outputExists
	sched := scheduler.NewBuildScheduler()
	s.ctx = buildCtx.NewBuildContext(buildCtx.ContextOptions{
		IsTesting:    isTesting,
		IsDev:        cfg.IsDev,
		IsCleanBuild: s.isCleanBuild,
		Scheduler:    sched,
		Logger:       s.logger,
	})
	VerifyThemeFs(s.vfs, cfg, s.logger, isTesting)

	// Ensure all packages use the configured repository root
	fspkg.SetRepoRoot(cfg.KoshSourceRoot)
}

func (s *buildSetup) initDiagnostics() {
	s.buildMetrics = metrics.NewBuildMetrics()
}

func (s *buildSetup) initCache() {
	SetupCacheDirectoriesFs(s.vfs, s.cfg, s.logger, fspkg.DetectTestingMode())
	cacheManager, diagramAdapter, err := SetupCacheManager(s.cfg, s.logger)
	if err != nil {
		s.logger.Warn("Failed to open cache database, using in-memory cache", "error", err)
	}
	if cacheManager != nil {
		s.cacheSvc = svcCache.NewService(svcCache.Dependencies{
			Ctx:     s.ctx,
			Manager: cacheManager,
			Logger:  s.logger,
		})
	}
	s.diagramAdapter = diagramAdapter
}

func (s *buildSetup) initNativeRenderer() {
	nativeWorkers := max(runtime.NumCPU(), 4)
	workers := nativeWorkers
	if s.cfg.ParserWorkers > 0 {
		workers = s.cfg.ParserWorkers
	}
	sched := s.ctx.Scheduler
	s.nativeRenderer = native.New(native.WithWorkers(workers), native.WithScheduler(sched))
	go s.nativeRenderer.EnsureInitialized(context.Background())

	var ssrMap parser.SSRMap
	if s.diagramAdapter != nil {
		ssrMap = s.diagramAdapter
	} else {
		ssrMap = parser.NewMemorySSRMap()
	}

	d2Group := s.nativeRenderer.GetD2Singleflight()
	s.mdPool = &sync.Pool{
		New: func() any {
			return parser.New(s.cfg, s.nativeRenderer, ssrMap, d2Group)
		},
	}
}

func (s *buildSetup) initServices() {
	// assetsReady is created per-build and closed by AssetService when assets are ready.
	// RenderService receives it via SetAssetsGate and waits before rendering pages.
	// This is a one-way synchronization channel, not a bidirectional dependency.
	// AssetService owns the channel lifecycle; RenderService only waits on it.
	rnd := renderer.NewWithFs(renderer.RendererOptions{
		SourceFs:    s.vfs,
		Compress:    s.cfg.CompressImages,
		Sink:        nil,
		TemplateDir: s.cfg.TemplateDir,
		DevMode:     s.cfg.IsDev,
		Logger:      s.logger,
	})

	assetsReady := make(chan struct{})

	s.renderSvc = render.NewService(render.Dependencies{
		Ctx:      s.ctx,
		Renderer: rnd,
		Logger:   s.logger,
	})
	s.renderSvc.SetAssetsGate(assetsReady)

	s.assetSvc = asset.NewService(asset.Dependencies{
		Ctx:      s.ctx,
		SourceFs: s.vfs,
		Sink:     nil,
		Cfg:      s.cfg,
		Renderer: s.renderSvc,
		Logger:   s.logger,
		Metrics:  s.buildMetrics,
		Reporter: s.reporter,
	}, asset.WithAssetsReadySignal(assetsReady))

	s.postSvc = post.NewService(post.Dependencies{
		Ctx:            s.ctx,
		Cfg:            s.cfg,
		Cache:          s.cacheSvc,
		Renderer:       s.renderSvc,
		Logger:         s.logger,
		Metrics:        s.buildMetrics,
		MdPool:         s.mdPool,
		NativeRenderer: s.nativeRenderer,
		SourceFs:       s.vfs,
		DiagramAdapter: s.diagramAdapter,
		Reporter:       s.reporter,
	})
	s.metaScanner = scanner.NewScanner()

	s.wasmSvc = wasm.NewService(wasm.Dependencies{
		Ctx:    s.ctx,
		Cfg:    s.cfg,
		Logger: s.logger,
		Fs:     s.vfs,
	})
}

func newEngineWithConfigFs(vfs afero.Fs, cfg *config.Config, r ui.Reporter) *Engine {
	setup := &buildSetup{vfs: vfs, reporter: r}

	setup.initLoggerAndContext(cfg, r)
	setup.initDiagnostics()
	setup.initCache()
	setup.initNativeRenderer()
	setup.initServices()

	b := &Engine{
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
			SourceFs:       vfs,
			Logger:         setup.logger,
			Metrics:        setup.buildMetrics,
			MdPool:         setup.mdPool,
			NativeRenderer: setup.nativeRenderer,
			Reporter:       r,
		},
		State: EngineState{
			IsCleanBuild: setup.isCleanBuild,
		},
		Health: NewBuildHealthRegistry(),
	}

	b.Assets = assets.NewManager(assets.ManagerDependencies{
		Cfg:      cfg,
		Asset:    setup.assetSvc,
		Render:   setup.renderSvc,
		Logger:   setup.logger,
		Metrics:  setup.buildMetrics,
		SourceFs: vfs,
	})

	b.Search = search.NewManager(search.ManagerDependencies{
		Cfg:    cfg,
		Cache:  setup.cacheSvc,
		Logger: setup.logger,
		Health: b.Health,
	})

	b.State.ForceGenerators.Store(true)
	b.Incremental = incremental.NewManager(incremental.ManagerDependencies{
		Cfg:      cfg,
		Logger:   setup.logger,
		SourceFs: vfs,
		Deps: incremental.IncrementalDependencies{
			Cache:    setup.cacheSvc,
			Post:     setup.postSvc,
			Render:   setup.renderSvc,
			Diagrams: setup.diagramAdapter,
		},
		Builder:        b,
		Search:         b.Search,
		MdPool:         setup.mdPool,
		NativeRenderer: setup.nativeRenderer,
	})

	b.Watch = watch.New(watch.CoordinatorDependencies{
		Cfg:           cfg,
		BuildMu:       &b.State.BuildMu,
		Cache:         setup.cacheSvc,
		OnChange:      b.handleWatchChange,
		OnSearchRegen: func(ctx context.Context) { _ = b.Search.RegenerateIndex(ctx) },
	})
	b.Watch.Start()

	return b
}
