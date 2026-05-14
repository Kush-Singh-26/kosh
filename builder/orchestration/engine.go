package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"

	assetpkg "github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/assets"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/incremental"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/search"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/asset"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/services/wasm"

	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

const (
	cacheGCPollInterval = 20
	cacheGCMaxAge       = 7 * 24 * time.Hour
)

// EngineDependencies bundles all engine construction dependencies for explicit injection.
// This reduces function signatures and makes test setup more readable.
type EngineDependencies struct {
	// Services
	Cache     svcCache.Service
	Content   content.Service
	Asset     asset.Service
	Render    render.Service
	Wasm      wasm.Service
	Scanner   scanner.Scanner
	Diagrams  *cache.DiagramCacheAdapter
	Fragments *cache.FragmentCacheAdapter
	Reporter  ui.Reporter

	// Config & runtime
	Config         *config.Config
	SourceFs       afero.Fs
	Logger         *slog.Logger
	Metrics        *metrics.BuildMetrics
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
}

// EngineState holds build-time coordination state separate from dependencies.
type EngineState struct {
	// Build coordination - prevents concurrent builds during watch mode
	BuildMu sync.Mutex

	ForceGenerators atomic.Bool
	ForceRerender   atomic.Bool

	// True when output directory did not exist at build start.
	IsCleanBuild bool

	// True during incremental asset-only (CSS/JS) builds.
	IsAssetOnlyBuild bool

	// Cleanup coordination
	CloseOnce sync.Once
}

// Engine maintains the state for site builds.
type Engine struct {
	Cfg *config.Config
	Ctx *buildctx.BuildContext

	// Service dependencies - injected at construction
	Deps EngineDependencies

	// Background cache flush coordination
	flushWaitGroup sync.WaitGroup

	// Build phase coordination - ensures background goroutines finish before returning from Build.
	buildWaitGroup sync.WaitGroup

	// Build output
	artifactMu       sync.Mutex
	artifactSink     fspkg.ArtifactSink
	buildTransaction tx.BuildTransaction

	// Watch coordinator for incremental builds
	Watch *watch.Coordinator

	// Asset pipeline manager
	Assets *assets.Manager

	// Incremental build manager
	Incremental *incremental.Manager

	// Search manager for search index regeneration
	Search *search.Manager

	// Build health registry
	Health *BuildHealthRegistry

	// Build coordination state
	State EngineState

	// Optional callbacks for build lifecycle synchronization
	OnBuildStart func()
	OnBuildDone  func()

	// Optional callbacks for search index regeneration lifecycle
	OnSearchStart func()
	OnSearchDone  func()
}

// newEngineFromManual creates a builder with manual dependency injection (for testing/benchmarks).
// Unspecified fields default to nil / zero values.
func newEngineFromManual(deps EngineDependencies) *Engine {
	if deps.Config == nil {
		deps.Config = &config.Config{}
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	cfg := deps.Config

	e := &Engine{
		Cfg: cfg,
		Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
			IsTesting:    true,
			IsDev:        cfg.IsDev,
			IsCleanBuild: false,
			Scheduler:    scheduler.NewBuildScheduler(),
			Logger:       deps.Logger,
		}),
		Deps:   deps,
		Health: NewBuildHealthRegistry(),
	}

	e.initManagers(deps.Cache, deps.Content, deps.Asset, deps.Render, deps.Scanner, deps.Diagrams, deps.MdPool, deps.NativeRenderer, deps.SourceFs)
	if deps.Render != nil {
		e.Search.Reconfigure(nil, deps.Render)
	}

	e.initWatch(deps.Cache)

	return e
}

func (e *Engine) initManagers(
	cacheSvc svcCache.Service,
	contentSvc content.Service,
	assetSvc asset.Service,
	renderSvc render.Service,
	_ scanner.Scanner,
	diagramAdapter *cache.DiagramCacheAdapter,
	mdPool *sync.Pool,
	nativeRenderer *native.Renderer,
	sourceFs afero.Fs,
) {
	e.Assets = assets.NewManager(assets.ManagerDependencies{
		Cfg:      e.Cfg,
		Asset:    assetSvc,
		Render:   renderSvc,
		Logger:   e.Deps.Logger,
		Metrics:  e.Deps.Metrics,
		SourceFs: sourceFs,
	})

	e.Search = search.NewManager(search.ManagerDependencies{
		Cfg:    e.Cfg,
		Cache:  cacheSvc,
		Logger: e.Deps.Logger,
		Health: e.Health,
	})

	e.State.ForceGenerators.Store(true)
	e.Incremental = incremental.NewManager(incremental.ManagerDependencies{
		Cfg:      e.Cfg,
		Logger:   e.Deps.Logger,
		SourceFs: sourceFs,
		Deps: incremental.Dependencies{
			Cache:    cacheSvc,
			Content:  contentSvc,
			Render:   renderSvc,
			Diagrams: diagramAdapter,
		},
		Builder:        e,
		Search:         e.Search,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	})
}

