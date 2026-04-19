// Package mocks provides mock implementations for testing
package mocks

import (
"context"
"maps"

"github.com/Kush-Singh-26/kosh/builder/cache/core"
"github.com/Kush-Singh-26/kosh/builder/cache/gc"
"github.com/Kush-Singh-26/kosh/builder/models"
)

// MockCacheService is a test double for the cache service.
type MockCacheService struct {
	Posts map[string]*models.ContentMeta
	PostsByPath map[string]*models.ContentMeta
	HTML map[string][]byte
	SearchRecords map[string]*models.SearchRecord
	Dirty map[string]bool
	SocialCardHashes map[string]string
	Fragments map[string][]byte
	GraphHash string
	WasmHash string
	SearchHash string
	Err error
	CallCount map[string]int
	BatchCommitPosts []*models.ContentMeta
	BatchCommitRecords map[string]*models.SearchRecord
	BatchCommitDeps map[string]*models.Dependencies
	GetItemByPathFn func(path string) (*models.ContentMeta, error)
}

// NewMockCacheService returns a new mock cache service with initialized maps.
func NewMockCacheService() *MockCacheService {

	return &MockCacheService{
		Posts: make(map[string]*models.ContentMeta),
		PostsByPath: make(map[string]*models.ContentMeta),
		HTML: make(map[string][]byte),
		SearchRecords: make(map[string]*models.SearchRecord),
		Dirty: make(map[string]bool),
		SocialCardHashes: make(map[string]string),
		Fragments: make(map[string][]byte),
		CallCount: make(map[string]int),
		BatchCommitRecords: make(map[string]*models.SearchRecord),
		BatchCommitDeps: make(map[string]*models.Dependencies),
	}
}

func (m *MockCacheService) recordCall(method string) {
	if m.CallCount == nil {
		m.CallCount = make(map[string]int)
	}
	m.CallCount[method]++
}

// GetItemByID returns the cached post by ID.
func (m *MockCacheService) GetItemByID(id string) (*models.ContentMeta, error) {
	m.recordCall("GetItemByID")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Posts[id], nil
}

