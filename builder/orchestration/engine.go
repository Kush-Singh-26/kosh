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
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
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

// Engine maintains the state for site builds
type Engine struct {
	Cfg *config.Config
	Ctx *buildCtx.BuildContext

	// Service dependencies - injected at construction
	Deps EngineDependencies

	// Background cache flush coordination
	flushWg sync.WaitGroup

	// Build output
	Sink fspkg.ArtifactSink
	Tx   tx.BuildTransaction

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

	b := &Engine{
		Cfg: cfg,
		Ctx: buildCtx.NewBuildContext(buildCtx.ContextOptions{
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
	b.Assets = assets.NewManager(assets.ManagerDependencies{
		Cfg:      cfg,
		Asset:    deps.Asset,
		Render:   deps.Render,
		Logger:   deps.Logger,
		Metrics:  deps.Metrics,
		SourceFs: deps.SourceFs,
	})

	// Initialize search manager
	b.Search = search.NewManager(search.ManagerDependencies{
		Cfg:    cfg,
		Logger: deps.Logger,
		Health: b.Health,
	})
	if deps.Render != nil {
		b.Search.Reconfigure(nil, deps.Render)
	}

	// Initialize incremental build manager
	b.Incremental = incremental.NewManager(incremental.ManagerDependencies{
		Cfg:      cfg,
		Logger:   deps.Logger,
		SourceFs: deps.SourceFs,
		Deps: incremental.IncrementalDependencies{
			Cache:    deps.Cache,
			Post:     deps.Post,
			Render:   deps.Render,
			Diagrams: deps.Diagrams,
		},
		Builder:        b,
		Search:         b.Search,
		MdPool:         deps.MdPool,
		NativeRenderer: deps.NativeRenderer,
	})

	// Initialize watch coordinator for incremental builds
	b.Watch = watch.New(watch.CoordinatorDependencies{
		Cfg:           cfg,
		BuildMu:       &b.State.BuildMu,
		Cache:         b.Deps.Cache,
		OnChange:      b.handleWatchChange,
		OnSearchRegen: func(ctx context.Context) { _ = b.Search.RegenerateIndex(ctx) },
	})

	return b
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
	o := &engineOptions{
		vfs: afero.NewOsFs(),
	}
	for _, opt := range opts {
		opt(o)
	}

	// If manual dependencies are provided, use the fast-path constructor.
	if o.deps != nil {
		return newEngineFromManual(*o.deps)
	}

	// Otherwise, proceed with full service initialization.
	cfg := o.cfg
	if cfg == nil {
		cfg = config.Load(o.args)
	}

	return newEngineWithConfigFs(o.vfs, cfg, o.reporter)
}

// SetReporter updates the reporter and logger for the engine and all services.
func (b *Engine) SetReporter(r ui.Reporter) {
	b.Deps.Reporter = r
	b.Deps.Logger = InitLogger(r)
	b.Ctx.Logger = b.Deps.Logger

	// Update all services that hold onto the logger or reporter
	b.Deps.Asset.ReconfigureWithReporter(r, b.Deps.Logger)
	b.Deps.Post.ReconfigureWithReporter(r, b.Deps.Logger)
	b.Deps.Render.ReconfigureWithLogger(b.Deps.Logger)
	b.Assets.ReconfigureWithLogger(b.Deps.Logger)
	b.Incremental.ReconfigureWithLogger(b.Deps.Logger)
	b.Search.ReconfigureWithLogger(b.Deps.Logger)
}

// SetDevMode toggles dev mode on the active configuration.
func (b *Engine) SetDevMode(isDev bool) {
	config.SetDevMode(b.Cfg, isDev)
}

// SetSink configures the artifact sink and reconfigures services for a build pass.
func (b *Engine) SetSink(sink fspkg.ArtifactSink) {
	b.Sink = sink
	if sink != nil {
		b.Deps.Post.ReconfigureForBuild(sink, b.Deps.SourceFs)
		if b.Assets != nil {
			b.Assets.Reconfigure(sink, b.Deps.SourceFs)
		} else {
			b.Deps.Asset.ReconfigureForBuild(sink, b.Deps.SourceFs)
		}
		b.Deps.Render.ReconfigureForBuild(sink, b.Deps.SourceFs)
		if b.Search != nil {
			b.Search.Reconfigure(sink, b.Deps.Render)
		}
	}
}

// SetSourceFs updates the source filesystem and reconfigures dependent services.
func (b *Engine) SetSourceFs(fs afero.Fs) {
	b.Deps.SourceFs = fs
	// Trigger reconfiguration of all services with the new filesystem
	if b.Sink != nil {
		b.SetSink(b.Sink)
	}
}

// getLogoPath returns the site logo path from config.
func (b *Engine) getLogoPath() string {
	return b.Cfg.Logo
}

// SaveCaches waits for any background cache writes and persists BoltDB changes.
// Diagram cache flush is deferred to a background goroutine that completes during Close().
func (b *Engine) SaveCaches() {
	// Wait for background cache commit goroutines before closing BoltDB
	if b.Deps.Post != nil {
		b.Deps.Post.WaitForCacheCommit()
	}
	if b.Deps.Diagrams != nil {
		// Launch flush in background — completes during Close() before BoltDB closes.
		// Cache loss on process crash is acceptable: entries are regenerated next build.
		b.flushWg.Add(1)
		async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
			Ctx:       context.Background(),
			Logger:    b.Deps.Logger,
			Operation: "diagram cache flush",
			Fn: func() error {
				if err := b.Deps.Diagrams.Flush(); err != nil {
					b.Deps.Logger.Warn("Diagram cache flush failed", "error", err)
				}
				return nil
			},
			Cleanup: b.flushWg.Done,
		})
	}
	if b.Deps.Cache != nil {
		// Trigger garbage collection every cacheGCPollInterval builds
		if count, err := b.Deps.Cache.IncrementBuildCount(); err == nil && count >= cacheGCPollInterval {
			b.Deps.Logger.Info("Triggering scheduled cache garbage collection", "builds", count)
			if result, err := b.Deps.Cache.RunGC(gc.GCConfig{
				MaxAge: cacheGCMaxAge,
			}); err != nil {
				b.Deps.Logger.Warn("Cache garbage collection failed", "error", err)
			} else {
				b.Deps.Logger.Info("Cache garbage collection complete",
					"deleted_blobs", result.DeletedBlobs,
					"deleted_bytes", result.DeletedBytes,
					"duration", result.Duration)
			}
		}

		DevLogSuccess("Saved caches")
	}
}

// Close releases build resources
func (b *Engine) Close() {
	b.State.CloseOnce.Do(func() {
		if b.Watch != nil {
			b.Watch.Close()
		}

		// Wait for any background cache flush to complete before closing BoltDB
		b.flushWg.Wait()

		if b.Deps.Diagrams != nil {
			_ = b.Deps.Diagrams.Close()
		}

		if b.Deps.NativeRenderer != nil {
			_ = b.Deps.NativeRenderer.Close()
		}
		if b.Deps.Cache != nil {
			_ = b.Deps.Cache.Close()
		}
	})
}

// BuildAssetOnly runs asset-only incremental build (exposed for SiteBuilder interface)
func (b *Engine) BuildAssetOnly(ctx context.Context) error {
	return b.buildAssetOnly(ctx)
}

// BuildAssetOnlyWithOptions runs asset-only incremental build with options
func (b *Engine) BuildAssetOnlyWithOptions(ctx context.Context, forceImages bool) error {
	b.State.IsAssetOnlyBuild = true
	defer func() { b.State.IsAssetOnlyBuild = false }()

	// Start fresh session/tracking state
	b.refreshBuildSession()

	return b.Assets.BuildAssetOnlyWithOptions(ctx, func(ctx context.Context) error {
		b.Deps.Post.SetAssetsGate(nil)
		b.State.ForceGenerators.Store(true)

		metadataResult, err := b.Deps.Scanner.Scan(scanner.ScanOptions{
			Ctx:        ctx,
			ContentDir: b.Cfg.ContentDir,
			SrcFs:      b.Deps.SourceFs,
			Cfg:        b.Cfg,
			FileChan:   nil,
		})
		if err != nil {
			return fmt.Errorf("metadata scan failed: %w", err)
		}

		// Set up site-wide generators (need full asset completion)
		// Since assets are already built in BuildAssetOnlyWithOptions, we pass a closed channel
		assetsReady := make(chan struct{})
		close(assetsReady)
		wasmWg := &sync.WaitGroup{}
		runSiteWide, _ := b.setupSiteWideRendering(SiteWideOptions{
			Ctx:                ctx,
			AssetsReady:        assetsReady,
			WasmWg:             wasmWg,
			ForceSocialRebuild: false,
		})

		shouldForce := false
		forceSocialRebuild := false
		outputMissing := false
		postResult, err := b.Deps.Post.Process(post.ProcessOptions{
			Ctx:                ctx,
			ShouldForce:        shouldForce,
			ForceSocialRebuild: forceSocialRebuild,
			OutputMissing:      outputMissing,
			Files:              metadataResult.Files,
		})
		if err != nil {
			return fmt.Errorf("post processing failed: %w", err)
		}

		// Run site-wide generators
		assetsChanged := true // Assets definitely changed in an asset-only build
		metadataCtx := postResult.ToMetadataContext()
		siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

		if siteWideGroup != nil {
			if err := b.waitForSiteWideRendering(siteWideGroup, siteTimer, postResult.Has404 || b.Deps.Render.Has404Template()); err != nil {
				return fmt.Errorf("site-wide rendering failed: %w", err)
			}
		}

		// Remove original raster images when .webp equivalents exist
		assetpkg.CleanupOriginalImages(b.Tx.StagingDir())

		// Finalize the build
		if err := b.Tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		return nil
	}, forceImages)
}

// RefreshBuildSession refreshes build session state
func (b *Engine) RefreshBuildSession() {
	b.refreshBuildSession()
}

// Commit commits the current transaction
func (b *Engine) Commit(ctx context.Context) error {
	return b.Tx.Commit(ctx)
}

// LockBuild acquires the build mutex to prevent concurrent builds
func (b *Engine) LockBuild() {
	b.State.BuildMu.Lock()
}

// UnlockBuild releases the build mutex
func (b *Engine) UnlockBuild() {
	b.State.BuildMu.Unlock()
}

// GetWatch returns the watch coordinator
func (b *Engine) GetWatch() incremental.WatchCoordinator {
	if b.Watch == nil {
		return nil
	}
	return b.Watch
}

// GetRender returns the render service
func (b *Engine) GetRender() render.Service {
	return b.Deps.Render
}

// GetPost returns the post service
func (b *Engine) GetPost() post.Service {
	return b.Deps.Post
}

// handleWatchChange is the callback invoked by WatchCoordinator when a debounced
// file change batch is ready to process.
func (b *Engine) handleWatchChange(evt watch.ChangeEvent) {
	if b.Incremental != nil {
		b.Incremental.BuildSingleFileChange(context.Background(), evt.Path, evt.Op)
	}
}

// Run executes the main build logic
func Run(args []string, r ui.Reporter) error {
	b := NewEngine(WithArgs(args), WithReporter(r))
	if r != nil {
		r.Start("Build")
	}
	defer b.Close()
	defer b.SaveCaches()
	if err := b.Build(context.Background()); err != nil {
		b.Deps.Logger.Error("Build failed", "error", err)
		return err
	}
	return nil
}
