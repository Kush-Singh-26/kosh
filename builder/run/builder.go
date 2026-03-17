package run

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"

	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"

	"github.com/Kush-Singh-26/kosh/builder/utils/fs/tx"
)

// BuilderDependencies bundles service dependencies for explicit injection.
// This reduces direct coupling by grouping related services.
type BuilderDependencies struct {
	Cache    services.CacheService
	Post     services.PostService
	Asset    services.AssetService
	Render   services.RenderService
	Wasm     services.WasmService
	Scanner  services.MetadataScanner
	Diagrams *cache.DiagramCacheAdapter
}

// BuilderState holds build-time coordination state separate from dependencies.
type BuilderState struct {
	// Build coordination - prevents concurrent builds during watch mode
	buildMu sync.Mutex

	// Cached data for incremental builds
	indexedPosts []models.IndexedPost

	// Incremental rebuild coordination
	buildQueue chan buildRequest
	// searchIndexCh is a buffered channel (capacity 1) for debounced search index regeneration.
	// Sends use non-blocking select with default case to drop pending requests when full.
	// The processSearchIndexQueue goroutine drains with timer-based debouncing.
	searchIndexCh               chan struct{}
	searchDebounceTimer         *time.Timer
	lastSearchIndexRegeneration time.Time
	forceGenerators             atomic.Bool
	lastAssetHash               uint64 // Hash of asset map from last site-wide render

	// True when output directory did not exist at build start.
	isCleanBuild bool

	// Cleanup coordination
	closeOnce sync.Once
}

// Builder maintains the state for site builds
type Builder struct {
	cfg *config.Config

	// Service dependencies - injected at construction
	deps BuilderDependencies

	// Structured logging
	logger *slog.Logger

	// Build metrics tracking
	metrics *metrics.BuildMetrics

	// Filesystems
	SourceFs afero.Fs
	Sink     fspkg.ArtifactSink
	Tx       tx.BuildTransaction

	// Shared markdown parser pool for reuse in incremental builds
	mdPool *sync.Pool

	// Native renderer for D2/LaTeX
	nativeRenderer *native.Renderer

	// Mutex for concurrent rendering safety
	mu sync.Mutex

	// Build-time state (separate from dependencies)
	state BuilderState
}

// buildRequest represents a queued build request from watch mode
type buildRequest struct {
	paths []string
	op    fsnotify.Op
}

func init() {
	DetectTestingMode()
}

// DetectTestingMode inspects os.Args to determine if we are running in a test context.
// This is called automatically in init() but can also be triggered explicitly.
func DetectTestingMode() {
	if len(os.Args) > 0 {
		if strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe") {
			utils.SetTestingMode(true)
		}
	}
}

// NewBuilder initializes a new site builder
func NewBuilder(args []string) *Builder {
	cfg := config.Load(args)
	return newBuilderWithConfig(cfg)
}

// NewBuilderFromManual creates a builder with manual service injection (for testing/benchmarks)
func NewBuilderFromManual(cfg *config.Config, render services.RenderService, asset services.AssetService, post services.PostService, meta services.MetadataScanner, wasm services.WasmService, logger *slog.Logger, m *metrics.BuildMetrics, sourceFs afero.Fs, mdPool *sync.Pool, nativeRenderer *native.Renderer) *Builder {
	return &Builder{
		cfg: cfg,
		deps: BuilderDependencies{
			Post:    post,
			Asset:   asset,
			Render:  render,
			Wasm:    wasm,
			Scanner: meta,
		},
		logger:         logger,
		metrics:        m,
		SourceFs:       sourceFs,
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
	}
}

// NewBuilderWithConfig initializes a new site builder with a pre-loaded config
func NewBuilderWithConfig(cfg *config.Config) *Builder {
	return newBuilderWithConfig(cfg)
}

// NewBuilderWithFs initializes a new site builder with a pre-loaded config and custom filesystem
func NewBuilderWithFs(vfs afero.Fs, cfg *config.Config) *Builder {
	return newBuilderWithConfigFs(vfs, cfg)
}

