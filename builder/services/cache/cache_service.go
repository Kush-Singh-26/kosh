package services

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
	dirty sync.Map
}

// NewCacheService creates a new CacheService with the given dependencies.
// Using dependency struct pattern for API coherence.
func NewCacheService(deps CacheServiceDependencies) CacheService {
	return &cacheService{
		ctx:     deps.Ctx,
		manager: deps.Manager,
		logger:  deps.Logger,
	}
}

// NewCacheServiceWith creates a new CacheService with explicit parameters.
// Deprecated: use NewCacheService(CacheServiceDependencies{...}) instead.
func NewCacheServiceWith(manager *cache.Manager, logger *slog.Logger) CacheService {
	return &cacheService{
		manager: manager,
		logger:  logger,
	}
}

func (s *cacheService) GetPost(id string) (*models.PostMeta, error) {
	return s.manager.GetPostByID(id)
}

func (s *cacheService) ListAllPosts() ([]string, error) {
	return s.manager.ListAllPosts()
}

func (s *cacheService) GetPostByPath(path string) (*models.PostMeta, error) {
	return s.manager.GetPostByPath(path)
}

func (s *cacheService) GetPostsByIDs(ids []string) (map[string]*models.PostMeta, error) {
	return s.manager.GetPostsByIDs(ids)
}

func (s *cacheService) GetPostsByTemplate(templatePath string) ([]string, error) {
	return s.manager.GetPostsByTemplate(templatePath)
}

func (s *cacheService) GetSearchRecords(ids []string) (map[string]*models.SearchRecord, error) {
	return s.manager.GetSearchRecords(ids)
}

func (s *cacheService) GetSearchRecord(id string) (*models.SearchRecord, error) {
	return s.manager.GetSearchRecord(id)
}

func (s *cacheService) GetHTMLContent(post *models.PostMeta) ([]byte, error) {
	return s.manager.GetHTMLContent(post)
}

func (s *cacheService) GetSocialCardHash(path string) (string, error) {
	return s.manager.GetSocialCardHash(path)
}

func (s *cacheService) SetSocialCardHash(path, hash string) error {
	return s.manager.SetSocialCardHash(path, hash)
}

func (s *cacheService) BatchSetSocialCardHashes(hashes map[string]string) error {
	return s.manager.BatchSetSocialCardHashes(hashes)
}

func (s *cacheService) GetGraphHash() (string, error) {
	return s.manager.GetGraphHash()
}

func (s *cacheService) SetGraphHash(hash string) error {
	return s.manager.SetGraphHash(hash)
}

func (s *cacheService) GetWasmHash() (string, error) {
	return s.manager.GetWasmHash()
}

func (s *cacheService) SetWasmHash(hash string) error {
	return s.manager.SetWasmHash(hash)
}

func (s *cacheService) StoreHTML(content []byte) (string, error) {
	return s.manager.StoreHTML(content)
}

func (s *cacheService) StoreHTMLForPost(post *models.PostMeta, content []byte) error {
	return s.manager.StoreHTMLForPost(post, content)
}

func (s *cacheService) BatchCommit(posts []*models.PostMeta, records map[string]*models.SearchRecord, deps map[string]*models.Dependencies) error {
	return s.manager.BatchCommit(posts, records, deps)
}

func (s *cacheService) DeletePost(postID string) error {
	return s.manager.DeletePost(postID)
}

func (s *cacheService) MarkDirty(postID string) {
	s.dirty.Store(postID, true)
	// We also call manager.MarkDirty if the manager still relies on it.
	s.manager.MarkDirty(postID)
}

func (s *cacheService) IsDirty(postID string) bool {
	val, ok := s.dirty.Load(postID)
	if !ok {
		return false
	}
	dirty, ok := val.(bool)
	return ok && dirty
}

func (s *cacheService) ClearDirty() {
	// Use Range+Delete instead of reassignment to prevent lost dirty marks
	s.dirty.Range(func(key, value any) bool {
		s.dirty.Delete(key)
		return true
	})
}

func (s *cacheService) Stats() (*cache.CacheStats, error) {
	return s.manager.Stats()
}

func (s *cacheService) IncrementBuildCount() (uint32, error) {
	return s.manager.IncrementBuildCount()
}

func (s *cacheService) RunGC(cfg gc.GCConfig) (*gc.GCResult, error) {
	return s.manager.RunGC(cfg)
}

func (s *cacheService) Close() error {
	return s.manager.Close()
}

func (s *cacheService) GetPostsMetadataByVersion(version string) ([]models.PostListMeta, error) {
	return s.manager.GetPostsMetadataByVersion(version)
}

// Additional helper to expose the underlying manager if absolutely necessary (try to avoid)
func (s *cacheService) Manager() *cache.Manager {
	return s.manager
}
