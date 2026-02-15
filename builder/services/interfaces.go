package services

import (
	"context"
	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
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

// PostService defines operations for processing markdown posts
type PostService interface {
	Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool) (*PostResult, error)
	ProcessSingle(ctx context.Context, path string) error
	RenderCachedPosts()
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
	GetGraphHash() (string, error)
	SetGraphHash(hash string) error
	GetWasmHash() (string, error)
	SetWasmHash(hash string) error

	// Write operations
	StoreHTML(content []byte) (string, error)
	StoreHTMLForPost(post *cache.PostMeta, content []byte) error
	StoreHTMLForPostDirect(content []byte) (string, error)
	BatchCommit(posts []*cache.PostMeta, records map[string]*cache.SearchRecord, deps map[string]*cache.Dependencies) error
	DeletePost(postID string) error

	// Dirty tracking
	MarkDirty(postID string)
	IsDirty(postID string) bool

	// Lifecycle
	Stats() (*cache.CacheStats, error)
	IncrementBuildCount() error
	Close() error
}

// AssetService handles static asset processing
type AssetService interface {
	Build(ctx context.Context) error
}

// RenderService handles rendering logic
type RenderService interface {
	RenderPage(path string, data models.PageData)
	RenderIndex(path string, data models.PageData)
	Render404(path string, data models.PageData)
	RenderGraph(path string, data models.PageData)
	RegisterFile(path string)
	SetAssets(assets map[string]string)
	GetAssets() map[string]string
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
}
