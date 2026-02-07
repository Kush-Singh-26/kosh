package run

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark"

	"my-ssg/builder/cache"
	"my-ssg/builder/config"
	"my-ssg/builder/models"
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

	// In-memory build cache (used by processPosts pipeline)
	buildCache *models.MetadataCache

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

	// Create cache directory if it doesn't exist
	_ = os.MkdirAll(".kosh-cache", 0755)

	// Open BoltDB cache
	var cacheManager *cache.Manager
	var diagramAdapter *cache.DiagramCacheAdapter

	cm, err := cache.Open(".kosh-cache")
	if err != nil {
		fmt.Printf("⚠️  Failed to open cache database: %v. Using in-memory cache.\n", err)
	} else {
		cacheManager = cm

		// Generate and verify cache ID
		cacheID := generateCacheID(cfg)
		needsRebuild, _ := cacheManager.VerifyCacheID(cacheID)
		if needsRebuild {
			fmt.Printf("🔄 Cache fingerprint changed. Triggering rebuild.\n")
			cfg.ForceRebuild = true
			cacheManager.SetCacheID(cacheID)
		}

		diagramAdapter = cache.NewDiagramCacheAdapter(cacheManager)
	}

	// In-memory build cache (used by processPosts pipeline)
	buildCache := &models.MetadataCache{
		Posts:        make(map[string]models.CachedPost),
		DiagramCache: make(map[string]string),
		Dependencies: models.DependencyGraph{
			Tags:      make(map[string][]string),
			Templates: make(map[string][]string),
			Assets:    make(map[string][]string),
		},
		BaseURL: cfg.BaseURL,
	}

	// Create native renderer (Worker Pool)
	nativeRenderer := native.New()

	// Initialize Filesystems
	sourceFs := afero.NewOsFs()
	destFs := afero.NewMemMapFs()

	// Use the adapter's map for markdown parser
	var diagramCache map[string]string
	if diagramAdapter != nil {
		diagramCache = diagramAdapter.AsMap()
	} else {
		diagramCache = buildCache.DiagramCache
	}

	builder := &Builder{
		cfg:            cfg,
		cacheManager:   cacheManager,
		diagramAdapter: diagramAdapter,
		buildCache:     buildCache,
		md:             mdParser.New(cfg.BaseURL, nativeRenderer, diagramCache, &sync.Mutex{}),
		rnd:            renderer.New(cfg.CompressImages, destFs, cfg.TemplateDir),
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
	currentHash, err := utils.HashDirs(wasmSrcDirs)
	if err != nil {
		fmt.Printf("⚠️ Failed to calculate WASM source hash: %v\n", err)
		return
	}

	if currentHash != b.buildCache.WasmHash {
		if build.CheckWASM("") {
			b.mu.Lock()
			b.buildCache.WasmHash = currentHash
			b.mu.Unlock()
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
		if err := b.diagramAdapter.Flush(); err != nil {
			fmt.Printf("Warning: Failed to flush diagram cache: %v\n", err)
		}
	}

	// Increment build count
	if b.cacheManager != nil {
		b.cacheManager.IncrementBuildCount()
	}

	fmt.Printf("   💾 Saved caches to .kosh-cache/\n")
}

// Close cleans up resources
func (b *Builder) Close() {
	if b.cacheManager != nil {
		b.cacheManager.Close()
	}
}

// Run executes the main build logic
func Run(args []string) {
	b := NewBuilder(args)
	defer b.Close()
	defer b.SaveCaches()
	b.Build()
}
