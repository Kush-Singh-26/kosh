package services

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"

	"github.com/spf13/afero"
)

// PostResult contains the aggregated results of post processing
type PostResult struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	TagMap         map[string][]models.PostMetadata
	IndexedPosts   []models.IndexedPost
	AnyPostChanged bool
	Has404         bool
}

// MetadataContext holds aggregated post metadata ready for site-wide generators.
// This allows site-wide generators to overlap with the post render phase.
type MetadataContext struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	TagMap         map[string][]models.PostMetadata
	IndexedPosts   []models.IndexedPost
	AnyPostChanged bool
}

// MetadataScanner scans content directory for markdown files and extracts metadata.
type MetadataScanner interface {
	Scan(ctx context.Context, contentDir string, srcFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error)
	ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error)
}

// IsCacheMiss returns true if the error is a cache miss (sentinel ErrNoContent).
func IsCacheMiss(err error) bool {
	return errors.Is(err, core.ErrNoContent)
}

// CacheService provides cache operations for all services.
//
// Error Contract:
//   - Read errors: returns IsCacheMiss(err) == true if key not found, other errors for I/O or corruption
//   - Write errors: returned (disk full, I/O error)
//   - BatchCommit: atomic - all or nothing, returns error on any failure
//   - Dirty tracking: in-memory only, never fails (lost on crash)
//
// Concurrency:
//   - All methods are safe for concurrent calls
//   - Underlying bbolt DB handles its own synchronization
type CacheService interface {
	models.PostCache
	models.SearchCache
	models.SocialCardCache
	models.BuildArtifactCache

	GetGraphHash() (string, error)
	SetGraphHash(hash string) error
	GetWasmHash() (string, error)
	SetWasmHash(hash string) error

	Stats() (*cache.CacheStats, error)
	IncrementBuildCount() error
	Close() error
}

// PostServiceCache is a narrowed interface for PostService.
type PostServiceCache interface {
	models.PostCache
	models.SearchCache
	models.SocialCardCache
	models.BuildArtifactCache
}

// CacheServiceDependencies holds all dependencies for CacheService.
type CacheServiceDependencies struct {
	Ctx     *buildCtx.BuildContext
	Manager *cache.Manager
	Logger  *slog.Logger
}

// PostServiceDependencies holds all dependencies for PostService.
// Using a struct pattern for API coherence and easier testing.
type PostServiceDependencies struct {
	Ctx            *buildCtx.BuildContext
	Cfg            *config.Config
	Cache          PostServiceCache
	Renderer       RenderService
	Logger         *slog.Logger
	Metrics        *metrics.BuildMetrics
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
	SourceFs       afero.Fs
	Sink           fspkg.ArtifactSink
	DiagramAdapter *cache.DiagramCacheAdapter
}

// PostService handles markdown parsing and post processing.
//
// Error Contract:
//   - Parse errors: returned immediately, caller must handle
//   - Cache errors: logged, build continues (best-effort)
//   - Filesystem errors: returned, build halts
//
// Concurrency:
//   - Process: safe for concurrent calls (streaming mode)
//   - ProcessSingle: not thread-safe for same path
//   - Internal state protected by worker pool serialization
type PostService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	// Consolidates sink and filesystem injection into a single explicit call.
	ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs)

	// SetAssetsGate sets the channel to wait on before rendering.
	SetAssetsGate(ch <-chan struct{})

	// Process handles streaming post processing.
	// Returns error only for critical failures (context cancelled, pool exhausted).
	// Individual post errors are logged and skipped.
	Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*PostResult, error)

	// ProcessSingle processes a single post file.
	// Returns error if file cannot be read or parsed.
	ProcessSingle(ctx context.Context, path string, source []byte) error

	// ProcessSingleWithResult processes a single post and populates result.
	// Returns error if parsing or rendering fails.
	ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *ParsedMarkdownResult) error

	// WaitForCacheCommit blocks until async cache commit completes.
	// Safe to call even if no commit is pending.
	WaitForCacheCommit()
}

// RenderServiceDependencies holds all dependencies for RenderService.
// Using a struct pattern for API coherence and easier testing.
type RenderServiceDependencies struct {
	Ctx      *buildCtx.BuildContext
	Renderer *renderer.Renderer
	Logger   *slog.Logger
}

