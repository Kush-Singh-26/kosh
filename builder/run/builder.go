package run

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
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

	// Shared markdown parser for reuse in incremental builds
	md goldmark.Markdown

	// Build coordination - prevents concurrent builds during watch mode
	buildMu sync.Mutex
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
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Verify Theme Exists (Early Fail)
	themePath := filepath.Join(cfg.ThemeDir, cfg.Theme)
	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		logger.Error("Theme not found",
			"theme", cfg.Theme,
			"path", themePath,
			"hint", "Please ensure you have installed the theme into '"+cfg.ThemeDir+"/"+cfg.Theme+"/'")
		logger.Info("Theme installation:", "example", "git clone <theme-repo-url> "+filepath.Join(cfg.ThemeDir, cfg.Theme))
		os.Exit(1)
	}

	// Verify required theme directories exist
	templatePath := cfg.TemplateDir
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		logger.Error("Theme templates directory not found",
			"theme", cfg.Theme,
			"path", templatePath,
			"hint", "Theme must have a 'templates' directory")
		os.Exit(1)
	}

	staticPath := cfg.StaticDir
	if _, err := os.Stat(staticPath); os.IsNotExist(err) {
		logger.Warn("Theme static directory not found, creating empty",
			"theme", cfg.Theme,
			"path", staticPath)
		_ = os.MkdirAll(staticPath, 0755)
	}

	// Initialize build metrics
	buildMetrics := metrics.NewBuildMetrics()

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
		logger.Error("Failed to create cache directory", "path", cfg.CacheDir, "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Join(cfg.CacheDir, "social-cards"), 0755); err != nil {
		logger.Error("Failed to create social-cards cache directory", "error", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.CacheDir, "assets"), 0755); err != nil {
		logger.Error("Failed to create assets cache directory", "error", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.CacheDir, "images"), 0755); err != nil {
		logger.Error("Failed to create images cache directory", "error", err)
	}
	if err := os.MkdirAll(filepath.Join(cfg.CacheDir, "pwa-icons"), 0755); err != nil {
		logger.Error("Failed to create pwa-icons cache directory", "error", err)
	}

	// Open BoltDB cache
	var cacheManager *cache.Manager
	var diagramAdapter *cache.DiagramCacheAdapter

	cm, err := cache.Open(cfg.CacheDir, cfg.IsDev)
	if err != nil {
		logger.Warn("Failed to open cache database, using in-memory cache", "error", err)
	} else {
		cacheManager = cm

		// Generate and verify cache ID
		cacheID := generateCacheID(cfg)
		needsRebuild, _ := cacheManager.VerifyCacheID(cacheID)
		if needsRebuild {
			logger.Info("Cache fingerprint changed, triggering rebuild")
			cfg.ForceRebuild = true
			_ = cacheManager.SetCacheID(cacheID)
		}

		diagramAdapter = cache.NewDiagramCacheAdapter(cacheManager)
	}

	// Create native renderer (Worker Pool)
	nativeRenderer := native.New()

	// Initialize Filesystems
	sourceFs := afero.NewOsFs()
	destFs := afero.NewMemMapFs()

	// 3. Load theme metadata
	themeMetadata := config.ThemeConfig{
		Name:               cfg.Theme,
		SupportsVersioning: false,
	}
	themeYamlPath := filepath.Join(themePath, "theme.yaml")
	if data, err := afero.ReadFile(sourceFs, themeYamlPath); err == nil {
		if err := yaml.Unmarshal(data, &themeMetadata); err != nil {
			logger.Warn("Failed to parse theme.yaml", "error", err)
		}
	}
	cfg.ThemeMetadata = themeMetadata

	// Create sync.Map for diagram cache (thread-safe, no mutex needed)
	diagramCache := &sync.Map{}

	// Create core components
	md := mdParser.New(cfg.BaseURL, nativeRenderer, diagramCache)
	rnd := renderer.New(cfg.CompressImages, destFs, cfg.TemplateDir, logger)

	// Create Services
	var cacheSvc services.CacheService
	if cacheManager != nil {
		cacheSvc = services.NewCacheService(cacheManager, logger)
	}

	renderSvc := services.NewRenderService(rnd, logger)
	assetSvc := services.NewAssetService(sourceFs, destFs, cfg, renderSvc, logger)
	postSvc := services.NewPostService(cfg, cacheSvc, renderSvc, logger, buildMetrics, md, nativeRenderer, sourceFs, destFs, diagramAdapter)

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
		md:             md,
	}

	return builder
}

// generateCacheID creates a fingerprint of all dependencies that affect output
func generateCacheID(cfg *config.Config) string {
	// Combine versions of all SSR dependencies
	components := []string{
		"kosh:1.0",
		"goldmark:1.7",
		"d2:0.7",
		"katex:embedded",
	}

	combined := ""
	for _, c := range components {
		combined += c + "|"
	}

	return cache.HashString(combined)
}

// Config returns the builder's configuration
func (b *Builder) Config() *config.Config {
	return b.cfg
}

// checkWasmUpdate checks if Search WASM needs rebuild based on source hash.
func (b *Builder) checkWasmUpdate() {
	wasmSrcDirs := []string{
		"cmd/search",
		"builder/search",
		"builder/models",
	}

	// Optimization: Check if WASM exists and is newer than source
	// This skips hashing entirely if not needed
	wasmPath := "static/wasm/search.wasm"
	if wasmInfo, err := os.Stat(wasmPath); err == nil {
		isFresh := true
		errFoundNewer := fmt.Errorf("newer")

		for _, dir := range wasmSrcDirs {
			err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
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
		if err := b.diagramAdapter.Close(); err != nil {
			b.logger.Warn("Failed to flush diagram cache", "error", err)
		}
	}

	// Increment build count
	if b.cacheService != nil {
		_ = b.cacheService.IncrementBuildCount()
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
	if b.cacheService != nil {
		_ = b.cacheService.Close()
	}
}

// Run executes the main build logic
func Run(args []string) {
	b := NewBuilder(args)
	defer b.Close()
	defer b.SaveCaches()
	if err := b.Build(context.Background()); err != nil {
		b.logger.Error("Build failed", "error", err)
	}
}
