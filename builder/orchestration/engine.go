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
	"github.com/Kush-Singh-26/kosh/builder/services/post"
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
	Cache    svcCache.Service
	Post     post.Service
	Asset    asset.Service
	Render   render.Service
	Wasm     wasm.Service
	Scanner  scanner.Scanner
	Diagrams *cache.DiagramCacheAdapter
	Reporter ui.Reporter

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

	engineInstance := &Engine{
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

	// Initialize asset manager
	engineInstance.Assets = assets.NewManager(assets.ManagerDependencies{
		Cfg:      cfg,
		Asset:    deps.Asset,
		Render:   deps.Render,
		Logger:   deps.Logger,
		Metrics:  deps.Metrics,
		SourceFs: deps.SourceFs,
	})

	// Initialize search manager
	engineInstance.Search = search.NewManager(search.ManagerDependencies{
		Cfg:    cfg,
		Logger: deps.Logger,
		Health: engineInstance.Health,
	})
	if deps.Render != nil {
		engineInstance.Search.Reconfigure(nil, deps.Render)
	}

	// Initialize incremental build manager
	engineInstance.Incremental = incremental.NewManager(incremental.ManagerDependencies{
		Cfg:      cfg,
		Logger:   deps.Logger,
		SourceFs: deps.SourceFs,
		Deps: incremental.IncrementalDependencies{
			Cache:    deps.Cache,
			Post:     deps.Post,
			Render:   deps.Render,
			Diagrams: deps.Diagrams,
		},
		Builder:        engineInstance,
		Search:         engineInstance.Search,
		MdPool:         deps.MdPool,
		NativeRenderer: deps.NativeRenderer,
	})

	// Initialize watch coordinator for incremental builds
	engineInstance.Watch = watch.New(watch.CoordinatorDependencies{
		Cfg:           cfg,
		BuildMu:       &engineInstance.State.BuildMu,
		Cache:         engineInstance.Deps.Cache,
		OnChange:      engineInstance.handleWatchChange,
		OnSearchRegen: engineInstance.handleSearchRegen,
	})

	return engineInstance
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
func (engineInstance *Engine) SetReporter(reporter ui.Reporter) {
	engineInstance.Deps.Reporter = reporter
	engineInstance.Deps.Logger = InitLogger(reporter)
	engineInstance.Ctx.Logger = engineInstance.Deps.Logger

	// Update all services that hold onto the logger or reporter
	engineInstance.Deps.Asset.ReconfigureWithReporter(reporter, engineInstance.Deps.Logger)
	engineInstance.Deps.Post.ReconfigureWithReporter(reporter, engineInstance.Deps.Logger)
	engineInstance.Deps.Render.ReconfigureWithLogger(engineInstance.Deps.Logger)
	engineInstance.Assets.ReconfigureWithLogger(engineInstance.Deps.Logger)
	engineInstance.Incremental.ReconfigureWithLogger(engineInstance.Deps.Logger)
	engineInstance.Search.ReconfigureWithLogger(engineInstance.Deps.Logger)
}

// SetDevMode toggles dev mode on the active configuration.
func (engineInstance *Engine) SetDevMode(isDev bool) {
	config.SetDevMode(engineInstance.Cfg, isDev)
}

// SetSink configures the artifact sink and reconfigures services for a build pass.
func (engineInstance *Engine) SetSink(sink fspkg.ArtifactSink) {
	engineInstance.artifactSink = sink
	if sink != nil {
		engineInstance.Deps.Post.ReconfigureForBuild(sink, engineInstance.Deps.SourceFs)
		if engineInstance.Assets != nil {
			engineInstance.Assets.Reconfigure(sink, engineInstance.Deps.SourceFs)
		} else {
			engineInstance.Deps.Asset.ReconfigureForBuild(sink, engineInstance.Deps.SourceFs)
		}
		engineInstance.Deps.Render.ReconfigureForBuild(sink, engineInstance.Deps.SourceFs)
		if engineInstance.Search != nil {
			engineInstance.Search.Reconfigure(sink, engineInstance.Deps.Render)
		}
	}
}

// SetArtifactSink explicitly overrides the build engine's artifact sink.
// This is primarily used for testing and benchmarking.
func (engineInstance *Engine) SetArtifactSink(sink fspkg.ArtifactSink) {
	engineInstance.artifactMu.Lock()
	defer engineInstance.artifactMu.Unlock()
	engineInstance.artifactSink = sink
}

// SetBuildTransaction explicitly overrides the build engine's atomic transaction.
// This is primarily used for testing and benchmarking.
func (engineInstance *Engine) SetBuildTransaction(tx tx.BuildTransaction) {
	engineInstance.artifactMu.Lock()
	defer engineInstance.artifactMu.Unlock()
	engineInstance.buildTransaction = tx
}

// GetLogoPath returns the site logo path from config.
func (engineInstance *Engine) GetLogoPath() string {
	return engineInstance.Cfg.Logo
}

// SaveCaches waits for any background cache writes and persists BoltDB changes.
// Diagram cache flush is deferred to a background goroutine that completes during Close().
func (engineInstance *Engine) SaveCaches() {
	// Wait for background cache commit goroutines before closing BoltDB
	if engineInstance.Deps.Post != nil {
		engineInstance.Deps.Post.WaitForCacheCommit()
	}
	if engineInstance.Deps.Diagrams != nil {
		// Launch flush in background — completes during Close() before BoltDB closes.
		// Cache loss on process crash is acceptable: entries are regenerated next build.
		engineInstance.flushWaitGroup.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       context.Background(),
			Logger:    engineInstance.Deps.Logger,
			Operation: "diagram cache flush",
			Fn: func() error {
				if err := engineInstance.Deps.Diagrams.Flush(context.Background()); err != nil {
					engineInstance.Deps.Logger.Warn("Diagram cache flush failed", "error", err)
				}
				return nil
			},
			Cleanup: engineInstance.flushWaitGroup.Done,
		})
	}
	if engineInstance.Deps.Cache != nil {
		// Trigger garbage collection every cacheGCPollInterval builds
		if count, err := engineInstance.Deps.Cache.IncrementBuildCount(); err == nil && count >= cacheGCPollInterval {
			engineInstance.Deps.Logger.Info("Triggering scheduled cache garbage collection", "builds", count)
			if result, err := engineInstance.Deps.Cache.RunGC(gc.GCConfig{
				MaxAge: cacheGCMaxAge,
			}); err != nil {
				engineInstance.Deps.Logger.Warn("Cache garbage collection failed", "error", err)
			} else {
				engineInstance.Deps.Logger.Info("Cache garbage collection complete",
					"deleted_blobs", result.DeletedBlobs,
					"deleted_bytes", result.DeletedBytes,
					"duration", result.Duration)
			}
		}

		DevLogSuccess("Saved caches")
	}
}

