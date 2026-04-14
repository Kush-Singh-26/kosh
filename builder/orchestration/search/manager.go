package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
	"runtime"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
)

// Manager coordinates search index regeneration and cache updates.
type Manager struct {
	cfg    *config.Config
	cache  svcCache.Service
	logger *slog.Logger
	health HealthRegistry

	sink   fspkg.ArtifactSink
	render render.Service

	mu           sync.RWMutex // protects sink, render, logger, indexedPosts
	indexedPosts []models.IndexedPost
}

// HealthRegistry records search-related health metrics.
type HealthRegistry interface {
	RecordSearchStats(docs int64, size int64)
}

// ManagerDependencies holds dependencies for the search Manager.
type ManagerDependencies struct {
	Cfg    *config.Config
	Cache  svcCache.Service
	Logger *slog.Logger
	Health HealthRegistry
}

// NewManager constructs a new search Manager.
func NewManager(deps ManagerDependencies) *Manager {
	return &Manager{
		cfg:    deps.Cfg,
		cache:  deps.Cache,
		logger: deps.Logger,
		health: deps.Health,
	}
}

// Reconfigure updates build-time sink and render dependencies.
func (managerInstance *Manager) Reconfigure(sink fspkg.ArtifactSink, renderSvc render.Service) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()
	managerInstance.sink = sink
	managerInstance.render = renderSvc
}

// ReconfigureWithLogger updates the logger for the manager.
func (managerInstance *Manager) ReconfigureWithLogger(logger *slog.Logger) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()
	managerInstance.logger = logger
}

// SetIndexedPosts replaces the in-memory indexed posts cache.
func (managerInstance *Manager) SetIndexedPosts(posts []models.IndexedPost) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()
	managerInstance.indexedPosts = posts
}

// GetIndexedPosts returns the current indexed posts cache.
func (managerInstance *Manager) GetIndexedPosts() []models.IndexedPost {
	managerInstance.mu.RLock()
	defer managerInstance.mu.RUnlock()
	return managerInstance.indexedPosts
}

// UpdateIndexedPostCache updates or inserts a single indexed post entry.
func (managerInstance *Manager) UpdateIndexedPostCache(relativePath string, parseResult *post.ParsedMarkdownResult) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()

	if len(managerInstance.indexedPosts) == 0 {
		return
	}

	found := false
	targetKey := fspkg.NormalizePath(relativePath)
	for index, indexedPost := range managerInstance.indexedPosts {
		if indexedPostStableKey(indexedPost) == targetKey {
			managerInstance.indexedPosts[index] = models.IndexedPost{
				Record:          parseResult.SearchRecord,
				SourcePath:      targetKey,
				WordFreqs:       parseResult.WordFreqs,
				DocLen:          parseResult.DocLen,
				StemMap:         parseResult.StemMap,
				PositionalIndex: parseResult.PositionalIndex,
				ByteOffsets:     parseResult.ByteOffsets,
			}
			found = true
			break
		}
	}

	if !found {
		managerInstance.indexedPosts = append(managerInstance.indexedPosts, models.IndexedPost{
			Record:          parseResult.SearchRecord,
			SourcePath:      targetKey,
			WordFreqs:       parseResult.WordFreqs,
			DocLen:          parseResult.DocLen,
			StemMap:         parseResult.StemMap,
			PositionalIndex: parseResult.PositionalIndex,
			ByteOffsets:     parseResult.ByteOffsets,
		})
	}
}

// PruneDeletedPost removes a post from the indexed cache.
func (managerInstance *Manager) PruneDeletedPost(relativePath string) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()

	targetKey := fspkg.NormalizePath(relativePath)
	newIndexed := make([]models.IndexedPost, 0, len(managerInstance.indexedPosts))
	for _, indexedPost := range managerInstance.indexedPosts {
		if indexedPostStableKey(indexedPost) != targetKey {
			newIndexed = append(newIndexed, indexedPost)
		}
	}
	managerInstance.indexedPosts = newIndexed
}

