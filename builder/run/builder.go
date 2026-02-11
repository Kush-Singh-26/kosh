package run

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"

	"my-ssg/builder/cache"
	"my-ssg/builder/config"
	"my-ssg/builder/metrics"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/renderer"
	"my-ssg/builder/renderer/native"
	"my-ssg/builder/utils"
	"my-ssg/internal/build"
)

// Builder maintains the state for site builds
type Builder struct {
	cfg *config.Config

	// BoltDB-based cache
	cacheManager   *cache.Manager
	diagramAdapter *cache.DiagramCacheAdapter

	// Structured logging
	logger *slog.Logger

	// Build metrics tracking
	metrics *metrics.BuildMetrics

	md       goldmark.Markdown
	rnd      *renderer.Renderer
	native   *native.Renderer
	mu       sync.Mutex
	SourceFs afero.Fs
	DestFs   afero.Fs
}

// NewBuilder initializes a new site builder
func NewBuilder(args []string) *Builder {
	cfg := config.Load(args)
	utils.InitMinifier()

	// Initialize structured logger early
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Verify Theme Exists (Early Fail)
	themePath := filepath.Join(cfg.ThemeDir, cfg.Theme)
	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		logger.Error("Theme not found", "theme", cfg.Theme, "path", themePath, "hint", "Please ensure you have cloned/downloaded the theme into '"+cfg.ThemeDir+"'")
		os.Exit(1)
	}

	// Initialize build metrics
	buildMetrics := metrics.NewBuildMetrics()

	// Create cache directory if it doesn't exist
	_ = os.MkdirAll(".kosh-cache", 0755)
	_ = os.MkdirAll(".kosh-cache/social-cards", 0755)
	_ = os.MkdirAll(".kosh-cache/assets", 0755)
	_ = os.MkdirAll(".kosh-cache/images", 0755)
	_ = os.MkdirAll(".kosh-cache/pwa-icons", 0755)

	// Open BoltDB cache
	var cacheManager *cache.Manager
	var diagramAdapter *cache.DiagramCacheAdapter

	cm, err := cache.Open(".kosh-cache", cfg.IsDev)
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

	// Use the adapter's map for markdown parser
	var diagramCache map[string]string
	if diagramAdapter != nil {
		diagramCache = diagramAdapter.AsMap()
	} else {
		diagramCache = make(map[string]string)
	}

	builder := &Builder{
		cfg:            cfg,
		cacheManager:   cacheManager,
		diagramAdapter: diagramAdapter,
		logger:         logger,
		metrics:        buildMetrics,
		md:             mdParser.New(cfg.BaseURL, nativeRenderer, diagramCache, &sync.Mutex{}),
		rnd:            renderer.New(cfg.CompressImages, destFs, cfg.TemplateDir, logger),
		native:         nativeRenderer,
		SourceFs:       sourceFs,
		DestFs:         destFs,
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

// CacheManager returns the cache manager (may be nil if unavailable)
func (b *Builder) CacheManager() *cache.Manager {
	return b.cacheManager
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
	// Optimization: Check if WASM exists (in source/static) and is newer than source
	// This skips hashing entirely if not needed.
	// We check 'static/wasm/search.wasm' because 'public' might be cleaned,
	// but the intermediate build artifact in 'static' should persist.
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
			if err == errFoundNewer {
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
	if b.cacheManager != nil {
		storedHash, _ = b.cacheManager.GetWasmHash()
	}

	if currentHash != storedHash {
		// Only trigger rebuild if hash changed
		if build.CheckWASM("") {
			if b.cacheManager != nil {
				if err := b.cacheManager.SetWasmHash(currentHash); err != nil {
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
	if b.cacheManager != nil {
		_ = b.cacheManager.IncrementBuildCount()
	}

	// Record end time
	b.metrics.RecordEnd()

	// Only print metrics in non-dev mode or on full builds
	if !b.cfg.IsDev {
		b.metrics.Print()
	}

	b.logger.Info("Saved caches", "path", ".kosh-cache/")
}

// Close cleans up resources
func (b *Builder) Close() {
	if b.cacheManager != nil {
		_ = b.cacheManager.Close()
	}
}

// Run executes the main build logic
func Run(args []string) {
	b := NewBuilder(args)
	defer b.Close()
	defer b.SaveCaches()
	b.Build()
}
