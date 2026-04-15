package cache

import (
	"context"
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// cacheService implements CacheService
type cacheService struct {
	ctx     *buildctx.BuildContext
	manager *cache.Manager
	logger  *slog.Logger

	// Dirty tracking using sync.Map for thread safety
	dirtyPostsMap sync.Map // postID -> bool
}

// NewService creates a new CacheService with the given dependencies.
// Using dependency struct pattern for API coherence.
func NewService(dependencies Dependencies) Service {
	return &cacheService{
		ctx:     dependencies.Ctx,
		manager: dependencies.Manager,
		logger:  dependencies.Logger,
	}
}

// GetPostByID returns a post by ID from the cache.
func (service *cacheService) GetPostByID(postID string) (*models.PostMeta, error) {
	return service.manager.GetPostByID(postID)
}

// ListAllPosts returns all post IDs in the cache.
func (service *cacheService) ListAllPosts() ([]string, error) {
	return service.manager.ListAllPosts()
}

// GetPostByPath returns a post by its source path.
func (service *cacheService) GetPostByPath(path string) (*models.PostMeta, error) {
	return service.manager.GetPostByPath(path)
}

// GetPostsByIDs returns posts for the provided IDs.
func (service *cacheService) GetPostsByIDs(postIDs []string) (map[string]*models.PostMeta, error) {
	return service.manager.GetPostsByIDs(postIDs)
}

// GetPostsByTemplate returns post IDs that depend on a template.
func (service *cacheService) GetPostsByTemplate(templatePath string) ([]string, error) {
	return service.manager.GetPostsByTemplate(templatePath)
}

// GetSearchRecords returns search records for the provided IDs.
func (service *cacheService) GetSearchRecords(postIDs []string) (map[string]*models.SearchRecord, error) {
	return service.manager.GetSearchRecords(postIDs)
}

// GetSearchRecord returns a search record by ID.
func (service *cacheService) GetSearchRecord(postID string) (*models.SearchRecord, error) {
	return service.manager.GetSearchRecord(postID)
}

// GetHTMLContent returns cached HTML for a post.
func (service *cacheService) GetHTMLContent(post *models.PostMeta) ([]byte, error) {
	return service.manager.GetHTMLContent(post)
}

// GetSocialCardHash returns the cached social card hash for a path.
func (service *cacheService) GetSocialCardHash(path string) (string, error) {
	return service.manager.GetSocialCardHash(path)
}

// SetSocialCardHash stores the social card hash for a path.
func (service *cacheService) SetSocialCardHash(path, hash string) error {
	return service.manager.SetSocialCardHash(path, hash)
}

// BatchSetSocialCardHashes stores multiple social card hashes.
func (service *cacheService) BatchSetSocialCardHashes(hashes map[string]string) error {
	return service.manager.BatchSetSocialCardHashes(hashes)
}

// GetGraphHash returns the cached graph hash.
func (service *cacheService) GetGraphHash() (string, error) {
	return service.manager.GetGraphHash()
}

// SetGraphHash stores the cached graph hash.
func (service *cacheService) SetGraphHash(hash string) error {
	return service.manager.SetGraphHash(hash)
}

// GetWasmHash returns the cached WASM hash.
func (service *cacheService) GetWasmHash() (string, error) {
	return service.manager.GetWasmHash()
}

// SetWasmHash stores the cached WASM hash.
func (service *cacheService) SetWasmHash(hash string) error {
	return service.manager.SetWasmHash(hash)
}

// GetSearchHash returns the cached search hash.
func (service *cacheService) GetSearchHash() (string, error) {
	return service.manager.GetSearchHash()
}

// SetSearchHash stores the cached search hash.
func (service *cacheService) SetSearchHash(hash string) error {
	return service.manager.SetSearchHash(hash)
}

// StoreHTML stores HTML content and returns its hash.
func (service *cacheService) StoreHTML(content []byte) (string, error) {
	return service.manager.StoreHTML(content)
}

// StoreHTMLForPost stores HTML content and updates the post fields.
func (service *cacheService) StoreHTMLForPost(post *models.PostMeta, content []byte) error {
	return service.manager.StoreHTMLForPost(post, content)
}

// BatchCommit commits posts, search records, and dependencies in a batch.
func (service *cacheService) BatchCommit(posts []*models.PostMeta, records map[string]*models.SearchRecord, dependencies map[string]*models.Dependencies) error {
	return service.manager.BatchCommit(posts, records, dependencies)
}

// DeletePost removes a post from the cache.
func (service *cacheService) DeletePost(postID string) error {
	return service.manager.DeletePost(postID)
}

// MarkDirty marks a post as dirty.
func (service *cacheService) MarkDirty(postID string) {
	service.dirtyPostsMap.Store(postID, true)
	// We also call manager.MarkDirty if the manager still relies on it.
	service.manager.MarkDirty(postID)
}

// GetFragment retrieves a fragment from the cache.
func (service *cacheService) GetFragment(key string) (string, error) {
	return service.manager.GetFragment(key)
}

// StoreFragment stores a fragment in the cache.
func (service *cacheService) StoreFragment(key string, html string) error {
	return service.manager.StoreFragment(key, html)
}

// Flush flushes the fragment cache. No-op for direct service as it writes immediately.
func (service *cacheService) Flush(_ context.Context) error {
	return nil
}

// IsDirty reports whether a post is marked dirty.
func (service *cacheService) IsDirty(postID string) bool {
	value, loaded := service.dirtyPostsMap.Load(postID)
	if !loaded {
		return false
	}
	isDirty, loaded := value.(bool)
	return loaded && isDirty
}

// ClearDirty clears all dirty markers.
func (service *cacheService) ClearDirty() {
	// Use Range+Delete instead of reassignment to prevent lost dirty marks
	service.dirtyPostsMap.Range(func(key, _ any) bool {
		service.dirtyPostsMap.Delete(key)
		return true
	})
}

// Stats returns cache stats.
func (service *cacheService) Stats() (*cache.Stats, error) {
	return service.manager.Stats()
}

// IncrementBuildCount increments and returns the build count.
func (service *cacheService) IncrementBuildCount() (uint32, error) {
	return service.manager.IncrementBuildCount()
}

// RunGC runs garbage collection with the provided config.
func (service *cacheService) RunGC(config gc.Config) (*gc.Result, error) {
	return service.manager.RunGC(config)
}

// Close closes the cache service.
func (service *cacheService) Close() error {
	return service.manager.Close()
}

// GetAllPostsMetadata returns a lightweight list of post metadata.
func (service *cacheService) GetAllPostsMetadata() ([]models.PostListMeta, error) {
	return service.manager.GetAllPostsMetadata()
}

// Manager exposes the underlying cache manager (avoid in production code).
func (service *cacheService) Manager() *cache.Manager {
	return service.manager
}
