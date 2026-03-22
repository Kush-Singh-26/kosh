package search

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
)

type Manager struct {
	cfg    *config.Config
	cache  svcCache.Service
	logger *slog.Logger
	health HealthRegistry

	sink   fspkg.ArtifactSink
	render render.Service

	mu           sync.RWMutex
	indexedPosts []models.IndexedPost
}

type HealthRegistry interface {
	RecordSearchStats(docs int64, size int64)
}

type ManagerDependencies struct {
	Cfg    *config.Config
	Cache  svcCache.Service
	Logger *slog.Logger
	Health HealthRegistry
}

func NewManager(deps ManagerDependencies) *Manager {
	return &Manager{
		cfg:    deps.Cfg,
		cache:  deps.Cache,
		logger: deps.Logger,
		health: deps.Health,
	}
}

func (m *Manager) Reconfigure(sink fspkg.ArtifactSink, render render.Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sink = sink
	m.render = render
}

func (m *Manager) SetIndexedPosts(posts []models.IndexedPost) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.indexedPosts = posts
}

func (m *Manager) GetIndexedPosts() []models.IndexedPost {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.indexedPosts
}

func (m *Manager) UpdateIndexedPostCache(relPath string, parseRes *post.ParsedMarkdownResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.indexedPosts) == 0 {
		return
	}

	found := false
	targetKey := fspkg.NormalizePath(relPath)
	for i, ip := range m.indexedPosts {
		if indexedPostStableKey(ip) == targetKey {
			m.indexedPosts[i] = models.IndexedPost{
				Record:          parseRes.SearchRecord,
				SourcePath:      targetKey,
				WordFreqs:       parseRes.WordFreqs,
				DocLen:          parseRes.DocLen,
				StemMap:         parseRes.StemMap,
				PositionalIndex: parseRes.PositionalIndex,
				ByteOffsets:     parseRes.ByteOffsets,
			}
			found = true
			break
		}
	}

	if !found {
		m.indexedPosts = append(m.indexedPosts, models.IndexedPost{
			Record:          parseRes.SearchRecord,
			SourcePath:      targetKey,
			WordFreqs:       parseRes.WordFreqs,
			DocLen:          parseRes.DocLen,
			StemMap:         parseRes.StemMap,
			PositionalIndex: parseRes.PositionalIndex,
			ByteOffsets:     parseRes.ByteOffsets,
		})
	}
}

func (m *Manager) PruneDeletedPost(relPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	targetKey := fspkg.NormalizePath(relPath)
	newIndexed := make([]models.IndexedPost, 0, len(m.indexedPosts))
	for _, ip := range m.indexedPosts {
		if indexedPostStableKey(ip) != targetKey {
			newIndexed = append(newIndexed, ip)
		}
	}
	m.indexedPosts = newIndexed
}

func (m *Manager) RegenerateIndex(ctx context.Context) error {
	m.mu.Lock()
	sink := m.sink
	render := m.render
	m.mu.Unlock()

	if sink == nil || render == nil {
		return nil
	}

	indexedPosts, err := m.ensureIndexedPosts()
	if err != nil {
		return err
	}

	if len(indexedPosts) == 0 {
		return nil
	}

	indexedPosts = dedupeIndexedPosts(indexedPosts)
	m.mu.Lock()
	m.indexedPosts = indexedPosts
	m.mu.Unlock()

	path, size, err := generators.GenerateSearchIndex(sink, indexedPosts)
	if err != nil {
		return err
	}
	render.RegisterFile(path)

	if m.health != nil {
		m.health.RecordSearchStats(int64(len(indexedPosts)), size)
	}

	return nil
}

func (m *Manager) ensureIndexedPosts() ([]models.IndexedPost, error) {
	m.mu.RLock()
	if len(m.indexedPosts) > 0 {
		posts := m.indexedPosts
		m.mu.RUnlock()
		return posts, nil
	}
	m.mu.RUnlock()

	if m.cache == nil {
		return nil, nil
	}

	postIDs, err := m.cache.ListAllPosts()
	if err != nil {
		return nil, err
	}
	if len(postIDs) == 0 {
		return nil, nil
	}

	posts, err := m.cache.GetPostsByIDs(postIDs)
	if err != nil {
		return nil, err
	}
	searchRecords, err := m.cache.GetSearchRecords(postIDs)
	if err != nil {
		return nil, err
	}

	sort.Strings(postIDs)
	indexedPosts := make([]models.IndexedPost, 0, len(posts))
	for _, postID := range postIDs {
		postMeta, ok := posts[postID]
		if !ok || postMeta == nil {
			continue
		}
		searchRec, ok := searchRecords[postID]
		if !ok || searchRec == nil {
			continue
		}
		htmlRelPath := strings.ToLower(strings.Replace(postMeta.Path, ".md", ".html", 1))
		indexedPosts = append(indexedPosts, models.IndexedPost{
			Record: models.PostRecord{
				ID:              xxh3.HashString(htmlRelPath),
				Title:           postMeta.Title,
				NormalizedTitle: searchRec.NormalizedTitle,
				Link:            htmlRelPath,
				Description:     postMeta.Description,
				Tags:            postMeta.Tags,
				NormalizedTags:  searchRec.NormalizedTags,
				Version:         postMeta.Version,
			},
			SourcePath:      postMeta.Path,
			WordFreqs:       searchRec.BM25Data,
			DocLen:          searchRec.DocLen,
			StemMap:         searchRec.StemMap,
			PositionalIndex: searchRec.PositionalIndex,
			ByteOffsets:     searchRec.ByteOffsets,
		})
	}
	return indexedPosts, nil
}

func indexedPostStableKey(ip models.IndexedPost) string {
	if ip.SourcePath != "" {
		return fspkg.NormalizePath(ip.SourcePath)
	}
	return fspkg.NormalizePath(ip.Record.Link)
}

func dedupeIndexedPosts(posts []models.IndexedPost) []models.IndexedPost {
	if len(posts) < 2 {
		return posts
	}
	seen := make(map[string]int, len(posts))
	result := make([]models.IndexedPost, 0, len(posts))
	for _, ip := range posts {
		key := indexedPostStableKey(ip)
		if idx, ok := seen[key]; ok {
			result[idx] = ip
			continue
		}
		seen[key] = len(result)
		result = append(result, ip)
	}
	return result
}
