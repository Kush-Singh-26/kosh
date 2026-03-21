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
	"github.com/Kush-Singh-26/kosh/builder/orchestration/assets"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/search"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/spf13/afero"
)

func init() {
	fspkg.InitMinifier()
	debug.SetGCPercent(200)
}

type buildSetup struct {
	vfs            afero.Fs
	cfg            *config.Config
	logger         *slog.Logger
	ctx            *buildCtx.BuildContext
	isCleanBuild   bool
	buildMetrics   *metrics.BuildMetrics
	cacheSvc       services.CacheService
	nativeRenderer *native.Renderer
	mdPool         *sync.Pool
	renderSvc      services.RenderService
	assetSvc       services.AssetService
	postSvc        services.PostService
	wasmSvc        services.WasmService
	metaScanner    services.MetadataScanner
	diagramAdapter *cache.DiagramCacheAdapter
}

func (s *buildSetup) initLoggerAndContext(cfg *config.Config) {
	s.cfg = cfg
	s.logger = InitLogger()
	isTesting := buildCtx.DetectTestingMode()
	outputExists, _ := afero.Exists(s.vfs, cfg.OutputDir)
	s.isCleanBuild = !outputExists
	sched := scheduler.GetGlobalScheduler()
	s.ctx = buildCtx.NewBuildContext(isTesting, cfg.IsDev, s.isCleanBuild, sched, s.logger)
	VerifyThemeFs(s.vfs, cfg, s.logger, isTesting)

	// Ensure all packages use the configured repository root
	fspkg.SetRepoRoot(cfg.KoshSourceRoot)
}

func (s *buildSetup) initDiagnostics() {
	s.buildMetrics = metrics.NewBuildMetrics()
}

func (s *buildSetup) initCache() {
	SetupCacheDirectoriesFs(s.vfs, s.cfg, s.logger, buildCtx.DetectTestingMode())
	cacheManager, diagramAdapter, err := SetupCacheManager(s.cfg, s.logger)
	if err != nil {
		s.logger.Warn("Failed to open cache database, using in-memory cache", "error", err)
	}
	if cacheManager != nil {
		s.cacheSvc = services.NewCacheService(services.CacheServiceDependencies{
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
	sched := scheduler.GetGlobalScheduler()
	s.nativeRenderer = native.New(native.WithWorkers(workers), native.WithScheduler(sched))
	go s.nativeRenderer.EnsureInitialized(context.Background())

	var ssrMap parser.SSRMap
	if s.diagramAdapter != nil {
		s.diagramAdapter.Start()
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
	rnd := renderer.NewWithFs(s.vfs, s.cfg.CompressImages, nil, s.cfg.TemplateDir, s.cfg.IsDev, s.logger)

	assetsReady := make(chan struct{})

	s.renderSvc = services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      s.ctx,
		Renderer: rnd,
		Logger:   s.logger,
	})
	s.renderSvc.SetAssetsGate(assetsReady)

	s.assetSvc = services.NewAssetService(services.AssetServiceDependencies{
		Ctx:      s.ctx,
		SourceFs: s.vfs,
		Sink:     nil,
		Cfg:      s.cfg,
		Renderer: s.renderSvc,
		Logger:   s.logger,
		Metrics:  s.buildMetrics,
	}, services.WithAssetsReadySignal(assetsReady))

	s.postSvc = services.NewPostService(services.PostServiceDependencies{
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
	})
	s.metaScanner = services.NewMetadataScanner()

	s.wasmSvc = services.NewWasmService(services.WasmServiceDependencies{
		Ctx:    s.ctx,
		Cfg:    s.cfg,
		Logger: s.logger,
		Fs:     s.vfs,
	})
}

func newEngineWithConfigFs(vfs afero.Fs, cfg *config.Config) *Engine {
	setup := &buildSetup{vfs: vfs}

	setup.initLoggerAndContext(cfg)
	setup.initDiagnostics()
	setup.initCache()
	setup.initNativeRenderer()
	setup.initServices()

	b := &Engine{
		Cfg: cfg,
		Ctx: setup.ctx,
		Deps: EngineDependencies{
			Cache:    setup.cacheSvc,
			Post:     setup.postSvc,
			Asset:    setup.assetSvc,
			Render:   setup.renderSvc,
			Wasm:     setup.wasmSvc,
			Scanner:  setup.metaScanner,
			Diagrams: setup.diagramAdapter,
		},
		Logger:         setup.logger,
		Metrics:        setup.buildMetrics,
		SourceFs:       vfs,
		MdPool:         setup.mdPool,
		NativeRenderer: setup.nativeRenderer,
		State: EngineState{
			IsCleanBuild: setup.isCleanBuild,
		},
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
	})

	b.State.ForceGenerators.Store(true)

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