// ListAllItems lists all post IDs stored in the mock.
func (m *MockCacheService) ListAllItems() ([]string, error) {
	m.recordCall("ListAllItems")
	if m.Err != nil {
		return nil, m.Err
	}
	ids := make([]string, 0, len(m.Posts))
	for id := range m.Posts {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetItemByPath returns the cached post by its path.
func (m *MockCacheService) GetItemByPath(path string) (*models.ContentMeta, error) {
	m.recordCall("GetItemByPath")
	if m.GetItemByPathFn != nil {
		return m.GetItemByPathFn(path)
	}
	if m.Err != nil {
		return nil, m.Err
	}
	return m.PostsByPath[path], nil
}

// GetItemsByIDs returns cached posts for a set of IDs.
func (m *MockCacheService) GetItemsByIDs(ids []string) (map[string]*models.ContentMeta, error) {
	m.recordCall("GetItemsByIDs")
	if m.Err != nil {
		return nil, m.Err
	}
	result := make(map[string]*models.ContentMeta)
	for _, id := range ids {
		if post, ok := m.Posts[id]; ok {
			result[id] = post
		}
	}
	return result, nil
}

// GetItemsByTemplate returns post IDs that depend on a template.
func (m *MockCacheService) GetItemsByTemplate(_ string) ([]string, error) {
	m.recordCall("GetItemsByTemplate")
	if m.Err != nil {
		return nil, m.Err
	}
	return []string{}, nil
}

// GetSearchRecords returns search records for the given IDs.
func (m *MockCacheService) GetSearchRecords(ids []string) (map[string]*models.SearchRecord, error) {
	m.recordCall("GetSearchRecords")
	if m.Err != nil {
		return nil, m.Err
	}
	result := make(map[string]*models.SearchRecord)
	for _, id := range ids {
		if rec, ok := m.SearchRecords[id]; ok {
			result[id] = rec
		}
	}
	return result, nil
}

// GetSearchRecord returns a search record by ID.
func (m *MockCacheService) GetSearchRecord(id string) (*models.SearchRecord, error) {
	m.recordCall("GetSearchRecord")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.SearchRecords[id], nil
}

// GetHTMLContent returns cached HTML for a post.
func (m *MockCacheService) GetHTMLContent(post *models.ContentMeta) ([]byte, error) {
	m.recordCall("GetHTMLContent")
	if m.Err != nil {
		return nil, m.Err
	}
	if post.InlineHTML != nil {
		return post.InlineHTML, nil
	}
	if post.HTMLHash != "" {
		return m.HTML[post.HTMLHash], nil
	}
	return nil, nil
}

// GetSocialCardHash returns the cached social card hash for a path.
func (m *MockCacheService) GetSocialCardHash(path string) (string, error) {
	m.recordCall("GetSocialCardHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.SocialCardHashes[path], nil
}

// SetSocialCardHash stores the social card hash for a path.
func (m *MockCacheService) SetSocialCardHash(path, hash string) error {
	m.recordCall("SetSocialCardHash")
	if m.Err != nil {
		return m.Err
	}
	m.SocialCardHashes[path] = hash
	return nil
}

// BatchSetSocialCardHashes stores multiple social card hashes.
func (m *MockCacheService) BatchSetSocialCardHashes(hashes map[string]string) error {
	m.recordCall("BatchSetSocialCardHashes")
	if m.Err != nil {
		return m.Err
	}
	maps.Copy(m.SocialCardHashes, hashes)
	return nil
}

// GetGraphHash returns the cached graph hash.
func (m *MockCacheService) GetGraphHash() (string, error) {
	m.recordCall("GetGraphHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.GraphHash, nil
}

// SetGraphHash stores the cached graph hash.
func (m *MockCacheService) SetGraphHash(hash string) error {
	m.recordCall("SetGraphHash")
	if m.Err != nil {
		return m.Err
	}
	m.GraphHash = hash
	return nil
}

// GetWasmHash returns the cached wasm hash.
func (m *MockCacheService) GetWasmHash() (string, error) {
	m.recordCall("GetWasmHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.WasmHash, nil
}

// SetWasmHash stores the cached wasm hash.
func (m *MockCacheService) SetWasmHash(hash string) error {
	m.recordCall("SetWasmHash")
	if m.Err != nil {
		return m.Err
	}
	m.WasmHash = hash
	return nil
}

// GetSearchHash returns the cached search hash.
func (m *MockCacheService) GetSearchHash() (string, error) {
	m.recordCall("GetSearchHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.SearchHash, nil
}

// SetSearchHash stores the cached search hash.
func (m *MockCacheService) SetSearchHash(hash string) error {
	m.recordCall("SetSearchHash")
	if m.Err != nil {
		return m.Err
	}
	m.SearchHash = hash
	return nil
}

// GetFragment retrieves a cached fragment.
func (m *MockCacheService) GetFragment(key string) ([]byte, error) {
	m.recordCall("GetFragment")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Fragments[key], nil
}

// StoreFragment stores a fragment in the cache.
func (m *MockCacheService) StoreFragment(key string, data []byte) error {
	m.recordCall("StoreFragment")
	if m.Err != nil {
		return m.Err
	}
	m.Fragments[key] = data
	return nil
}

// GetPostsByTaxonomy returns post IDs for a given taxonomy and term.
func (m *MockCacheService) GetPostsByTaxonomy(_, _ string) ([]string, error) {
	m.recordCall("GetPostsByTaxonomy")
	if m.Err != nil {
		return nil, m.Err
	}
	return []string{}, nil
}

// StoreHTML stores HTML content and returns its hash.
func (m *MockCacheService) StoreHTML(content []byte) (string, error) {
	m.recordCall("StoreHTML")
	if m.Err != nil {
		return "", m.Err
	}
	// Simple hash for testing
	hash := string(content)
	m.HTML[hash] = content
	return hash, nil
}

// StoreHTMLForItem stores HTML content for a post and updates its fields.
func (m *MockCacheService) StoreHTMLForItem(post *models.ContentMeta, content []byte) error {
	m.recordCall("StoreHTMLForItem")
	if m.Err != nil {
		return m.Err
	}
	if len(content) < models.InlineHTMLThreshold {
		post.InlineHTML = content
		post.HTMLHash = ""
	} else {
		hash := string(content)
		m.HTML[hash] = content
		post.HTMLHash = hash
		post.InlineHTML = nil
	}
	return nil
}

// BatchCommit records a batch commit in the mock.
func (m *MockCacheService) BatchCommit(posts []*models.ContentMeta, records map[string]*models.SearchRecord, deps map[string]*models.Dependencies) error {
	m.recordCall("BatchCommit")
	if m.Err != nil {
		return m.Err
	}
	m.BatchCommitPosts = posts
	m.BatchCommitRecords = records
	m.BatchCommitDeps = deps
	for _, post := range posts {
		m.Posts[post.ContentID] = post
		m.PostsByPath[post.Path] = post
	}
	return nil
}

// DeleteItem removes a post from the mock.
func (m *MockCacheService) DeleteItem(contentID string) error {
	m.recordCall("DeleteItem")
	if m.Err != nil {
		return m.Err
	}
	delete(m.Posts, contentID)
	return nil
}

// MarkDirty marks a post as dirty in the mock.
func (m *MockCacheService) MarkDirty(contentID string) {
	m.recordCall("MarkDirty")
	m.Dirty[contentID] = true
}

// IsDirty reports whether a post is marked dirty.
func (m *MockCacheService) IsDirty(contentID string) bool {
	m.recordCall("IsDirty")
	return m.Dirty[contentID]
}

// ClearDirty clears the dirty state for all posts.
func (m *MockCacheService) ClearDirty() {
	m.recordCall("ClearDirty")
	m.Dirty = make(map[string]bool)
}

// Stats returns cache stats for the mock.
func (m *MockCacheService) Stats() (*core.CacheStats, error) {
	m.recordCall("Stats")
	if m.Err != nil {
		return nil, m.Err
	}
	return &core.CacheStats{}, nil
}

// IncrementBuildCount increments and returns the build count.
func (m *MockCacheService) IncrementBuildCount() (uint32, error) {
	m.recordCall("IncrementBuildCount")
	if m.Err != nil {
		return 0, m.Err
	}
	return 1, nil
}

// RunGC runs garbage collection using the provided config.
func (m *MockCacheService) RunGC(_ gc.Config) (*gc.Result, error) {
	m.recordCall("RunGC")
	if m.Err != nil {
		return nil, m.Err
	}
	return &gc.Result{}, nil
}

// Close closes the mock cache service.
func (m *MockCacheService) Close() error {
	m.recordCall("Close")
	return m.Err
}

// Flush flushes the fragment cache.
func (m *MockCacheService) Flush(_ context.Context) error {
	m.recordCall("Flush")
	return nil
}

// GetAllItemsMetadata returns a lightweight list of post metadata.
func (m *MockCacheService) GetAllItemsMetadata() ([]models.ContentListMeta, error) {
	m.recordCall("GetAllItemsMetadata")
	if m.Err != nil {
		return nil, m.Err
	}
	var result []models.ContentListMeta
	for _, post := range m.Posts {
		result = append(result, models.ContentListMeta{
			Title:      post.Title,
			Link:       post.Link,
			Weight:     post.Weight,
			Date:       post.Date,
			Taxonomies: post.Taxonomies,
		})
	}
	return result, nil
}
