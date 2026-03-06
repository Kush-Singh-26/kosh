package run

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"
	"github.com/twincats/golibvips/libvips"

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

// errFoundNewer is a sentinel error for WASM source freshness check
var errFoundNewer = errors.New("source newer than WASM")

// Builder maintains the state for site builds
type Builder struct {
	cfg *config.Config

	// Services
	cacheService  services.CacheService
	postService   services.PostService
	assetService  services.AssetService
	renderService services.RenderService

	// Legacy access if needed (or for SaveCaches/Close)
	diagramAdapter *cache.DiagramCacheAdapter

	// Structured logging
	logger *slog.Logger

	// Build metrics tracking
	metrics *metrics.BuildMetrics

	// Filesystems
	SourceFs afero.Fs
	DestFs   afero.Fs

	// Shared markdown parser pool for reuse in incremental builds
	mdPool *sync.Pool

	// Native renderer for D2/LaTeX
	nativeRenderer *native.Renderer

	// Mutex for concurrent rendering safety
	mu sync.Mutex

	// Build coordination - prevents concurrent builds during watch mode
	buildMu sync.Mutex

	// Cached data for incremental builds
	indexedPosts []models.IndexedPost

	// True when output directory did not exist at build start.
	isCleanBuild bool
}

// NewBuilder initializes a new site builder
func NewBuilder(args []string) *Builder {
	cfg := config.Load(args)
	return newBuilderWithConfig(cfg)
}

// NewBuilderWithConfig initializes a new site builder with a pre-loaded config
func NewBuilderWithConfig(cfg *config.Config) *Builder {
	return newBuilderWithConfig(cfg)
}

// newBuilderWithConfig is the internal implementation
func newBuilderWithConfig(cfg *config.Config) *Builder {
	utils.InitMinifier()

	// Initialize structured logger early
	logger := InitLogger()

	// Verify Theme Exists
	VerifyTheme(cfg, logger)

	// Initialize build metrics
	buildMetrics := metrics.NewBuildMetrics()

	// Create cache directory if it doesn't exist
	SetupCacheDirectories(cfg, logger)

	// Initialize libvips with configured concurrency
	InitLibvips(cfg, logger)

	// Open BoltDB cache
	cacheManager, diagramAdapter := SetupCacheManager(cfg, logger)

	// Create native renderer (Worker Pool)
	var nativeRenderer *native.Renderer
	if cfg.ParserWorkers > 0 {
		nativeRenderer = native.New(native.WithWorkers(cfg.ParserWorkers))
	} else {
		nativeRenderer = native.New()
	}

	// Initialize Filesystems
	sourceFs := afero.NewOsFs()
	destFs := afero.NewMemMapFs()

	// 3. Load theme metadata
	LoadThemeMetadata(cfg, sourceFs, logger)

	// Create sync.Map for diagram cache (thread-safe, no mutex needed)
	diagramCache := &sync.Map{}

	// Create core components mapping pool
	mdPool := &sync.Pool{
		New: func() interface{} {
			return mdParser.New(cfg, nativeRenderer, diagramCache)
		},
	}

	rnd := renderer.New(cfg.CompressImages, destFs, cfg.TemplateDir, cfg.IsDev, logger)

	// Create Services
	var cacheSvc services.CacheService
	if cacheManager != nil {
		cacheSvc = services.NewCacheService(cacheManager, logger)
	}

	renderSvc := services.NewRenderService(rnd, logger)
	assetSvc := services.NewAssetService(sourceFs, destFs, cfg, renderSvc, logger)
	postSvc := services.NewPostService(cfg, cacheSvc, renderSvc, logger, buildMetrics, mdPool, nativeRenderer, sourceFs, destFs, diagramAdapter)

	builder := &Builder{
		cfg:            cfg,
		cacheService:   cacheSvc,
		postService:    postSvc,
		assetService:   assetSvc,
		renderService:  renderSvc,
		diagramAdapter: diagramAdapter,
		logger:         logger,
		metrics:        buildMetrics,
		SourceFs:       sourceFs,
		DestFs:         destFs,
		mdPool:         mdPool,
		nativeRenderer: nativeRenderer,
	}

	return builder
}

// See generateCacheID in setup.go

func (b *Builder) Config() *config.Config {
	return b.cfg
}