func (e *Engine) initWatch(cacheSvc svcCache.Service) {
	e.Watch = watch.New(watch.CoordinatorDependencies{
		Ctx:           e.Ctx.Ctx,
		Cfg:           e.Cfg,
		BuildMu:       &e.State.BuildMu,
		Cache:         cacheSvc,
		OnChange:      e.handleWatchChange,
		OnSearchRegen: e.handleSearchRegen,
	})
	e.Watch.Start()
}

// BuildAssetOnly runs asset-only incremental build (exposed for SiteBuilder interface).
// Delegates to BuildAssetOnlyWithOptions with forceImages=false.
func (e *Engine) BuildAssetOnly(ctx context.Context) error {
	return e.BuildAssetOnlyWithOptions(ctx, false)
}

// BuildAssetOnlyWithOptions runs asset-only incremental build with options.
func (e *Engine) BuildAssetOnlyWithOptions(ctx context.Context, forceImages bool) error {
	e.State.IsAssetOnlyBuild = true
	defer func() { e.State.IsAssetOnlyBuild = false }()

	// Start fresh session/tracking state
	e.refreshBuildSession(ctx)

	return e.Assets.BuildAssetOnlyWithOptions(ctx, e.runAssetOnlyPostProcessing, forceImages)
}

func (e *Engine) runAssetOnlyPostProcessing(ctx context.Context) error {
	e.Deps.Content.SetAssetsGate(nil)
	e.State.ForceGenerators.Store(true)

	metadataResult, err := e.Deps.Scanner.Scan(scanner.ScanOptions{
		Ctx:        ctx,
		ContentDir: e.Cfg.ContentDir,
		SrcFs:      e.Deps.SourceFs,
		Cfg:        e.Cfg,
		FileChan:   nil,
	})
	if err != nil {
		return fmt.Errorf("metadata scan failed: %w", err)
	}

	// Set up site-wide generators (need full asset completion)
	// Since assets are already built in BuildAssetOnlyWithOptions, we pass a closed channel
	assetsReadySignal := make(chan struct{})
	close(assetsReadySignal)
	wasmWaitGroup := &sync.WaitGroup{}
	runSiteWide, _ := e.setupSiteWideRendering(SiteWideOptions{
		Ctx:                ctx,
		AssetsReadySignal:  assetsReadySignal,
		WasmWaitGroup:      wasmWaitGroup,
		ForceSocialRebuild: false,
	})

	// For asset-only builds, we FORCE Content re-rendering to update asset hashes in HTML.
	contentResult, processError := e.Deps.Content.Process(content.ProcessOptions{
		Ctx:                ctx,
		ShouldForce:        false,
		ForceRerender:      true,
		ForceSocialRebuild: false,
		OutputMissing:      false,
		Files:              metadataResult.Files,
	})
	if processError != nil {
		return fmt.Errorf("content processing failed: %w", processError)
	}

	// Run site-wide generators
	metadataContext := contentResult.ToContext()
	siteWideGroup, siteTimer := runSiteWide(metadataContext, true)

	if siteWideGroup != nil {
		if err := e.waitForSiteWideRendering(siteWideGroup, siteTimer, contentResult.Has404 || e.Deps.Render.Has404Template(), metadataContext); err != nil {
			return fmt.Errorf("site-wide rendering failed: %w", err)
		}
	}

	// Remove original raster images when .webp equivalents exist
	assetpkg.CleanupOriginalImages(ctx, e.buildTransaction.StagingDir())

	// Finalize the build
	if err := e.buildTransaction.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	e.cleanupOrphans()

	if e.Deps.Metrics != nil {
		e.Deps.Metrics.RecordEnd()
		e.Deps.Logger.Info("Build complete")
		e.Deps.Metrics.Print()
	}

	return nil
}

// engineOptions holds configuration for NewEngine.
type engineOptions struct {
	args     []string
	cfg      *config.Config
	vfs      afero.Fs
	reporter ui.Reporter
	deps     *EngineDependencies
}

// EngineOption defines a functional option for Engine construction.
type EngineOption func(*engineOptions)

// WithArgs provides CLI arguments for configuration loading.
func WithArgs(args []string) EngineOption {
	return func(o *engineOptions) { o.args = args }
}

// WithConfig provides a pre-loaded configuration.
func WithConfig(cfg *config.Config) EngineOption {
	return func(o *engineOptions) { o.cfg = cfg }
}

// WithFs provides a custom filesystem (e.g., for testing).
func WithFs(vfs afero.Fs) EngineOption {
	return func(o *engineOptions) { o.vfs = vfs }
}

