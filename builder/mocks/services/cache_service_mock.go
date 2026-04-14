// Package mocks provides mock implementations for testing
package mocks

import (
	"context"
	"maps"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// MockCacheService is a test double for the cache service.
type MockCacheService struct {
	Posts              map[string]*cache.PostMeta
	PostsByPath        map[string]*cache.PostMeta
	HTML               map[string][]byte
	SearchRecords      map[string]*cache.SearchRecord
	Dirty              map[string]bool
	SocialCardHashes   map[string]string
	Fragments          map[string]string
	GraphHash          string
	WasmHash           string
	SearchHash         string
	Err                error
	CallCount          map[string]int
	BatchCommitPosts   []*cache.PostMeta
	BatchCommitRecords map[string]*cache.SearchRecord
	BatchCommitDeps    map[string]*cache.Dependencies
	GetPostByPathFn    func(path string) (*cache.PostMeta, error)
}

// NewMockCacheService returns a new mock cache service with initialized maps.
func NewMockCacheService() *MockCacheService {
	return &MockCacheService{
		Posts:              make(map[string]*cache.PostMeta),
		PostsByPath:        make(map[string]*cache.PostMeta),
		HTML:               make(map[string][]byte),
		SearchRecords:      make(map[string]*cache.SearchRecord),
		Dirty:              make(map[string]bool),
		SocialCardHashes:   make(map[string]string),
		Fragments:          make(map[string]string),
		CallCount:          make(map[string]int),
		BatchCommitRecords: make(map[string]*cache.SearchRecord),
		BatchCommitDeps:    make(map[string]*cache.Dependencies),
	}
}

func (m *MockCacheService) recordCall(method string) {
	if m.CallCount == nil {
		m.CallCount = make(map[string]int)
	}
	m.CallCount[method]++
}

// GetPostByID returns the cached post by ID.
func (m *MockCacheService) GetPostByID(id string) (*cache.PostMeta, error) {
	m.recordCall("GetPostByID")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Posts[id], nil
}

// ListAllPosts lists all post IDs stored in the mock.
func (m *MockCacheService) ListAllPosts() ([]string, error) {
	m.recordCall("ListAllPosts")
	if m.Err != nil {
		return nil, m.Err
	}
	ids := make([]string, 0, len(m.Posts))
	for id := range m.Posts {
		ids = append(ids, id)
	}
	return ids, nil
}

// GetPostByPath returns the cached post by its path.
func (m *MockCacheService) GetPostByPath(path string) (*cache.PostMeta, error) {
	m.recordCall("GetPostByPath")
	if m.GetPostByPathFn != nil {
		return m.GetPostByPathFn(path)
	}
	if m.Err != nil {
		return nil, m.Err
	}
	return m.PostsByPath[path], nil
}

// GetPostsByIDs returns cached posts for a set of IDs.
func (m *MockCacheService) GetPostsByIDs(ids []string) (map[string]*cache.PostMeta, error) {
	m.recordCall("GetPostsByIDs")
	if m.Err != nil {
		return nil, m.Err
	}
	result := make(map[string]*cache.PostMeta)
	for _, id := range ids {
		if post, ok := m.Posts[id]; ok {
			result[id] = post
		}
	}
	return result, nil
}

// GetPostsByTemplate returns post IDs that depend on a template.
func (m *MockCacheService) GetPostsByTemplate(templatePath string) ([]string, error) {
	m.recordCall("GetPostsByTemplate")
	if m.Err != nil {
		return nil, m.Err
	}
	return []string{}, nil
}

// GetSearchRecords returns search records for the given IDs.
func (m *MockCacheService) GetSearchRecords(ids []string) (map[string]*cache.SearchRecord, error) {
	m.recordCall("GetSearchRecords")
	if m.Err != nil {
		return nil, m.Err
	}
	result := make(map[string]*cache.SearchRecord)
	for _, id := range ids {
		if rec, ok := m.SearchRecords[id]; ok {
			result[id] = rec
		}
	}
	return result, nil
}

// GetSearchRecord returns a search record by ID.
func (m *MockCacheService) GetSearchRecord(id string) (*cache.SearchRecord, error) {
	m.recordCall("GetSearchRecord")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.SearchRecords[id], nil
}

// GetHTMLContent returns cached HTML for a post.
func (m *MockCacheService) GetHTMLContent(post *cache.PostMeta) ([]byte, error) {
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
func (m *MockCacheService) GetFragment(key string) (string, error) {
	m.recordCall("GetFragment")
	if m.Err != nil {
		return "", m.Err
	}
	return m.Fragments[key], nil
}

// StoreFragment stores a fragment in the cache.
func (m *MockCacheService) StoreFragment(key, content string) error {
	m.recordCall("StoreFragment")
	if m.Err != nil {
		return m.Err
	}
	m.Fragments[key] = content
	return nil
}

// GetPostsByTaxonomy returns post IDs for a given taxonomy and term.
func (m *MockCacheService) GetPostsByTaxonomy(taxonomy, term string) ([]string, error) {
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

// StoreHTMLForPost stores HTML content for a post and updates its fields.
func (m *MockCacheService) StoreHTMLForPost(post *cache.PostMeta, content []byte) error {
	m.recordCall("StoreHTMLForPost")
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
func (m *MockCacheService) BatchCommit(posts []*cache.PostMeta, records map[string]*cache.SearchRecord, deps map[string]*cache.Dependencies) error {
	m.recordCall("BatchCommit")
	if m.Err != nil {
		return m.Err
	}
	m.BatchCommitPosts = posts
	m.BatchCommitRecords = records
	m.BatchCommitDeps = deps
	for _, post := range posts {
		m.Posts[post.PostID] = post
		m.PostsByPath[post.Path] = post
	}
	return nil
}

// DeletePost removes a post from the mock.
func (m *MockCacheService) DeletePost(postID string) error {
	m.recordCall("DeletePost")
	if m.Err != nil {
		return m.Err
	}
	delete(m.Posts, postID)
	return nil
}

// MarkDirty marks a post as dirty in the mock.
func (m *MockCacheService) MarkDirty(postID string) {
	m.recordCall("MarkDirty")
	m.Dirty[postID] = true
}

// IsDirty reports whether a post is marked dirty.
func (m *MockCacheService) IsDirty(postID string) bool {
	m.recordCall("IsDirty")
	return m.Dirty[postID]
}

// ClearDirty clears the dirty state for all posts.
func (m *MockCacheService) ClearDirty() {
	m.recordCall("ClearDirty")
	m.Dirty = make(map[string]bool)
}

// Stats returns cache stats for the mock.
func (m *MockCacheService) Stats() (*cache.CacheStats, error) {
	m.recordCall("Stats")
	if m.Err != nil {
		return nil, m.Err
	}
	return &cache.CacheStats{}, nil
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
func (m *MockCacheService) RunGC(cfg gc.GCConfig) (*gc.GCResult, error) {
	m.recordCall("RunGC")
	if m.Err != nil {
		return nil, m.Err
	}
	return &gc.GCResult{}, nil
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

// GetAllPostsMetadata returns a lightweight list of post metadata.
func (m *MockCacheService) GetAllPostsMetadata() ([]cache.PostListMeta, error) {
	m.recordCall("GetAllPostsMetadata")
	if m.Err != nil {
		return nil, m.Err
	}
	var result []cache.PostListMeta
	for _, post := range m.Posts {
		result = append(result, cache.PostListMeta{
			Title:      post.Title,
			Link:       post.Link,
			Weight:     post.Weight,
			Date:       post.Date,
			Taxonomies: post.Taxonomies,
		})
	}
	return result, nil
}
