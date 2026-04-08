package cache

import (
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// cacheService implements CacheService
type cacheService struct {
	ctx     *buildCtx.BuildContext
	manager *cache.Manager
	logger  *slog.Logger

	// Dirty tracking using sync.Map for thread safety
	dirty sync.Map // postID -> bool
}

// NewService creates a new CacheService with the given dependencies.
// Using dependency struct pattern for API coherence.
func NewService(deps Dependencies) Service {
	return &cacheService{
		ctx:     deps.Ctx,
		manager: deps.Manager,
		logger:  deps.Logger,
	}
}

// GetPost returns a post by ID from the cache.
func (s *cacheService) GetPost(id string) (*models.PostMeta, error) {
	return s.manager.GetPostByID(id)
}

// ListAllPosts returns all post IDs in the cache.
func (s *cacheService) ListAllPosts() ([]string, error) {
	return s.manager.ListAllPosts()
}

// GetPostByPath returns a post by its source path.
func (s *cacheService) GetPostByPath(path string) (*models.PostMeta, error) {
	return s.manager.GetPostByPath(path)
}

// GetPostsByIDs returns posts for the provided IDs.
func (s *cacheService) GetPostsByIDs(ids []string) (map[string]*models.PostMeta, error) {
	return s.manager.GetPostsByIDs(ids)
}

// GetPostsByTemplate returns post IDs that depend on a template.
func (s *cacheService) GetPostsByTemplate(templatePath string) ([]string, error) {
	return s.manager.GetPostsByTemplate(templatePath)
}

// GetSearchRecords returns search records for the provided IDs.
func (s *cacheService) GetSearchRecords(ids []string) (map[string]*models.SearchRecord, error) {
	return s.manager.GetSearchRecords(ids)
}

// GetSearchRecord returns a search record by ID.
func (s *cacheService) GetSearchRecord(id string) (*models.SearchRecord, error) {
	return s.manager.GetSearchRecord(id)
}

// GetHTMLContent returns cached HTML for a post.
func (s *cacheService) GetHTMLContent(post *models.PostMeta) ([]byte, error) {
	return s.manager.GetHTMLContent(post)
}

// GetSocialCardHash returns the cached social card hash for a path.
func (s *cacheService) GetSocialCardHash(path string) (string, error) {
	return s.manager.GetSocialCardHash(path)
}

// SetSocialCardHash stores the social card hash for a path.
func (s *cacheService) SetSocialCardHash(path, hash string) error {
	return s.manager.SetSocialCardHash(path, hash)
}

// BatchSetSocialCardHashes stores multiple social card hashes.
func (s *cacheService) BatchSetSocialCardHashes(hashes map[string]string) error {
	return s.manager.BatchSetSocialCardHashes(hashes)
}

// GetGraphHash returns the cached graph hash.
func (s *cacheService) GetGraphHash() (string, error) {
	return s.manager.GetGraphHash()
}

// SetGraphHash stores the cached graph hash.
func (s *cacheService) SetGraphHash(hash string) error {
	return s.manager.SetGraphHash(hash)
}

// GetWasmHash returns the cached WASM hash.
func (s *cacheService) GetWasmHash() (string, error) {
	return s.manager.GetWasmHash()
}

// SetWasmHash stores the cached WASM hash.
func (s *cacheService) SetWasmHash(hash string) error {
	return s.manager.SetWasmHash(hash)
}

// StoreHTML stores HTML content and returns its hash.
func (s *cacheService) StoreHTML(content []byte) (string, error) {
	return s.manager.StoreHTML(content)
}

// StoreHTMLForPost stores HTML content and updates the post fields.
func (s *cacheService) StoreHTMLForPost(post *models.PostMeta, content []byte) error {
	return s.manager.StoreHTMLForPost(post, content)
}

// BatchCommit commits posts, search records, and dependencies in a batch.
func (s *cacheService) BatchCommit(posts []*models.PostMeta, records map[string]*models.SearchRecord, deps map[string]*models.Dependencies) error {
	return s.manager.BatchCommit(posts, records, deps)
}

// DeletePost removes a post from the cache.
func (s *cacheService) DeletePost(postID string) error {
	return s.manager.DeletePost(postID)
}

// MarkDirty marks a post as dirty.
func (s *cacheService) MarkDirty(postID string) {
	s.dirty.Store(postID, true)
	// We also call manager.MarkDirty if the manager still relies on it.
	s.manager.MarkDirty(postID)
}

// IsDirty reports whether a post is marked dirty.
func (s *cacheService) IsDirty(postID string) bool {
	val, ok := s.dirty.Load(postID)
	if !ok {
		return false
	}
	dirty, ok := val.(bool)
	return ok && dirty
}

// ClearDirty clears all dirty markers.
func (s *cacheService) ClearDirty() {
	// Use Range+Delete instead of reassignment to prevent lost dirty marks
	s.dirty.Range(func(key, value any) bool {
		s.dirty.Delete(key)
		return true
	})
}

// Stats returns cache stats.
func (s *cacheService) Stats() (*cache.CacheStats, error) {
	return s.manager.Stats()
}

// IncrementBuildCount increments and returns the build count.
func (s *cacheService) IncrementBuildCount() (uint32, error) {
	return s.manager.IncrementBuildCount()
}

// RunGC runs garbage collection with the provided config.
func (s *cacheService) RunGC(cfg gc.GCConfig) (*gc.GCResult, error) {
	return s.manager.RunGC(cfg)
}

// Close closes the cache service.
func (s *cacheService) Close() error {
	return s.manager.Close()
}

// GetAllPostsMetadata returns a lightweight list of post metadata.
func (s *cacheService) GetAllPostsMetadata() ([]models.PostListMeta, error) {
	return s.manager.GetAllPostsMetadata()
}

// Manager exposes the underlying cache manager (avoid in production code).
func (s *cacheService) Manager() *cache.Manager {
	return s.manager
}