// WithReporter provides a custom UI reporter.
func WithReporter(r ui.Reporter) EngineOption {
	return func(o *engineOptions) { o.reporter = r }
}

// WithDeps provides manual dependency injection (primarily for testing/benchmarks).
func WithDeps(deps EngineDependencies) EngineOption {
	return func(o *engineOptions) { o.deps = &deps }
}

// NewEngine initializes a new site builder using functional options.
func NewEngine(opts ...EngineOption) *Engine {
	options := &engineOptions{
		vfs: afero.NewOsFs(),
	}
	for _, opt := range opts {
		opt(options)
	}

	// If manual dependencies are provided, use the fast-path constructor.
	if options.deps != nil {
		return newEngineFromManual(*options.deps)
	}

	// Otherwise, proceed with full service initialization.
	cfg := options.cfg
	if cfg == nil {
		cfg = config.Load(options.args)
	}

	return newEngineWithConfigFs(options.vfs, cfg, options.reporter)
}

// SetReporter updates the reporter and logger for the engine and all services.
func (e *Engine) SetReporter(reporter ui.Reporter) {
	e.Deps.Reporter = reporter
	e.Deps.Logger = InitLogger(reporter)
	e.Ctx.Logger = e.Deps.Logger

	// Update all services that hold onto the logger or reporter
	e.Deps.Asset.ReconfigureWithReporter(reporter, e.Deps.Logger)
	e.Deps.Content.ReconfigureWithReporter(reporter, e.Deps.Logger)
	e.Deps.Render.ReconfigureWithLogger(e.Deps.Logger)
	e.Assets.ReconfigureWithLogger(e.Deps.Logger)
	e.Incremental.ReconfigureWithLogger(e.Deps.Logger)
	e.Search.ReconfigureWithLogger(e.Deps.Logger)
}

// SetDevMode toggles dev mode on the active configuration.
func (e *Engine) SetDevMode(isDev bool) {
	config.SetDevMode(e.Cfg, isDev)
}

// SetSink configures the artifact sink and reconfigures services for a build pass.
func (e *Engine) SetSink(sink fspkg.ArtifactSink) {
	e.artifactSink = sink
	if sink != nil {
		e.Deps.Content.ReconfigureForBuild(sink, e.Deps.SourceFs)
		if e.Assets != nil {
			e.Assets.Reconfigure(sink, e.Deps.SourceFs)
		} else {
			e.Deps.Asset.ReconfigureForBuild(sink, e.Deps.SourceFs)
		}
		e.Deps.Render.ReconfigureForBuild(sink, e.Deps.SourceFs)
		if e.Search != nil {
			e.Search.Reconfigure(sink, e.Deps.Render)
		}
	}
}

// SetArtifactSink explicitly overrides the build engine's artifact sink.
// This is primarily used for testing and benchmarking.
func (e *Engine) SetArtifactSink(sink fspkg.ArtifactSink) {
	e.artifactMu.Lock()
	defer e.artifactMu.Unlock()
	e.artifactSink = sink
}

// SetBuildTransaction explicitly overrides the build engine's atomic transaction.
// This is primarily used for testing and benchmarking.
func (e *Engine) SetBuildTransaction(tx tx.BuildTransaction) {
	e.artifactMu.Lock()
	defer e.artifactMu.Unlock()
	e.buildTransaction = tx
}

// GetLogoPath returns the site logo path from config.
func (e *Engine) GetLogoPath() string {
	return e.Cfg.Logo
}

// SaveCaches waits for any background cache writes and persists BoltDB changes.
// Diagram cache flush is deferred to a background goroutine that completes during Close().
func (e *Engine) SaveCaches() {
	// Wait for background cache commit goroutines before closing BoltDB
	if e.Deps.Content != nil {
		e.Deps.Content.WaitForCacheCommit()
	}
	if e.Deps.Diagrams != nil {
		// Launch flush in background — completes during Close() before BoltDB closes.
		// Cache loss on process crash is acceptable: entries are regenerated next build.
		e.flushWaitGroup.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       e.Ctx.Ctx,
			Logger:    e.Deps.Logger,
			Operation: "diagram cache flush",
			Fn: func() error {
				if err := e.Deps.Diagrams.Flush(e.Ctx.Ctx); err != nil {
					e.Deps.Logger.Warn("Diagram cache flush failed", "error", err)
				}
				return nil
			},
			Cleanup: e.flushWaitGroup.Done,
		})
	}
	if e.Deps.Fragments != nil {
		e.flushWaitGroup.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       e.Ctx.Ctx,
			Logger:    e.Deps.Logger,
			Operation: "fragment cache flush",
			Fn: func() error {
				if err := e.Deps.Fragments.Flush(e.Ctx.Ctx); err != nil {
					e.Deps.Logger.Warn("Fragment cache flush failed", "error", err)
				}
				return nil
			},
			Cleanup: e.flushWaitGroup.Done,
		})
	}
	if e.Deps.Cache != nil {
		// Trigger garbage collection every cacheGCPollInterval builds
		if count, err := e.Deps.Cache.IncrementBuildCount(); err == nil && count >= cacheGCPollInterval {
			e.Deps.Logger.Info("Triggering scheduled cache garbage collection", "builds", count)
			if result, err := e.Deps.Cache.RunGC(gc.Config{
				MaxAge: cacheGCMaxAge,
			}); err != nil {
				e.Deps.Logger.Warn("Cache garbage collection failed", "error", err)
			} else {
				e.Deps.Logger.Info("Cache garbage collection complete",
					"deleted_blobs", result.DeletedBlobs,
					"deleted_bytes", result.DeletedBytes,
					"duration", result.Duration)
			}
		}

		DevLogSuccess("Saved caches")
	}
}