// RenderService handles template rendering and HTML generation.
//
// Error Contract:
//   - Template errors: returned immediately (missing template, parse error)
//   - Render errors: returned (data binding failure, I/O error)
//   - Asset timeout: returned after 30s wait for assets
//
// Concurrency:
//   - All render methods are safe for concurrent calls
//   - Template reload is serialized internally
//   - Asset map access protected by renderer's internal mutex
type RenderService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs)

	// SetAssetsGate sets the channel to wait on before rendering.
	// Channel is owned by AssetService and closed when assets are ready.
	SetAssetsGate(ch <-chan struct{})

	// RenderPage renders a single post page.
	// Returns error if template is missing or data binding fails.
	RenderPage(path string, data models.PageData) error

	// RenderIndex renders the index/pagination page.
	// Returns error if template is missing or data binding fails.
	RenderIndex(path string, data models.PageData) error

	// Render404 renders the 404 page.
	// Returns error if template is missing or data binding fails.
	Render404(path string, data models.PageData) error

	// RenderGraph renders the graph view page.
	// Returns error if template is missing or data binding fails.
	RenderGraph(path string, data models.PageData) error

	// RenderSidebar renders the navigation sidebar from tree.
	// Returns empty HTML on error (never fails).
	RenderSidebar(tree []*models.TreeNode) template.HTML

	// RegisterFile marks a file for inclusion in build output.
	RegisterFile(path string)

	// Asset methods for template rendering.
	// These methods delegate directly to the underlying *renderer.Renderer.
	SetAssets(assets map[string]string)
	GetAssets() map[string]string

	// File tracking for incremental builds.
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()

	// Template lifecycle.
	// ReloadTemplates reloads all templates from disk.
	// Returns error if any template fails to parse.
	ReloadTemplates()

	// Has404Template returns true if the 404.html template was successfully loaded.
	// This allows the build pipeline to render a 404 page even without a content/404.md file.
	Has404Template() bool
}

// AssetServiceDependencies holds all dependencies for AssetService.
// Using a struct pattern for API coherence and easier testing.
//
// Channel Ownership:
//   - AssetsReady: created by caller, passed via WithAssetsReadySignal option
//   - ContentAssetsChan: created by caller (Scanner), passed via WithContentAssetsChannel
type AssetServiceDependencies struct {
	Ctx      *buildCtx.BuildContext
	SourceFs afero.Fs
	Sink     fspkg.ArtifactSink
	Cfg      *config.Config
	Renderer RenderService
	Logger   *slog.Logger
	Metrics  *metrics.BuildMetrics
}

// AssetService handles static asset processing (CSS/JS bundling, image optimization).
//
// Error Contract:
//   - Build errors: returned (esbuild failure, image processing error)
//   - Copy errors: returned (file not found, permission denied)
//   - Asset discovery: logged, build continues (best-effort for missing assets)
//
// Concurrency:
//   - Build: not thread-safe, must be called once per build cycle
//   - BuildForAssetChange: safe for concurrent calls (singleflight)
//   - ReconfigureForBuild: not thread-safe, call before build starts
//
// Channel Ownership:
//   - AssetsReadySignal: owned by AssetService, closed when assets are ready
//   - ContentAssetsChannel: owned by caller (Scanner), AssetService reads only
type AssetService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs)

	// SetMetrics sets the build metrics collector.
	SetMetrics(m *metrics.BuildMetrics)

	// SetAssetsReadySignal sets the signal channel to close when assets are ready.
	// Channel must be created by caller and passed in before Build() is called.
	SetAssetsReadySignal(ch chan struct{})

	// SetContentAssetsChannel sets the channel for receiving discovered assets.
	// Channel is owned by caller (Scanner), AssetService only reads.
	SetContentAssetsChannel(ch <-chan []models.ScannedAsset)

	// Build processes all assets (CSS/JS bundle, images, static files).
	// Closes the AssetsReadySignal channel when complete.
	// Returns error for critical failures (esbuild hung, disk full).
	Build(ctx context.Context) error

	// BuildForAssetChange processes a single changed asset.
	// Returns updated asset map for incremental HTML rerender.
	// Returns error if asset processing fails.
	BuildForAssetChange(ctx context.Context) (map[string]string, error)
}

// WasmService handles WASM compilation and deployment for Search.
// Isolate WASM logic from core build loop to adhere to single-responsibility principle.
//
// Error Contract:
//   - CheckAndUpdate: returns error if compilation fails or source cannot be read.
//   - Deploy: returns error if staging directory is missing or I/O failure.
//
// Concurrency:
//   - Methods are safe for concurrent calls via internal locking.
type WasmService interface {
	// CheckAndUpdate compiles WASM if source changed or missing.
	CheckAndUpdate(ctx context.Context) error
	// Deploy copies WASM to staging directory.
	Deploy(ctx context.Context, sink fspkg.ArtifactSink) error
	// SetSearchSourceDirty marks the search source as needing recompilation.
	SetSearchSourceDirty(dirty bool)
}
