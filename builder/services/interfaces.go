package services

import (
	"context"
	"html/template"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
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

// Service-specific cache interfaces for interface segregation.
// Each service depends only on the cache operations it actually uses.

// PostServiceCache provides cache operations needed by PostService
type PostServiceCache interface {
	PostCache
	ContentCache
	SocialCardCache
	DirtyTracker
	Stats() (*cache.CacheStats, error)
	IncrementBuildCount() error
	Close() error
}

// RenderServiceCache provides cache operations needed by RenderService
type RenderServiceCache interface {
	ContentCache
}

// AssetServiceCache provides cache operations needed by AssetService
type AssetServiceCache interface {
	BuildArtifactCache
}

// ScannerCache provides cache operations needed by Scanner
type ScannerCache interface {
	PostCache
}

// SiteGeneratorCache provides cache operations needed by site-wide generators
type SiteGeneratorCache interface {
	BuildArtifactCache
	PostCache
}

type PostService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	// Consolidates sink and filesystem injection into a single explicit call.
	ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs)

	// SetAssetsGate sets the channel to wait on before rendering.
	SetAssetsGate(ch <-chan struct{})

	Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*PostResult, error)
	ProcessSingle(ctx context.Context, path string, source []byte) error
	ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *ParsedMarkdownResult) error
	WaitForCacheCommit()
}

// PostCache provides post metadata query operations
type PostCache interface {
	GetPost(id string) (*cache.PostMeta, error)
	ListAllPosts() ([]string, error)
	GetPostByPath(path string) (*cache.PostMeta, error)
	GetPostsByIDs(ids []string) (map[string]*cache.PostMeta, error)
	GetPostsByTemplate(templatePath string) ([]string, error)
	GetPostsMetadataByVersion(version string) ([]cache.PostListMeta, error)
}

// ContentCache provides HTML content and search record operations
type ContentCache interface {
	GetSearchRecords(ids []string) (map[string]*cache.SearchRecord, error)
	GetSearchRecord(id string) (*cache.SearchRecord, error)
	GetHTMLContent(post *cache.PostMeta) ([]byte, error)
	StoreHTML(content []byte) (string, error)
	StoreHTMLForPost(post *cache.PostMeta, content []byte) error
	BatchCommit(posts []*cache.PostMeta, records map[string]*cache.SearchRecord, deps map[string]*cache.Dependencies) error
	DeletePost(postID string) error
}

// SocialCardCache provides social card hash operations
type SocialCardCache interface {
	GetSocialCardHash(path string) (string, error)
	SetSocialCardHash(path, hash string) error
	BatchSetSocialCardHashes(hashes map[string]string) error
}

// BuildArtifactCache provides build artifact hash operations (graph, WASM)
type BuildArtifactCache interface {
	GetGraphHash() (string, error)
	SetGraphHash(hash string) error
	GetWasmHash() (string, error)
	SetWasmHash(hash string) error
}

// DirtyTracker provides dirty tracking for incremental builds
type DirtyTracker interface {
	MarkDirty(postID string)
	IsDirty(postID string) bool
	ClearDirty()
}

// AssetService handles static asset processing
type AssetService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs)

	SetMetrics(m *metrics.BuildMetrics)
	SetAssetsReadySignal(ch chan struct{})
	SetContentAssetsChannel(ch <-chan []models.ScannedAsset)
	Build(ctx context.Context) error
	BuildForAssetChange(ctx context.Context) (map[string]string, error)
}

// RenderService handles rendering logic
type RenderService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs)

	// SetAssetsGate sets the channel to wait on before rendering.
	// Channel is owned by AssetService and closed when assets are ready.
	SetAssetsGate(ch <-chan struct{})

	RenderPage(path string, data models.PageData) error
	RenderIndex(path string, data models.PageData) error
	Render404(path string, data models.PageData) error
	RenderGraph(path string, data models.PageData) error
	RenderSidebar(tree []*models.TreeNode) template.HTML
	RegisterFile(path string)

	// Asset methods for template rendering.
	// These methods delegate directly to the underlying *renderer.Renderer.
	SetAssets(assets map[string]string)
	GetAssets() map[string]string

	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
	ReloadTemplates()
}