// Close releases build resources.
func (e *Engine) Close() {
	e.State.CloseOnce.Do(func() {
		if e.Watch != nil {
			e.Watch.Close()
		}

		// Gracefully stop the image cache background writer
		assetpkg.StopImageCacheWriter()

		// Wait for any background cache flush to complete before closing BoltDB
		e.flushWaitGroup.Wait()

		// Cancel engine lifetime context to stop any remaining background goroutines
		if e.Ctx != nil && e.Ctx.Cancel != nil {
			e.Ctx.Cancel()
		}

		if e.Deps.Diagrams != nil {
			_ = e.Deps.Diagrams.Close()
		}

		if e.Deps.NativeRenderer != nil {
			_ = e.Deps.NativeRenderer.Close()
		}
		if e.Deps.Cache != nil {
			_ = e.Deps.Cache.Close()
		}
	})
}

// RefreshBuildSession refreshes build session state.
func (e *Engine) RefreshBuildSession() {
	e.refreshBuildSession(e.Ctx.Ctx)
}

// Commit commits the current transaction.
func (e *Engine) Commit(ctx context.Context) error {
	return e.buildTransaction.Commit(ctx)
}

// LockBuild acquires the build mutex to prevent concurrent builds.
func (e *Engine) LockBuild() {
	e.State.BuildMu.Lock()
}

// UnlockBuild releases the build mutex.
func (e *Engine) UnlockBuild() {
	e.State.BuildMu.Unlock()
}

// GetWatch returns the watch coordinator.
func (e *Engine) GetWatch() incremental.WatchCoordinator {
	if e.Watch == nil {
		return nil
	}
	return e.Watch
}

// GetRender returns the render service.
func (e *Engine) GetRender() render.Service {
	return e.Deps.Render
}

// GetContent returns the Content service.
func (e *Engine) GetContent() content.Service {
	return e.Deps.Content
}

// handleWatchChange is the callback invoked by WatchCoordinator when a debounced
// file change batch is ready to process.
func (e *Engine) handleWatchChange(evt watch.ChangeEvent) {
	if e.OnBuildStart != nil {
		e.OnBuildStart()
	}
	defer func() {
		if e.OnBuildDone != nil {
			e.OnBuildDone()
		}
	}()

	// Check if kosh.yaml changed - requires full config reload and rebuild
	if evt.Path == "kosh.yaml" || evt.Path == "config.yaml" {
		DevLogInfo("Config file changed, reloading configuration...")
		if err := e.ReloadConfig(e.Ctx.Ctx); err != nil {
			DevLogError("Failed to reload config: " + err.Error())
			return
		}
		// Trigger full rebuild with the new config
		if e.Incremental != nil {
			e.Incremental.BuildSingleFileChange(e.Ctx.Ctx, evt.Path, evt.Op)
		}
		return
	}

	if e.Incremental != nil {
		e.Incremental.BuildSingleFileChange(e.Ctx.Ctx, evt.Path, evt.Op)
	}
}

// handleSearchRegen is the callback invoked by WatchCoordinator when a search
// index regeneration is requested.
func (e *Engine) handleSearchRegen(ctx context.Context) {
	if e.OnSearchStart != nil {
		e.OnSearchStart()
	}
	defer func() {
		if e.OnSearchDone != nil {
			e.OnSearchDone()
		}
	}()

	if e.Search != nil {
		_ = e.Search.RegenerateIndex(ctx)
	}
}

// Run executes the main build logic
func Run(args []string, reporter ui.Reporter) error {
	e := NewEngine(WithArgs(args), WithReporter(reporter))
	if reporter != nil {
		reporter.Start("Build")
	}
	defer e.Close()
	defer e.SaveCaches()
	if err := e.Build(e.Ctx.Ctx); err != nil {
		e.Deps.Logger.Error("Build failed", "error", err)
		return err
	}
	return nil
}