// RegenerateIndex rebuilds and writes the search index.
func (managerInstance *Manager) RegenerateIndex(workingContext context.Context) error {
	managerInstance.mu.Lock()
	sink := managerInstance.sink
	renderSvc := managerInstance.render
	managerInstance.mu.Unlock()

	if sink == nil || renderSvc == nil {
		return nil
	}

	indexedPosts, searchError := managerInstance.ensureIndexedPosts()
	if searchError != nil {
		return searchError
	}

	if len(indexedPosts) == 0 {
		return nil
	}

	indexedPosts = dedupeIndexedPosts(indexedPosts)

	// Optimization: Skip generation if search searchable content remains unchanged.
	newHash := managerInstance.calculateSearchHash(indexedPosts)
	if !managerInstance.cfg.ShouldForceRebuild {
		if cachedHash, err := managerInstance.cache.GetSearchHash(); err == nil && cachedHash == newHash {
			managerInstance.logger.Debug("Search index unchanged, skipping generation", "hash", newHash)
			return nil
		}
	}

	managerInstance.mu.Lock()
	managerInstance.indexedPosts = indexedPosts
	managerInstance.mu.Unlock()

	indexPath, indexSize, generateError := generators.GenerateSearchIndex(sink, indexedPosts)
	if generateError != nil {
		return generateError
	}
	renderSvc.RegisterFile(indexPath)

	// Persist the new hash for future builds.
	if err := managerInstance.cache.SetSearchHash(newHash); err != nil {
		managerInstance.logger.Warn("Failed to persist search hash", "error", err)
	}

	if managerInstance.health != nil {
		managerInstance.health.RecordSearchStats(int64(len(indexedPosts)), indexSize)
	}

	return nil
}

func (managerInstance *Manager) calculateSearchHash(posts []models.IndexedPost) string {
	if len(posts) == 0 {
		return ""
	}

	// Sort copy to ensure deterministic hash.
	sorted := make([]models.IndexedPost, len(posts))
	copy(sorted, posts)
	sort.Slice(sorted, func(i, j int) bool {
		return indexedPostStableKey(sorted[i]) < indexedPostStableKey(sorted[j])
	})

	hasher := xxh3.New()
	for _, p := range sorted {
		hasher.WriteString(indexedPostStableKey(p))
		// Use the Title and some content-based field for hashing.
		// Since BM25Data (WordFreqs) represents the searchable content, it's a good proxy.
		hasher.WriteString(p.Record.Title)
		hasher.WriteString(p.Record.Description)
	}

	return fmt.Sprintf("%x", hasher.Sum64())
}

func (managerInstance *Manager) ensureIndexedPosts() ([]models.IndexedPost, error) {
	managerInstance.mu.RLock()
	if len(managerInstance.indexedPosts) > 0 {
		posts := managerInstance.indexedPosts
		managerInstance.mu.RUnlock()
		return posts, nil
	}
	managerInstance.mu.RUnlock()

	if managerInstance.cache == nil {
		return nil, nil
	}

	postIDs, cacheError := managerInstance.cache.ListAllPosts()
	if cacheError != nil {
		return nil, cacheError
	}
	if len(postIDs) == 0 {
		return nil, nil
	}

	posts, postsError := managerInstance.cache.GetPostsByIDs(postIDs)
	if postsError != nil {
		return nil, postsError
	}
	searchRecords, recordsError := managerInstance.cache.GetSearchRecords(postIDs)
	if recordsError != nil {
		return nil, recordsError
	}

	sort.Strings(postIDs)
	indexedPosts := make([]models.IndexedPost, 0, len(posts))
	var mu sync.Mutex

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU())

	for _, id := range postIDs {
		postID := id
		g.Go(func() error {
			metadata, ok := posts[postID]
			if !ok || metadata == nil {
				return nil
			}
			record, ok := searchRecords[postID]
			if !ok || record == nil {
				return nil
			}

			htmlRelativePath := fspkg.MarkdownToHTMLPath(metadata.Path)
			indexed := models.IndexedPost{
				Record: models.PostRecord{
					ID:              xxh3.HashString(htmlRelativePath),
					Title:           metadata.Title,
					NormalizedTitle: record.NormalizedTitle,
					Link:            htmlRelativePath,
					Description:     metadata.Description,
					Taxonomies:      metadata.Taxonomies,
					NormalizedTaxs:  record.NormalizedTaxs,
					Content:         record.Content,
					Date:            metadata.Date.Unix(),
				},
				SourcePath:      metadata.Path,
				WordFreqs:       record.BM25Data,
				DocLen:          record.DocLen,
				StemMap:         record.StemMap,
				PositionalIndex: record.PositionalIndex,
				ByteOffsets:     record.ByteOffsets,
			}

			mu.Lock()
			indexedPosts = append(indexedPosts, indexed)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return indexedPosts, nil
}

func indexedPostStableKey(indexedPost models.IndexedPost) string {
	if indexedPost.SourcePath != "" {
		return fspkg.NormalizePath(indexedPost.SourcePath)
	}
	return fspkg.NormalizePath(indexedPost.Record.Link)
}

func dedupeIndexedPosts(posts []models.IndexedPost) []models.IndexedPost {
	if len(posts) < 2 {
		return posts
	}
	seen := make(map[string]int, len(posts))
	result := make([]models.IndexedPost, 0, len(posts))
	for _, indexedPost := range posts {
		key := indexedPostStableKey(indexedPost)
		if index, ok := seen[key]; ok {
			result[index] = indexedPost
			continue
		}
		seen[key] = len(result)
		result = append(result, indexedPost)
	}
	return result
}