// getFaviconPath returns the favicon path - uses custom logo if set, otherwise defaults to theme favicon
func (b *Builder) getFaviconPath() string {
	if b.cfg.Logo != "" {
		return b.cfg.Logo
	}
	return filepath.Join(b.cfg.ThemeDir, b.cfg.Theme, "static", "images", "favicon.png")
}

// checkWasmUpdate checks if Search WASM needs rebuild based on source hash.
func (b *Builder) checkWasmUpdate() {
	wasmSrcDirs := []string{
		"cmd/search",
		"builder/search",
		"builder/models",
	}

	// Detect if we are in the source tree
	inSourceTree := false
	if _, err := os.Stat(filepath.Join("cmd", "search", "main.go")); err == nil {
		inSourceTree = true
	}

	if !inSourceTree {
		build.CheckWASM("")
		return
	}

	// Optimization: Check if WASM exists and is newer than source
	// This skips hashing entirely if not needed
	wasmPath := "static/wasm/search.wasm"
	if wasmInfo, err := os.Stat(wasmPath); err == nil {
		isFresh := true

		for _, dir := range wasmSrcDirs {
			err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				info, err := d.Info()
				if err != nil {
					return err
				}
				if info.ModTime().After(wasmInfo.ModTime()) {
					return errFoundNewer
				}
				return nil
			})
			if errors.Is(err, errFoundNewer) {
				isFresh = false
				break
			}
		}

		if isFresh {
			return
		}
	}

	// Use Fast Hash (Metadata) for quick check
	currentHash, err := utils.HashDirsFast(wasmSrcDirs)
	if err != nil {
		b.logger.Warn("Failed to calculate WASM source hash", "error", err)
		return
	}

	// Use BoltDB if available
	var storedHash string
	if b.cacheService != nil {
		storedHash, _ = b.cacheService.GetWasmHash()
	}

	if currentHash != storedHash {
		// Only trigger rebuild if hash changed
		if inSourceTree {
			// Try to compile from source (developer mode)
			if err := build.CompileWASMFromSource(context.Background(), "./cmd/search", "static/wasm/search.wasm"); err == nil {
				if b.cacheService != nil {
					_ = b.cacheService.SetWasmHash(currentHash)
				}
				// Also update the compressed version
				_ = build.CompressGzip("static/wasm/search.wasm", "static/wasm/search.wasm.gz")
				return
			}
			b.logger.Warn("Automatic WASM compilation failed, falling back to embedded version")
		}

		if build.CheckWASM("") {
			if b.cacheService != nil {
				if err := b.cacheService.SetWasmHash(currentHash); err != nil {
					b.logger.Warn("Failed to store WASM hash", "error", err)
				}
			}
		}
	}
}

// SetDevMode enables/disables development mode (affects CSS hashing)
func (b *Builder) SetDevMode(isDev bool) {
	b.cfg.IsDev = isDev
}

// SaveCaches persists all caches
func (b *Builder) SaveCaches() {
	// Flush diagram adapter to BoltDB
	if b.diagramAdapter != nil {
		if b.isCleanBuild {
			b.logger.Info("Skipping diagram cache flush for clean build")
		} else {
			if err := b.diagramAdapter.Flush(); err != nil {
				b.logger.Warn("Failed to flush diagram cache", "error", err)
			}
		}
	}

	// Increment build count
	if b.cacheService != nil {
		_ = b.cacheService.IncrementBuildCount()
		b.cacheService.ClearDirty()
	}

	// Flush cache service if needed
	// (Our current implementation just wraps Manager which is closed below)

	// Record end time
	b.metrics.RecordEnd()

	// Only print metrics in non-dev mode or on full builds
	if !b.cfg.IsDev {
		b.metrics.Print()
	}

	b.logger.Info("Saved caches", "path", b.cfg.CacheDir)
}

// Close cleans up resources
func (b *Builder) Close() {
	if b.diagramAdapter != nil {
		if err := b.diagramAdapter.Close(); err != nil {
			b.logger.Warn("Failed to close diagram cache", "error", err)
		}
	}
	if b.cacheService != nil {
		_ = b.cacheService.Close()
	}
	libvips.Shutdown()
}

// See initLibvips in setup.go

// Run executes the main build logic
func Run(args []string) {
	b := NewBuilder(args)
	defer b.Close()
	defer b.SaveCaches()
	if err := b.Build(context.Background()); err != nil {
		b.logger.Error("Build failed", "error", err)
	}
}
