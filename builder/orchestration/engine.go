package orchestration

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"

	assetpkg "github.com/Kush-Singh-26/kosh/builder/assets"
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

// NewEngineFromManual creates a builder with manual dependency injection (for testing/benchmarks).
// Unspecified fields default to nil / zero values.
func NewEngineFromManual(deps EngineDependencies) *Engine {
	if deps.Config == nil {
		deps.Config = &config.Config{}
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	cfg := deps.Config

	b := &Engine{
		Cfg:    cfg,
		Ctx:    buildCtx.NewBuildContext(true, cfg.IsDev, false, scheduler.NewBuildScheduler(), deps.Logger),
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

// NewEngine initializes a new site builder
func NewEngine(args []string) *Engine {
	cfg := config.Load(args)
	return newEngineWithConfig(cfg)
}

// NewEngineWithConfig initializes a new site builder with a pre-loaded config
func NewEngineWithConfig(cfg *config.Config) *Engine {
	return newEngineWithConfig(cfg)
}

// NewEngineWithFs initializes a new site builder with a pre-loaded config and custom filesystem
func NewEngineWithFs(vfs afero.Fs, cfg *config.Config) *Engine {
	return newEngineWithConfigFs(vfs, cfg)
}

// newEngineWithConfig is the internal implementation
func newEngineWithConfig(cfg *config.Config) *Engine {
	return newEngineWithConfigFs(afero.NewOsFs(), cfg)
}

func (b *Engine) SetDevMode(isDev bool) {
	config.SetDevMode(b.Cfg, isDev)
}

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

func (b *Engine) SetSourceFs(fs afero.Fs) {
	b.Deps.SourceFs = fs
	// Trigger reconfiguration of all services with the new filesystem
	if b.Sink != nil {
		b.SetSink(b.Sink)
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
		go func() {
			defer b.flushWg.Done()
			if err := b.Deps.Diagrams.Flush(); err != nil {
				b.Deps.Logger.Warn("Diagram cache flush failed", "error", err)
			}
		}()
	}
	if b.Deps.Cache != nil {
		// Trigger garbage collection every 20 builds
		if count, err := b.Deps.Cache.IncrementBuildCount(); err == nil && count >= 20 {
			b.Deps.Logger.Info("Triggering scheduled cache garbage collection", "builds", count)
			if result, err := b.Deps.Cache.RunGC(gc.GCConfig{
				MaxAge: 7 * 24 * time.Hour,
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

		metadataResult, err := b.Deps.Scanner.Scan(ctx, b.Cfg.ContentDir, b.Deps.SourceFs, b.Cfg, nil)
		if err != nil {
			return fmt.Errorf("metadata scan failed: %w", err)
		}

		// Set up site-wide generators (need full asset completion)
		// Since assets are already built in BuildAssetOnlyWithOptions, we pass a closed channel
		assetsReady := make(chan struct{})
		close(assetsReady)
		wasmWg := &sync.WaitGroup{}
		runSiteWide, _ := b.setupSiteWideRendering(ctx, assetsReady, wasmWg, false)

		shouldForce := false
		forceSocialRebuild := false
		outputMissing := false
		postResult, err := b.processPosts(ctx, shouldForce, forceSocialRebuild, outputMissing, metadataResult.Files)
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

		// Batch rewrite image paths in output HTML for converted images.
		assetpkg.RewriteImagePaths(b.Tx.StagingDir())

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
func Run(args []string) error {
	b := NewEngine(args)
	defer b.Close()
	defer b.SaveCaches()
	if err := b.Build(context.Background()); err != nil {
		b.Deps.Logger.Error("Build failed", "error", err)
		return err
	}
	return nil
}
