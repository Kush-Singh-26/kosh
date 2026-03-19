package orchestration

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services"

	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
)

// EngineDependencies bundles service dependencies for explicit injection.
// This reduces direct coupling by grouping related services.
type EngineDependencies struct {
	Cache    services.CacheService
	Post     services.PostService
	Asset    services.AssetService
	Render   services.RenderService
	Wasm     services.WasmService
	Scanner  services.MetadataScanner
	Diagrams *cache.DiagramCacheAdapter
}

// EngineState holds build-time coordination state separate from dependencies.
type EngineState struct {
	// Build coordination - prevents concurrent builds during watch mode
	BuildMu sync.Mutex

	// Cached data for incremental builds
	IndexedPosts []models.IndexedPost

	// Incremental rebuild coordination
	BuildQueue chan BuildRequest
	// searchIndexCh is a buffered channel (capacity 1) for debounced search index regeneration.
	// Sends use non-blocking select with default case to drop pending requests when full.
	// The ProcessSearchIndexQueue goroutine drains with timer-based debouncing.
	SearchIndexCh               chan struct{}
	SearchDebounceTimer         *time.Timer
	LastSearchIndexRegeneration time.Time
	ForceGenerators             atomic.Bool
	LastAssetHash               uint64 // Hash of asset map from last site-wide render

	// True when output directory did not exist at build start.
	IsCleanBuild bool

	// Cleanup coordination
	CloseOnce sync.Once
}

// Engine maintains the state for site builds
type Engine struct {
	Cfg *config.Config
	Ctx *buildCtx.BuildContext

	// Service dependencies - injected at construction
	Deps EngineDependencies

	// Structured logging
	Logger *slog.Logger

	// Build metrics tracking
	Metrics *metrics.BuildMetrics

	// Filesystems
	SourceFs afero.Fs
	Sink     fspkg.ArtifactSink
	Tx       tx.BuildTransaction

	// Shared markdown parser pool for reuse in incremental builds
	MdPool *sync.Pool

	// Native renderer for D2/LaTeX
	NativeRenderer *native.Renderer

	// Mu mutex is deprecated for IndexedPosts access; use State.BuildMu instead.
	Mu sync.Mutex

	// Build-time state (separate from dependencies)
	State EngineState
}

// BuildRequest represents a queued build request from watch mode
type BuildRequest struct {
	Paths []string
	Op    fsnotify.Op
}

// NewEngine initializes a new site builder
func NewEngine(args []string) *Engine {
	cfg := config.Load(args)
	return newEngineWithConfig(cfg)
}

// NewEngineFromManual creates a builder with manual service injection (for testing/benchmarks)
func NewEngineFromManual(cfg *config.Config, render services.RenderService, asset services.AssetService, post services.PostService, meta services.MetadataScanner, wasm services.WasmService, logger *slog.Logger, m *metrics.BuildMetrics, sourceFs afero.Fs, mdPool *sync.Pool, nativeRenderer *native.Renderer) *Engine {
	return &Engine{
		Cfg: cfg,
		Ctx: buildCtx.NewBuildContext(true, cfg.IsDev, false, scheduler.GetGlobalScheduler(), logger),
		Deps: EngineDependencies{
			Post:    post,
			Asset:   asset,
			Render:  render,
			Wasm:    wasm,
			Scanner: meta,
		},
		Logger:         logger,
		Metrics:        m,
		SourceFs:       sourceFs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	}
}

// NewEngineWithConfig initializes a new site builder with a pre-loaded config
func NewEngineWithConfig(cfg *config.Config) *Engine {
	return newEngineWithConfig(cfg)
}

// NewEngineWithFs initializes a new site builder with a pre-loaded config and custom filesystem
func NewEngineWithFs(vfs afero.Fs, cfg *config.Config) *Engine {
	return newEngineWithConfigFs(vfs, cfg)
}

