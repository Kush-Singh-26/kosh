package orchestration

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"

	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/assets"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/search"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
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

	ForceGenerators atomic.Bool

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

	// Watch coordinator for incremental builds
	Watch *watch.Coordinator

	// Asset pipeline manager
	Assets *assets.Manager

	// Search manager for search index regeneration
	Search *search.Manager

	// Build-time state (separate from dependencies)
	State EngineState

	// Build health tracking
	Health *BuildHealthRegistry
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
	if cfg == nil {
		cfg = &config.Config{}
	}
	if logger == nil {
		logger = slog.Default()
	}

	b := &Engine{
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
		Health:         NewBuildHealthRegistry(),
	}

	// Initialize asset manager
	b.Assets = assets.NewManager(assets.ManagerDependencies{
		Cfg:      cfg,
		Asset:    asset,
		Render:   render,
		Logger:   logger,
		Metrics:  m,
		SourceFs: sourceFs,
	})

	// Initialize search manager
	b.Search = search.NewManager(search.ManagerDependencies{
		Cfg:    cfg,
		Logger: logger,
		Health: b.Health,
	})
	if render != nil {
		b.Search.Reconfigure(nil, render)
	}

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

func (b *Engine) Config() *config.Config {
	return b.Cfg
}

func (b *Engine) SetDevMode(isDev bool) {
	config.SetDevMode(b.Cfg, isDev)
}

func (b *Engine) SetSink(sink fspkg.ArtifactSink) {
	b.Sink = sink
	if b.Assets != nil {
		b.Assets.Reconfigure(sink, b.SourceFs)
	}
	if b.Search != nil {
		b.Search.Reconfigure(sink, b.Deps.Render)
	}
}

func (b *Engine) SetTx(tx tx.BuildTransaction) {
	b.Tx = tx
}

func (b *Engine) SetSourceFs(fs afero.Fs) {
	b.SourceFs = fs
	// Use consolidated reconfiguration method
	if b.Sink != nil {
		b.Deps.Post.ReconfigureForBuild(b.Sink, fs)
		if b.Assets != nil {
			b.Assets.Reconfigure(b.Sink, fs)
		} else {
			b.Deps.Asset.ReconfigureForBuild(b.Sink, fs)
		}
		b.Deps.Render.ReconfigureForBuild(b.Sink, fs)
		if b.Search != nil {
			b.Search.Reconfigure(b.Sink, b.Deps.Render)
		}
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
	if b.Deps.Diagrams != nil {
		_ = b.Deps.Diagrams.Flush()
	}
	if b.Deps.Cache != nil {
		// Manager implementation handles the actual DB commit
		if manager, ok := b.Deps.Cache.(interface{ Save() error }); ok {
			_ = manager.Save()
		}

		// Trigger garbage collection every 20 builds
		if count, err := b.Deps.Cache.IncrementBuildCount(); err == nil && count >= 20 {
			b.Logger.Info("Triggering scheduled cache garbage collection", "builds", count)
			if result, err := b.Deps.Cache.RunGC(gc.GCConfig{
				MaxAge: 7 * 24 * time.Hour,
			}); err != nil {
				b.Logger.Warn("Cache garbage collection failed", "error", err)
			} else {
				b.Logger.Info("Cache garbage collection complete",
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

		if b.Deps.Diagrams != nil {
			_ = b.Deps.Diagrams.Close()
		}

		if b.NativeRenderer != nil {
			_ = b.NativeRenderer.Close()
		}
		if b.Deps.Cache != nil {
			_ = b.Deps.Cache.Close()
		}
	})
}

// handleWatchChange is the callback invoked by WatchCoordinator when a debounced
// file change batch is ready to process.
func (b *Engine) handleWatchChange(evt watch.ChangeEvent) {
	b.buildSingleFileChange(context.Background(), evt.Path, evt.Op)
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
