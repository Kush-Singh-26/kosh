package run

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/afero"
	"github.com/yuin/goldmark"

	"my-ssg/builder/config"
	"my-ssg/builder/models"
	mdParser "my-ssg/builder/parser"
	"my-ssg/builder/renderer"
	"my-ssg/builder/renderer/native"
	"my-ssg/builder/utils"
)

// Builder maintains the state for site builds
type Builder struct {
	cfg             *config.Config
	socialCardCache *utils.SocialCardCache
	buildCache      *models.MetadataCache
	md              goldmark.Markdown
	rnd             *renderer.Renderer
	native          *native.Renderer
	mu              sync.Mutex
	SourceFs        afero.Fs
	DestFs          afero.Fs
}

// NewBuilder initializes a new site builder
func NewBuilder(args []string) *Builder {
	cfg := config.Load(args)
	utils.InitMinifier()

	// Create cache directory if it doesn't exist
	_ = os.MkdirAll(".kosh-cache", 0755)

	// Load caches from separate directory (not deployed to production)
	socialCardCache, cacheErr := utils.LoadSocialCardCache(".kosh-cache/.social-card-cache.json")
	if cacheErr != nil {
		fmt.Printf("Warning: Failed to load social card cache: %v\n", cacheErr)
		socialCardCache = utils.NewSocialCardCache()
	}

	buildCache, cacheErr := utils.LoadBuildCache(".kosh-cache/.kosh-build-cache.json")
	if cacheErr != nil {
		fmt.Printf("Warning: Failed to load build cache: %v\n", cacheErr)
		buildCache = &models.MetadataCache{
			Posts: make(map[string]models.CachedPost),
		}
	}

	// Init Dependencies
	if buildCache.Dependencies.Tags == nil {
		buildCache.Dependencies.Tags = make(map[string][]string)
	}
	if buildCache.Dependencies.Templates == nil {
		buildCache.Dependencies.Templates = make(map[string][]string)
	}
	if buildCache.Dependencies.Assets == nil {
		buildCache.Dependencies.Assets = make(map[string][]string)
	}

	// Smart Cache Invalidation: If BaseURL changed, force rebuild everything
	if buildCache.BaseURL != cfg.BaseURL {
		fmt.Printf("🔄 BaseURL changed (%s -> %s). Forcing full rebuild to update asset paths.\n", buildCache.BaseURL, cfg.BaseURL)
		cfg.ForceRebuild = true
		buildCache.BaseURL = cfg.BaseURL
	}

	// Initialize diagram cache if nil
	if buildCache.DiagramCache == nil {
		buildCache.DiagramCache = make(map[string]string)
	}

	// Create native renderer (Worker Pool)
	nativeRenderer := native.New()

	// Initialize Filesystems
	sourceFs := afero.NewOsFs()
	destFs := afero.NewMemMapFs()

	builder := &Builder{
		cfg:             cfg,
		socialCardCache: socialCardCache,
		buildCache:      buildCache,
		md:              mdParser.New(cfg.BaseURL, nativeRenderer, buildCache.DiagramCache, &sync.Mutex{}),
		rnd:             renderer.New(cfg.CompressImages, destFs, cfg.TemplateDir),
		native:          nativeRenderer,
		SourceFs:        sourceFs,
		DestFs:          destFs,
	}

	return builder
}

// Config returns the builder's configuration
func (b *Builder) Config() *config.Config {
	return b.cfg
}

// SetDevMode enables/disables development mode (affects CSS hashing)
func (b *Builder) SetDevMode(isDev bool) {
	b.cfg.IsDev = isDev
}

// SaveCaches persists the build and social card caches to disk
func (b *Builder) SaveCaches() {
	// Ensure cache directory exists
	_ = os.MkdirAll(".kosh-cache", 0755)

	if saveErr := utils.SaveSocialCardCache(".kosh-cache/.social-card-cache.json", b.socialCardCache); saveErr != nil {
		fmt.Printf("Warning: Failed to save social card cache: %v\n", saveErr)
	}
	if saveErr := utils.SaveBuildCache(".kosh-cache/.kosh-build-cache.json", b.buildCache); saveErr != nil {
		fmt.Printf("Warning: Failed to save build cache: %v\n", saveErr)
	}
	fmt.Printf("   💾 Saved build cache to .kosh-cache/\n")
}

// Run executes the main build logic
func Run(args []string) {
	b := NewBuilder(args)
	defer b.SaveCaches()
	b.Build()
}
