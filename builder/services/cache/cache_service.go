package cache

import (
"context"
"log/slog"
"sync"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	cachepkg "github.com/Kush-Singh-26/kosh/builder/cache"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"

"github.com/Kush-Singh-26/kosh/builder/models"
)

// cacheService implements CacheService
type cacheService struct {
	ctx *buildctx.BuildContext
	manager *cachepkg.Manager
	logger *slog.Logger

	// Dirty tracking using sync.Map for thread safety
	dirtyItemsMap sync.Map // contentID -> bool
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

// GetItemByID returns a post by ID from the cache.
func (service *cacheService) GetItemByID(contentID string) (*models.ContentMeta, error) {
	return service.manager.GetItemByID(contentID)
}

// ListAllItems returns all post IDs in the cache.
func (service *cacheService) ListAllItems() ([]string, error) {
	return service.manager.ListAllItems()
}

// GetItemByPath returns a post by its source path.
func (service *cacheService) GetItemByPath(path string) (*models.ContentMeta, error) {
	return service.manager.GetItemByPath(path)
}

// GetItemsByIDs returns posts for the provided IDs.
func (service *cacheService) GetItemsByIDs(itemIDs []string) (map[string]*models.ContentMeta, error) {
	return service.manager.GetItemsByIDs(itemIDs)
}

// GetItemsByTemplate returns post IDs that depend on a template.
func (service *cacheService) GetItemsByTemplate(templatePath string) ([]string, error) {
	return service.manager.GetItemsByTemplate(templatePath)
}

// GetSearchRecords returns search records for the provided IDs.
func (service *cacheService) GetSearchRecords(itemIDs []string) (map[string]*models.SearchRecord, error) {
	return service.manager.GetSearchRecords(itemIDs)
}

// GetSearchRecord returns a search record by ID.
func (service *cacheService) GetSearchRecord(contentID string) (*models.SearchRecord, error) {
	return service.manager.GetSearchRecord(contentID)
}

// GetHTMLContent returns cached HTML for a post.
func (service *cacheService) GetHTMLContent(item *models.ContentMeta) ([]byte, error) {
	return service.manager.GetHTMLContent(item)
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

// StoreHTMLForItem stores HTML content and updates the post fields.
func (service *cacheService) StoreHTMLForItem(item *models.ContentMeta, content []byte) error {
	return service.manager.StoreHTMLForItem(item, content)
}

// BatchCommit commits posts, search records, and dependencies in a batch.
func (service *cacheService) BatchCommit(items []*models.ContentMeta, records map[string]*models.SearchRecord, dependencies map[string]*models.Dependencies) error {
	return service.manager.BatchCommit(items, records, dependencies)
}

// DeleteItem removes a post from the cache.
func (service *cacheService) DeleteItem(contentID string) error {
	return service.manager.DeleteItem(contentID)
}

// MarkDirty marks a post as dirty.
func (service *cacheService) MarkDirty(contentID string) {
	service.dirtyItemsMap.Store(contentID, true)
	// We also call manager.MarkDirty if the manager still relies on it.
	service.manager.MarkDirty(contentID)
}

// GetFragment retrieves a fragment from the cache.
func (service *cacheService) GetFragment(key string) ([]byte, error) {
	return service.manager.GetFragment(key)
}

// StoreFragment stores a fragment in the cache.
func (service *cacheService) StoreFragment(key string, data []byte) error {
	return service.manager.StoreFragment(key, data)
}

// Flush flushes the fragment cache. No-op for direct service as it writes immediately.
func (service *cacheService) Flush(_ context.Context) error {
	return nil
}

// IsDirty reports whether a post is marked dirty.
func (service *cacheService) IsDirty(contentID string) bool {
	value, loaded := service.dirtyItemsMap.Load(contentID)
	if !loaded {
		return false
	}
	isDirty, loaded := value.(bool)
	return loaded && isDirty
}

// ClearDirty clears all dirty markers.
func (service *cacheService) ClearDirty() {
	// Use Range+Delete instead of reassignment to prevent lost dirty marks
	service.dirtyItemsMap.Range(func(key, _ any) bool {
		service.dirtyItemsMap.Delete(key)
		return true
	})
}

// Stats returns cache stats.
func (service *cacheService) Stats() (*core.CacheStats, error) {
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

// GetAllItemsMetadata returns a lightweight list of post metadata.
func (service *cacheService) GetAllItemsMetadata() ([]models.ContentListMeta, error) {
	return service.manager.GetAllItemsMetadata()
}

// Manager exposes the underlying cache manager (avoid in production code).
func (service *cacheService) Manager() *cachepkg.Manager {
	return service.manager
}