// Close releases build resources.
func (engineInstance *Engine) Close() {
	engineInstance.State.CloseOnce.Do(func() {
		if engineInstance.Watch != nil {
			engineInstance.Watch.Close()
		}

		// Wait for any background cache flush to complete before closing BoltDB
		engineInstance.flushWaitGroup.Wait()

		if engineInstance.Deps.Diagrams != nil {
			_ = engineInstance.Deps.Diagrams.Close()
		}

		if engineInstance.Deps.NativeRenderer != nil {
			_ = engineInstance.Deps.NativeRenderer.Close()
		}
		if engineInstance.Deps.Cache != nil {
			_ = engineInstance.Deps.Cache.Close()
		}
	})
}

// BuildAssetOnly runs asset-only incremental build (exposed for SiteBuilder interface).
func (engineInstance *Engine) BuildAssetOnly(ctx context.Context) error {
	return engineInstance.buildAssetOnly(ctx)
}

// BuildAssetOnlyWithOptions runs asset-only incremental build with options.
func (engineInstance *Engine) BuildAssetOnlyWithOptions(ctx context.Context, forceImages bool) error {
	engineInstance.State.IsAssetOnlyBuild = true
	defer func() { engineInstance.State.IsAssetOnlyBuild = false }()

	// Start fresh session/tracking state
	engineInstance.refreshBuildSession()

	return engineInstance.Assets.BuildAssetOnlyWithOptions(ctx, func(ctx context.Context) error {
		engineInstance.Deps.Post.SetAssetsGate(nil)
		engineInstance.State.ForceGenerators.Store(true)

		metadataResult, err := engineInstance.Deps.Scanner.Scan(scanner.ScanOptions{
			Ctx:        ctx,
			ContentDir: engineInstance.Cfg.ContentDir,
			SrcFs:      engineInstance.Deps.SourceFs,
			Cfg:        engineInstance.Cfg,
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
		runSiteWide, _ := engineInstance.setupSiteWideRendering(SiteWideOptions{
			Ctx:                ctx,
			AssetsReadySignal:  assetsReadySignal,
			WasmWaitGroup:      wasmWaitGroup,
			ForceSocialRebuild: false,
		})

		// For asset-only builds, we FORCE post re-rendering to update asset hashes in HTML.
		shouldForce := true
		forceSocialRebuild := false
		outputMissing := false
		postResult, processError := engineInstance.Deps.Post.Process(post.ProcessOptions{
			Ctx:                ctx,
			ShouldForce:        shouldForce,
			ForceSocialRebuild: forceSocialRebuild,
			OutputMissing:      outputMissing,
			Files:              metadataResult.Files,
		})
		if processError != nil {
			return fmt.Errorf("post processing failed: %w", processError)
		}

		// Run site-wide generators
		assetsChanged := true // Assets definitely changed in an asset-only build
		metadataContext := postResult.ToMetadataContext()
		siteWideGroup, siteTimer := runSiteWide(metadataContext, assetsChanged)

		if siteWideGroup != nil {
			if err := engineInstance.waitForSiteWideRendering(siteWideGroup, siteTimer, postResult.Has404 || engineInstance.Deps.Render.Has404Template(), metadataContext); err != nil {
				return fmt.Errorf("site-wide rendering failed: %w", err)
			}
		}

		// Remove original raster images when .webp equivalents exist
		assetpkg.CleanupOriginalImages(engineInstance.buildTransaction.StagingDir())

		// Finalize the build
		if err := engineInstance.buildTransaction.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		return nil
	}, forceImages)
}

// RefreshBuildSession refreshes build session state.
func (engineInstance *Engine) RefreshBuildSession() {
	engineInstance.refreshBuildSession()
}

// Commit commits the current transaction.
func (engineInstance *Engine) Commit(ctx context.Context) error {
	return engineInstance.buildTransaction.Commit(ctx)
}

// LockBuild acquires the build mutex to prevent concurrent builds.
func (engineInstance *Engine) LockBuild() {
	engineInstance.State.BuildMu.Lock()
}

// UnlockBuild releases the build mutex.
func (engineInstance *Engine) UnlockBuild() {
	engineInstance.State.BuildMu.Unlock()
}

// GetWatch returns the watch coordinator.
func (engineInstance *Engine) GetWatch() incremental.WatchCoordinator {
	if engineInstance.Watch == nil {
		return nil
	}
	return engineInstance.Watch
}

// GetRender returns the render service.
func (engineInstance *Engine) GetRender() render.Service {
	return engineInstance.Deps.Render
}

// GetPost returns the post service.
func (engineInstance *Engine) GetPost() post.Service {
	return engineInstance.Deps.Post
}

// handleWatchChange is the callback invoked by WatchCoordinator when a debounced
// file change batch is ready to process.
func (engineInstance *Engine) handleWatchChange(evt watch.ChangeEvent) {
	if engineInstance.OnBuildStart != nil {
		engineInstance.OnBuildStart()
	}
	defer func() {
		if engineInstance.OnBuildDone != nil {
			engineInstance.OnBuildDone()
		}
	}()

	if engineInstance.Incremental != nil {
		engineInstance.Incremental.BuildSingleFileChange(context.Background(), evt.Path, evt.Op)
	}
}

// handleSearchRegen is the callback invoked by WatchCoordinator when a search
// index regeneration is requested.
func (engineInstance *Engine) handleSearchRegen(ctx context.Context) {
	if engineInstance.OnSearchStart != nil {
		engineInstance.OnSearchStart()
	}
	defer func() {
		if engineInstance.OnSearchDone != nil {
			engineInstance.OnSearchDone()
		}
	}()

	if engineInstance.Search != nil {
		_ = engineInstance.Search.RegenerateIndex(ctx)
	}
}

// Run executes the main build logic
func Run(args []string, reporter ui.Reporter) error {
	engineInstance := NewEngine(WithArgs(args), WithReporter(reporter))
	if reporter != nil {
		reporter.Start("Build")
	}
	defer engineInstance.Close()
	defer engineInstance.SaveCaches()
	if err := engineInstance.Build(context.Background()); err != nil {
		engineInstance.Deps.Logger.Error("Build failed", "error", err)
		return err
	}
	return nil
}
