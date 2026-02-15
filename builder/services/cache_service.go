package services

import (
	"log/slog"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/cache"
)

// cacheServiceImpl implements CacheService
type cacheServiceImpl struct {
	manager *cache.Manager
	logger  *slog.Logger

	// Dirty tracking using sync.Map for thread safety
	dirty sync.Map
}

func NewCacheService(manager *cache.Manager, logger *slog.Logger) CacheService {
	return &cacheServiceImpl{
		manager: manager,
		logger:  logger,
	}
}

func (s *cacheServiceImpl) GetPost(id string) (*cache.PostMeta, error) {
	return s.manager.GetPostByID(id)
}

func (s *cacheServiceImpl) ListAllPosts() ([]string, error) {
	return s.manager.ListAllPosts()
}

func (s *cacheServiceImpl) GetPostByPath(path string) (*cache.PostMeta, error) {
	return s.manager.GetPostByPath(path)
}

func (s *cacheServiceImpl) GetPostsByIDs(ids []string) (map[string]*cache.PostMeta, error) {
	return s.manager.GetPostsByIDs(ids)
}

func (s *cacheServiceImpl) GetPostsByTemplate(templatePath string) ([]string, error) {
	return s.manager.GetPostsByTemplate(templatePath)
}

func (s *cacheServiceImpl) GetSearchRecords(ids []string) (map[string]*cache.SearchRecord, error) {
	return s.manager.GetSearchRecords(ids)
}

func (s *cacheServiceImpl) GetSearchRecord(id string) (*cache.SearchRecord, error) {
	return s.manager.GetSearchRecord(id)
}

func (s *cacheServiceImpl) GetHTMLContent(post *cache.PostMeta) ([]byte, error) {
	return s.manager.GetHTMLContent(post)
}

func (s *cacheServiceImpl) GetSocialCardHash(path string) (string, error) {
	return s.manager.GetSocialCardHash(path)
}

func (s *cacheServiceImpl) SetSocialCardHash(path, hash string) error {
	return s.manager.SetSocialCardHash(path, hash)
}

func (s *cacheServiceImpl) GetGraphHash() (string, error) {
	return s.manager.GetGraphHash()
}

func (s *cacheServiceImpl) SetGraphHash(hash string) error {
	return s.manager.SetGraphHash(hash)
}

func (s *cacheServiceImpl) GetWasmHash() (string, error) {
	return s.manager.GetWasmHash()
}

func (s *cacheServiceImpl) SetWasmHash(hash string) error {
	return s.manager.SetWasmHash(hash)
}

func (s *cacheServiceImpl) StoreHTML(content []byte) (string, error) {
	return s.manager.StoreHTML(content)
}

func (s *cacheServiceImpl) StoreHTMLForPostDirect(content []byte) (string, error) {
	return s.manager.StoreHTML(content)
}

func (s *cacheServiceImpl) StoreHTMLForPost(post *cache.PostMeta, content []byte) error {
	return s.manager.StoreHTMLForPost(post, content)
}

func (s *cacheServiceImpl) BatchCommit(posts []*cache.PostMeta, records map[string]*cache.SearchRecord, deps map[string]*cache.Dependencies) error {
	return s.manager.BatchCommit(posts, records, deps)
}

func (s *cacheServiceImpl) DeletePost(postID string) error {
	return s.manager.DeletePost(postID)
}

func (s *cacheServiceImpl) MarkDirty(postID string) {
	s.dirty.Store(postID, true)
	// We also call manager.MarkDirty if the manager still relies on it.
	s.manager.MarkDirty(postID)
}

func (s *cacheServiceImpl) IsDirty(postID string) bool {
	val, ok := s.dirty.Load(postID)
	if !ok {
		return false
	}
	dirty, ok := val.(bool)
	return ok && dirty
}

func (s *cacheServiceImpl) ClearDirty() {
	// Fresh map allocation is faster than Range+Delete for bulk clear
	s.dirty = sync.Map{}
}

func (s *cacheServiceImpl) Stats() (*cache.CacheStats, error) {
	return s.manager.Stats()
}

func (s *cacheServiceImpl) IncrementBuildCount() error {
	return s.manager.IncrementBuildCount()
}

func (s *cacheServiceImpl) Close() error {
	return s.manager.Close()
}

// Additional helper to expose the underlying manager if absolutely necessary (try to avoid)
func (s *cacheServiceImpl) Manager() *cache.Manager {
	return s.manager
}
