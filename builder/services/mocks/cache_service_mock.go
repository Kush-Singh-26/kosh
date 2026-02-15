// Package mocks provides mock implementations for testing
package mocks

import (
	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// MockCacheService is a mock implementation of services.CacheService
type MockCacheService struct {
	Posts              map[string]*cache.PostMeta
	PostsByPath        map[string]*cache.PostMeta
	HTML               map[string][]byte
	SearchRecords      map[string]*cache.SearchRecord
	Dirty              map[string]bool
	SocialCardHashes   map[string]string
	GraphHash          string
	WasmHash           string
	Err                error
	CallCount          map[string]int
	BatchCommitPosts   []*cache.PostMeta
	BatchCommitRecords map[string]*cache.SearchRecord
	BatchCommitDeps    map[string]*cache.Dependencies
}

// NewMockCacheService creates a new mock cache service
func NewMockCacheService() *MockCacheService {
	return &MockCacheService{
		Posts:              make(map[string]*cache.PostMeta),
		PostsByPath:        make(map[string]*cache.PostMeta),
		HTML:               make(map[string][]byte),
		SearchRecords:      make(map[string]*cache.SearchRecord),
		Dirty:              make(map[string]bool),
		SocialCardHashes:   make(map[string]string),
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

// GetPost returns a post by ID
func (m *MockCacheService) GetPost(id string) (*cache.PostMeta, error) {
	m.recordCall("GetPost")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Posts[id], nil
}

// ListAllPosts returns all post IDs
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

// GetPostByPath returns a post by path
func (m *MockCacheService) GetPostByPath(path string) (*cache.PostMeta, error) {
	m.recordCall("GetPostByPath")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.PostsByPath[path], nil
}

// GetPostsByIDs returns multiple posts by ID
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

// GetPostsByTemplate returns posts using a template
func (m *MockCacheService) GetPostsByTemplate(templatePath string) ([]string, error) {
	m.recordCall("GetPostsByTemplate")
	if m.Err != nil {
		return nil, m.Err
	}
	return []string{}, nil
}

// GetSearchRecords returns multiple search records
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

// GetSearchRecord returns a single search record
func (m *MockCacheService) GetSearchRecord(id string) (*cache.SearchRecord, error) {
	m.recordCall("GetSearchRecord")
	if m.Err != nil {
		return nil, m.Err
	}
	return m.SearchRecords[id], nil
}

// GetHTMLContent returns HTML content for a post
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

// GetSocialCardHash returns the hash for a social card
func (m *MockCacheService) GetSocialCardHash(path string) (string, error) {
	m.recordCall("GetSocialCardHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.SocialCardHashes[path], nil
}

// SetSocialCardHash sets the hash for a social card
func (m *MockCacheService) SetSocialCardHash(path, hash string) error {
	m.recordCall("SetSocialCardHash")
	if m.Err != nil {
		return m.Err
	}
	m.SocialCardHashes[path] = hash
	return nil
}

// GetGraphHash returns the graph hash
func (m *MockCacheService) GetGraphHash() (string, error) {
	m.recordCall("GetGraphHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.GraphHash, nil
}

// SetGraphHash sets the graph hash
func (m *MockCacheService) SetGraphHash(hash string) error {
	m.recordCall("SetGraphHash")
	if m.Err != nil {
		return m.Err
	}
	m.GraphHash = hash
	return nil
}

// GetWasmHash returns the WASM hash
func (m *MockCacheService) GetWasmHash() (string, error) {
	m.recordCall("GetWasmHash")
	if m.Err != nil {
		return "", m.Err
	}
	return m.WasmHash, nil
}

// SetWasmHash sets the WASM hash
func (m *MockCacheService) SetWasmHash(hash string) error {
	m.recordCall("SetWasmHash")
	if m.Err != nil {
		return m.Err
	}
	m.WasmHash = hash
	return nil
}

// StoreHTML stores HTML and returns its hash
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

// StoreHTMLForPost stores HTML for a specific post
func (m *MockCacheService) StoreHTMLForPost(post *cache.PostMeta, content []byte) error {
	m.recordCall("StoreHTMLForPost")
	if m.Err != nil {
		return m.Err
	}
	if len(content) < utils.InlineHTMLThreshold {
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

// BatchCommit commits multiple posts
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

// DeletePost removes a post
func (m *MockCacheService) DeletePost(postID string) error {
	m.recordCall("DeletePost")
	if m.Err != nil {
		return m.Err
	}
	delete(m.Posts, postID)
	return nil
}

// MarkDirty marks a post as dirty
func (m *MockCacheService) MarkDirty(postID string) {
	m.recordCall("MarkDirty")
	m.Dirty[postID] = true
}

// IsDirty checks if a post is dirty
func (m *MockCacheService) IsDirty(postID string) bool {
	m.recordCall("IsDirty")
	return m.Dirty[postID]
}

// Stats returns cache statistics
func (m *MockCacheService) Stats() (*cache.CacheStats, error) {
	m.recordCall("Stats")
	if m.Err != nil {
		return nil, m.Err
	}
	return &cache.CacheStats{}, nil
}

// IncrementBuildCount increments the build counter
func (m *MockCacheService) IncrementBuildCount() error {
	m.recordCall("IncrementBuildCount")
	return m.Err
}

// Close closes the cache
func (m *MockCacheService) Close() error {
	m.recordCall("Close")
	return m.Err
}