// newEngineWithConfigFs is the internal implementation with Fs
func newEngineWithConfigFs(vfs afero.Fs, cfg *config.Config) *Engine {
	fspkg.InitMinifier()

	// Tune GC for SSG throughput (memory is pooled, so we can trade space for speed)
	debug.SetGCPercent(200)

	// Initialize structured logger early
	logger := InitLogger()

	// Initialize BuildContext early to replace global state
	isTesting := buildCtx.DetectTestingMode()
	outputExists, _ := afero.Exists(vfs, cfg.OutputDir)
	isCleanBuild := !outputExists
	sched := scheduler.GetGlobalScheduler()
	ctx := buildCtx.NewBuildContext(isTesting, cfg.IsDev, isCleanBuild, sched, logger)

	// Verify Theme Exists
	VerifyThemeFs(vfs, cfg, logger, isTesting)

	// Initialize build metrics
	buildMetrics := metrics.NewBuildMetrics()

	// Create cache directory if it doesn't exist (must complete before BoltDB open)
	SetupCacheDirectoriesFs(vfs, cfg, logger, isTesting)

	// Open BoltDB cache
	cacheManager, diagramAdapter, err := SetupCacheManager(cfg, logger)
	if err != nil {
		logger.Warn("Failed to open cache database, using in-memory cache", "error", err)
	}

	// Create native renderer (Worker Pool)
	var nativeRenderer *native.Renderer
	nativeWorkers := max(runtime.NumCPU(), 4)

	if cfg.ParserWorkers > 0 {
		nativeRenderer = native.New(native.WithWorkers(cfg.ParserWorkers), native.WithScheduler(sched))
	} else {
		nativeRenderer = native.New(native.WithWorkers(nativeWorkers), native.WithScheduler(sched))
	}

	// Eagerly start KaTeX compilation in background — overlaps with
	// remaining builder setup (template parsing, service creation)
	// instead of blocking the first post that contains math.
	go nativeRenderer.EnsureInitialized(context.Background())

	diagramCache := &sync.Map{}
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	rnd := renderer.NewWithFs(vfs, cfg.CompressImages, nil, cfg.TemplateDir, cfg.IsDev, logger)

	// assetsReady is created per-build and closed when assets are ready.
	// RenderService and PostService wait on this channel but do not own its lifecycle.
	assetsReady := make(chan struct{})

	renderSvc := services.NewRenderService(services.RenderServiceDependencies{
		Ctx:      ctx,
		Renderer: rnd,
		Logger:   logger,
	})

	renderSvc.SetAssetsGate(assetsReady)

	assetSvc := services.NewAssetService(services.AssetServiceDependencies{
		Ctx:      ctx,
		SourceFs: vfs,
		Sink:     nil,
		Cfg:      cfg,
		Renderer: renderSvc,
		Logger:   logger,
		Metrics:  buildMetrics,
	}, services.WithAssetsReadySignal(assetsReady))

	var cacheSvc services.CacheService
	if cacheManager != nil {
		cacheSvc = services.NewCacheService(services.CacheServiceDependencies{
			Ctx:     ctx,
			Manager: cacheManager,
			Logger:  logger,
		})
	}

	postSvc := services.NewPostService(services.PostServiceDependencies{
		Ctx:            ctx,
		Cfg:            cfg,
		Cache:          cacheSvc,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       vfs,
		DiagramAdapter: diagramAdapter,
	})
	metadataScanner := services.NewMetadataScanner()

	wasmSvc := services.NewWasmService(services.WasmServiceDependencies{
		Ctx:    ctx,
		Cfg:    cfg,
		Logger: logger,
		Fs:     vfs,
	})

	b := &Engine{
		Cfg: cfg,
		Ctx: ctx,
		Deps: EngineDependencies{
			Cache:    cacheSvc,
			Post:     postSvc,
			Asset:    assetSvc,
			Render:   renderSvc,
			Wasm:     wasmSvc,
			Scanner:  metadataScanner,
			Diagrams: diagramAdapter,
		},
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       vfs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		State: EngineState{
			IsCleanBuild: isCleanBuild,
		},
	}

	// Always force site-wide generators on the first build of a session.
	// This ensures that index.html, tags, and other generated files are
	// registered with the Sink and not deleted by CleanupOrphans in dev mode.
	b.State.ForceGenerators.Store(true)

	// Initialize and start search index processor
	b.State.SearchIndexCh = make(chan struct{}, 1)
	go b.processSearchIndexQueue()

	// Initialize and start build queue processor
	b.State.BuildQueue = make(chan BuildRequest, 10)
	go b.processBuildQueue()

	return b
}

// newEngineWithConfig is the internal implementation
func newEngineWithConfig(cfg *config.Config) *Engine {
	return newEngineWithConfigFs(afero.NewOsFs(), cfg)
}

func (b *Engine) Config() *config.Config {
	return b.Cfg
}

func (b *Engine) SetDevMode(isDev bool) {
	config.SetDevMode(b.Cfg, isDev)
}

func (b *Engine) SetSink(sink fspkg.ArtifactSink) {
	b.Sink = sink
}

func (b *Engine) SetTx(tx tx.BuildTransaction) {
	b.Tx = tx
}

func (b *Engine) SetSourceFs(fs afero.Fs) {
	b.SourceFs = fs
	// Use consolidated reconfiguration method
	if b.Sink != nil {
		b.Deps.Post.ReconfigureForBuild(b.Sink, fs)
		b.Deps.Asset.ReconfigureForBuild(b.Sink, fs)
		b.Deps.Render.ReconfigureForBuild(b.Sink, fs)
	}
}

// getFaviconPath returns the favicon path - uses custom logo if set, otherwise defaults to theme favicon
func (b *Engine) getFaviconPath() string {
	if b.Cfg.Logo != "" {
		return b.Cfg.Logo
	}
	// Fallback to static/favicon.ico or similar in theme
	return filepath.Join(b.Cfg.StaticDir, "favicon.ico")
}

// SaveCaches waits for any background cache writes and persists BoltDB changes
func (b *Engine) SaveCaches() {
	// Wait for background cache commit goroutines before closing BoltDB
	if b.Deps.Post != nil {
		b.Deps.Post.WaitForCacheCommit()
	}
	if b.Deps.Cache != nil {
		// Manager implementation handles the actual DB commit
		if manager, ok := b.Deps.Cache.(interface{ Save() error }); ok {
			_ = manager.Save()
		}
		DevLogSuccess("Saved caches")
	}
}

// Close releases build resources
func (b *Engine) Close() {
	b.State.CloseOnce.Do(func() {
		// Close build queue processor
		if b.State.BuildQueue != nil {
			close(b.State.BuildQueue)
		}

		// Close search index processor
		if b.State.SearchIndexCh != nil {
			close(b.State.SearchIndexCh)
		}

		if b.NativeRenderer != nil {
			_ = b.NativeRenderer.Close()
		}
		if b.Deps.Cache != nil {
			_ = b.Deps.Cache.Close()
		}
	})
}

// Run executes the main build logic
func Run(args []string) error {
	b := NewEngine(args)
	defer b.Close()
	defer b.SaveCaches()
	if err := b.Build(context.Background()); err != nil {
		b.Logger.Error("Build failed", "error", err)
		return err
	}
	return nil
}