// newBuilderWithConfigFs is the internal implementation with Fs
func newBuilderWithConfigFs(vfs afero.Fs, cfg *config.Config) *Builder {
	fspkg.InitMinifier()

	// Tune GC for SSG throughput (memory is pooled, so we can trade space for speed)
	debug.SetGCPercent(200)

	// Initialize structured logger early
	logger := InitLogger()

	// Verify Theme Exists
	VerifyThemeFs(vfs, cfg, logger)

	// Initialize build metrics
	buildMetrics := metrics.NewBuildMetrics()

	// Create cache directory if it doesn't exist (must complete before BoltDB open)
	SetupCacheDirectoriesFs(vfs, cfg, logger)

	// Open BoltDB cache
	cacheManager, diagramAdapter := SetupCacheManager(cfg, logger)

	// Create native renderer (Worker Pool)
	var nativeRenderer *native.Renderer
	nativeWorkers := max(runtime.NumCPU(), 4)

	if cfg.ParserWorkers > 0 {
		nativeRenderer = native.New(native.WithWorkers(cfg.ParserWorkers), native.WithScheduler(utils.GlobalScheduler))
	} else {
		nativeRenderer = native.New(native.WithWorkers(nativeWorkers), native.WithScheduler(utils.GlobalScheduler))
	}

	// Initialize isCleanBuild based on output existence
	outputExists, _ := afero.Exists(vfs, cfg.OutputDir)
	isCleanBuild := !outputExists

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
		Renderer: rnd,
		Logger:   logger,
	})

	renderSvc.SetAssetsGate(assetsReady)

	assetSvc := services.NewAssetService(services.AssetServiceDependencies{
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
			Manager: cacheManager,
			Logger:  logger,
		})
	}

	postSvc := services.NewPostService(services.PostServiceDependencies{
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
		Cfg:    cfg,
		Logger: logger,
		Fs:     vfs,
	})

	b := &Builder{
		cfg: cfg,
		deps: BuilderDependencies{
			Cache:    cacheSvc,
			Post:     postSvc,
			Asset:    assetSvc,
			Render:   renderSvc,
			Wasm:     wasmSvc,
			Scanner:  metadataScanner,
			Diagrams: diagramAdapter,
		},
		logger:         logger,
		metrics:        buildMetrics,
		SourceFs:       vfs,
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
		state: BuilderState{
			isCleanBuild: isCleanBuild,
		},
	}

	// Always force site-wide generators on the first build of a session.
	// This ensures that index.html, tags, and other generated files are
	// registered with the Sink and not deleted by CleanupOrphans in dev mode.
	b.state.forceGenerators.Store(true)

	// Initialize and start search index processor
	b.state.searchIndexCh = make(chan struct{}, 1)
	go b.processSearchIndexQueue()

	// Initialize and start build queue processor
	b.state.buildQueue = make(chan buildRequest, 10)
	go b.processBuildQueue()

	return b
}

// newBuilderWithConfig is the internal implementation
func newBuilderWithConfig(cfg *config.Config) *Builder {
	return newBuilderWithConfigFs(afero.NewOsFs(), cfg)
}

func (b *Builder) Config() *config.Config {
	return b.cfg
}

func (b *Builder) SetDevMode(isDev bool) {
	config.SetDevMode(b.cfg, isDev)
}

func (b *Builder) SetSink(sink fspkg.ArtifactSink) {
	b.Sink = sink
}

func (b *Builder) SetTx(tx tx.BuildTransaction) {
	b.Tx = tx
}

func (b *Builder) SetSourceFs(fs afero.Fs) {
	b.SourceFs = fs
	// Use consolidated reconfiguration method
	if b.Sink != nil {
		b.deps.Post.ReconfigureForBuild(b.Sink, fs)
		b.deps.Asset.ReconfigureForBuild(b.Sink, fs)
		b.deps.Render.ReconfigureForBuild(b.Sink, fs)
	}
}

// getFaviconPath returns the favicon path - uses custom logo if set, otherwise defaults to theme favicon
func (b *Builder) getFaviconPath() string {
	if b.cfg.Logo != "" {
		return b.cfg.Logo
	}
	// Fallback to static/favicon.ico or similar in theme
	return filepath.Join(b.cfg.StaticDir, "favicon.ico")
}

// SaveCaches waits for any background cache writes and persists BoltDB changes
func (b *Builder) SaveCaches() {
	// Wait for background cache commit goroutines before closing BoltDB
	if b.deps.Post != nil {
		b.deps.Post.WaitForCacheCommit()
	}
	if b.deps.Cache != nil {
		// Manager implementation handles the actual DB commit
		if manager, ok := b.deps.Cache.(interface{ Save() error }); ok {
			_ = manager.Save()
		}
		DevLogSuccess("Saved caches")
	}
}

// Close releases build resources
func (b *Builder) Close() {
	b.state.closeOnce.Do(func() {
		// Close build queue processor
		if b.state.buildQueue != nil {
			close(b.state.buildQueue)
		}

		// Close search index processor
		if b.state.searchIndexCh != nil {
			close(b.state.searchIndexCh)
		}

		if b.nativeRenderer != nil {
			_ = b.nativeRenderer.Close()
		}
		if b.deps.Cache != nil {
			_ = b.deps.Cache.Close()
		}
	})
}

// Run executes the main build logic
func Run(args []string) error {
	b := NewBuilder(args)
	defer b.Close()
	defer b.SaveCaches()
	if err := b.Build(context.Background()); err != nil {
		b.logger.Error("Build failed", "error", err)
		return err
	}
	return nil
}
