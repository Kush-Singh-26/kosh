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

// MetadataReadyFunc is called when post metadata becomes available (after parse,
// before render). This allows site-wide tasks (sitemap, RSS, search, pagination,
// tags, PWA) to overlap with the post render phase.
type MetadataReadyFunc func(allPosts []models.PostMetadata, pinnedPosts []models.PostMetadata, tagMap map[string][]models.PostMetadata, indexedPosts []models.IndexedPost, anyChanged bool)

type PostService interface {
	SetSink(sink utils.ArtifactSink)
	SetSourceFs(fs afero.Fs)
	SetAssetsGate(ch <-chan struct{})
	SetMetadataCallback(fn MetadataReadyFunc)
	Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile, has404 bool) (*PostResult, error)
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
	SetSink(sink utils.ArtifactSink)
	SetSourceFs(fs afero.Fs)
	SetMetrics(m *metrics.BuildMetrics)
	SetAssetsReadySignal(ch chan struct{})
	SetContentAssetsChannel(ch <-chan []models.ScannedAsset)
	Build(ctx context.Context) error
	BuildForAssetChange(ctx context.Context) (map[string]string, error)
}

// RenderService handles rendering logic
type RenderService interface {
	SetAssetsGate(ch <-chan struct{})
	SetSink(sink utils.ArtifactSink)
	SetSourceFs(fs afero.Fs)
	RenderPage(path string, data models.PageData)
	RenderIndex(path string, data models.PageData)
	Render404(path string, data models.PageData)
	RenderGraph(path string, data models.PageData)
	RenderSidebar(tree []*models.TreeNode) template.HTML
	RegisterFile(path string)
	SetAssets(assets map[string]string)
	GetAssets() map[string]string
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
	ReloadTemplates()
}
