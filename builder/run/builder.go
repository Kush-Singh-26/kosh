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
	"github.com/Kush-Singh-26/kosh/internal/build"
)

// Builder maintains the state for site builds
type Builder struct {
	cfg *config.Config

	// Services
	cacheService    services.CacheService
	postService     services.PostService
	assetService    services.AssetService
	renderService   services.RenderService
	metadataScanner services.MetadataScanner

	// Legacy access if needed (or for SaveCaches/Close)
	diagramAdapter *cache.DiagramCacheAdapter

	// Structured logging
	logger *slog.Logger

	// Build metrics tracking
	metrics *metrics.BuildMetrics

	// Filesystems
	SourceFs afero.Fs
	Sink     utils.ArtifactSink
	Tx       utils.BuildTransaction

	// Shared markdown parser pool for reuse in incremental builds
	mdPool *sync.Pool

	// Native renderer for D2/LaTeX
	nativeRenderer *native.Renderer

	// Mutex for concurrent rendering safety
	mu sync.Mutex

	// Build coordination - prevents concurrent builds during watch mode
	buildMu sync.Mutex

	// Cached data for incremental builds
	indexedPosts      []models.IndexedPost
	searchSourceDirty atomic.Bool

	// Incremental rebuild coordination
	searchDebounceTimer         *time.Timer
	lastSearchIndexRegeneration time.Time
	forceGenerators             atomic.Bool
	lastAssetHash               uint64 // Hash of asset map from last site-wide render

	// True when output directory did not exist at build start.
	isCleanBuild bool
}

func init() {
	if strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe") {
		utils.TestingMode = true
	}
}

// NewBuilder initializes a new site builder
func NewBuilder(args []string) *Builder {
	cfg := config.Load(args)
	return newBuilderWithConfig(cfg)
}

// NewBuilderFromManual creates a builder with manual service injection (for testing/benchmarks)
func NewBuilderFromManual(cfg *config.Config, render services.RenderService, asset services.AssetService, post services.PostService, meta services.MetadataScanner, logger *slog.Logger, m *metrics.BuildMetrics, sourceFs afero.Fs, mdPool *sync.Pool, nativeRenderer *native.Renderer) *Builder {
	return &Builder{
		cfg:             cfg,
		renderService:   render,
		assetService:    asset,
		postService:     post,
		metadataScanner: meta,
		logger:          logger,
		metrics:         m,
		SourceFs:        sourceFs,
		mdPool:          mdPool,
		nativeRenderer:  nativeRenderer,
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
	utils.InitMinifier()

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
	rnd.EnableLegacyProcessHTML = cfg.Build.EnableLegacyProcessHTML

	renderSvc := services.NewRenderService(rnd, logger)

	assetSvc := services.NewAssetService(vfs, nil, cfg, renderSvc, logger)
	assetSvc.SetMetrics(buildMetrics)

	var cacheSvc services.CacheService
	if cacheManager != nil {
		cacheSvc = services.NewCacheService(cacheManager, logger)
	}

	postSvc := services.NewPostService(cfg, cacheSvc, renderSvc, logger, buildMetrics, mdPool, nativeRenderer, vfs, nil, diagramAdapter)
	metadataScanner := services.NewMetadataScanner()

	b := &Builder{
		cfg:             cfg,
		cacheService:    cacheSvc,
		renderService:   renderSvc,
		assetService:    assetSvc,
		postService:     postSvc,
		metadataScanner: metadataScanner,
		diagramAdapter:  diagramAdapter,
		logger:          logger,
		metrics:         buildMetrics,
		SourceFs:        vfs,
		mdPool:          mdPool,
		nativeRenderer:  nativeRenderer,
		isCleanBuild:    isCleanBuild,
	}

	// Always force site-wide generators on the first build of a session.
	// This ensures that index.html, tags, and other generated files are
	// registered with the Sink and not deleted by CleanupOrphans in dev mode.
	b.forceGenerators.Store(true)

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

func (b *Builder) SetSink(sink utils.ArtifactSink) {
	b.Sink = sink
}

func (b *Builder) SetTx(tx utils.BuildTransaction) {
	b.Tx = tx
}

func (b *Builder) SetSourceFs(fs afero.Fs) {
	b.SourceFs = fs
	b.postService.SetSourceFs(fs)
	b.assetService.SetSourceFs(fs)
	b.renderService.SetSourceFs(fs)
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
	if b.postService != nil {
		b.postService.WaitForCacheCommit()
	}
	if b.cacheService != nil {
		// Manager implementation handles the actual DB commit
		if manager, ok := b.cacheService.(interface{ Save() error }); ok {
			_ = manager.Save()
		}
		DevLogSuccess("Saved caches")
	}
}

// Close releases build resources
func (b *Builder) Close() {
	if b.nativeRenderer != nil {
		_ = b.nativeRenderer.Close()
	}
	if b.cacheService != nil {
		_ = b.cacheService.Close()
	}
}

// checkWasmUpdate handles WASM compilation and deployment for Search.
// Skips operations in test mode to avoid filesystem dependencies.
func (b *Builder) checkWasmUpdate(ctx context.Context) {
	// Skip WASM operations in test mode
	if utils.TestingMode {
		return
	}

	wasmBinary := build.RepoPath("static", "wasm", "search.wasm")
	sourceAvailable := false
	if srcMod, err := latestSearchSourceModTime(); err == nil {
		sourceAvailable = true
		if b.searchSourceDirty.Load() {
			if err := build.CompileWASMFromSource(ctx, build.RepoPath("cmd", "search", "main.go"), wasmBinary); err != nil {
				b.logger.Warn("Failed to compile Search WASM", "error", err)
			}
			b.searchSourceDirty.Store(false)
		} else {
			wasmInfo, statErr := os.Stat(wasmBinary)
			if statErr != nil || srcMod.After(wasmInfo.ModTime()) {
				if err := build.CompileWASMFromSource(ctx, build.RepoPath("cmd", "search", "main.go"), wasmBinary); err != nil {
					b.logger.Warn("Failed to compile Search WASM", "error", err)
				}
			}
		}
	}

	// Always ensure embedded WASM is deployed if missing or old.
	// Prefer the locally compiled WASM if available so browser/runtime schema
	// always matches the current search.bin generator.
	if sourceAvailable {
		// Use the source WASM (either just rebuilt or already present)
		build.DeployWASMFromFile(afero.NewOsFs(), b.Tx.StagingDir(), b.cfg.CacheDir, wasmBinary)
	} else {
		// No source available (standard user), use embedded WASM
		build.CheckWASM(b.Tx.StagingDir(), b.cfg.CacheDir)
	}
}

func latestSearchSourceModTime() (time.Time, error) {
	paths := []string{
		build.RepoPath("cmd", "search"),
		build.RepoPath("builder", "search"),
		build.RepoPath("builder", "models"),
	}

	latest := time.Time{}
	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		})
		if err != nil {
			return time.Time{}, err
		}
	}

	if latest.IsZero() {
		return time.Time{}, os.ErrNotExist
	}
	return latest, nil
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
