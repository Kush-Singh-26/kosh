package orchestration

import (
	"bytes"
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
	"github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/asset"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/services/wasm"
	"github.com/Kush-Singh-26/kosh/builder/shortcodes"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/spf13/afero"
	"github.com/yuin/goldmark"
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
	sourceFs        afero.Fs
	config          *config.Config
	logger          *slog.Logger
	ctx             *buildctx.BuildContext
	isCleanBuild    bool
	buildMetrics    *metrics.BuildMetrics
	cacheSvc        svcCache.Service
	nativeRenderer  *native.Renderer
	mdPool          *sync.Pool
	renderSvc       render.Service
	assetSvc        asset.Service
	contentSvc      content.Service
	wasmSvc         wasm.Service
	metaScanner     scanner.Scanner
	diagramAdapter  *cache.DiagramCacheAdapter
	fragmentAdapter *cache.FragmentCacheAdapter
	reporter        ui.Reporter
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
	if cacheManager != nil {
		setup.fragmentAdapter = cache.NewFragmentCacheAdapter(cacheManager)
	}
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
	rendererInstance := setup.createRenderer()
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

	shortcodeProc := setup.initShortcodes()

	setup.contentSvc = content.NewService(content.Dependencies{
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
	setup.contentSvc.SetMarkdownRenderer(setup.createMarkdownRenderer())
	setup.metaScanner = scanner.NewScanner()

	setup.wasmSvc = wasm.NewService(wasm.Dependencies{
		Ctx:      setup.ctx,
		Cfg:      setup.config,
		Logger:   setup.logger,
		SourceFs: setup.sourceFs,
	})
}

func (setup *buildSetup) createRenderer() *renderer.Renderer {
	return renderer.NewWithFs(renderer.Options{
		SourceFs:    setup.sourceFs,
		Compress:    setup.config.ShouldCompressImages,
		Minify:      setup.config.ShouldMinify,
		Sink:        nil,
		TemplateDir: setup.config.TemplateDir,
		LayoutsDir:  setup.config.LayoutsDir,
		DevMode:     setup.config.IsDev,
		Logger:      setup.logger,
		Cache:       setup.fragmentAdapter,
	})
}

func (setup *buildSetup) initShortcodes() content.ShortcodeProcessor {
	themeShortcodesDir := filepath.Join(setup.config.TemplateDir, "shortcodes")
	shortcodeProc, err := shortcodes.New(setup.sourceFs, themeShortcodesDir)
	if err != nil {
		setup.logger.Warn("Failed to initialize shortcodes", "error", err)
	}

	if shortcodeProc != nil {
		shortcodeProc.SetRenderer(setup.createMarkdownRenderer())
	}
	return shortcodeProc
}

func (setup *buildSetup) createMarkdownRenderer() func(content []byte) ([]byte, error) {
	return func(content []byte) ([]byte, error) {
		md := setup.mdPool.Get().(goldmark.Markdown)
		defer setup.mdPool.Put(md)
		var buf bytes.Buffer
		if err := md.Convert(content, &buf); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
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
			Content:        setup.contentSvc,
			Asset:          setup.assetSvc,
			Render:         setup.renderSvc,
			Wasm:           setup.wasmSvc,
			Scanner:        setup.metaScanner,
			Diagrams:       setup.diagramAdapter,
			Fragments:      setup.fragmentAdapter,
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

	engineInstance.initManagers(setup.cacheSvc, setup.contentSvc, setup.assetSvc, setup.renderSvc, setup.metaScanner, setup.diagramAdapter, setup.mdPool, setup.nativeRenderer, sourceFs)
	engineInstance.initWatch(setup.cacheSvc)

	return engineInstance
}
