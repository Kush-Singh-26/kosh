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

type PostService interface {
	// ReconfigureForBuild updates sink and source for a new build pass.
	// Consolidates sink and filesystem injection into a single explicit call.
	ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs)
	
	// SetAssetsGate sets the channel to wait on before rendering.
	SetAssetsGate(ch <-chan struct{})
	
	// MetadataReadyChan returns a channel that closes when metadata is ready.
	// Provides explicit synchronization for site-wide generators.
	MetadataReadyChan() <-chan struct{}
	
	Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*PostResult, error)
	ProcessSingle(ctx context.Context, path string, source []byte) error
	ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *ParsedMarkdownResult) error
	WaitForCacheCommit()
}

// CacheService abstracts the caching layer
type CacheService interface {
	GetPost(id string) (*cache.PostMeta, error)
	ListAllPosts() ([]string, error)
	GetPostByPath(path string) (*cache.PostMeta, error)
	GetPostsByIDs(ids []string) (map[string]*cache.PostMeta, error)
	GetPostsByTemplate(templatePath string) ([]string, error)
	GetSearchRecords(ids []string) (map[string]*cache.SearchRecord, error)
	GetSearchRecord(id string) (*cache.SearchRecord, error)
	GetHTMLContent(post *cache.PostMeta) ([]byte, error)
	GetSocialCardHash(path string) (string, error)
	SetSocialCardHash(path, hash string) error
	BatchSetSocialCardHashes(hashes map[string]string) error
	GetGraphHash() (string, error)
	SetGraphHash(hash string) error
	GetWasmHash() (string, error)
	SetWasmHash(hash string) error
	GetPostsMetadataByVersion(version string) ([]cache.PostListMeta, error)

	// Write operations
	StoreHTML(content []byte) (string, error)
	StoreHTMLForPost(post *cache.PostMeta, content []byte) error
	BatchCommit(posts []*cache.PostMeta, records map[string]*cache.SearchRecord, deps map[string]*cache.Dependencies) error
	DeletePost(postID string) error

	// Dirty tracking
	MarkDirty(postID string)
	IsDirty(postID string) bool
	ClearDirty()

	// Lifecycle
	Stats() (*cache.CacheStats, error)
	IncrementBuildCount() error
	Close() error
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
	
	// SetAssets stores the asset map for template rendering.
	// The map is owned by RenderService after this call.
	SetAssets(assets map[string]string)
	
	// GetAssets returns the current asset map.
	// Callers should treat the result as read-only and not mutate it.
	GetAssets() map[string]string
	
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
	ReloadTemplates()
}
