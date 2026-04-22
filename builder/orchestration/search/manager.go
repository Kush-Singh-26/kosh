package search

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"
	"runtime"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	svcCache "github.com/Kush-Singh-26/kosh/builder/services/cache"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
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
	indexedPosts []searchpkg.IndexedContent
}

// HealthRegistry records search-related health metrics.
type HealthRegistry interface {
	RecordSearchStats(docs int64, size int64, configured bool, wasmSync bool)
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
func (managerInstance *Manager) SetIndexedPosts(posts []searchpkg.IndexedContent) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()
	managerInstance.indexedPosts = posts
}

// GetIndexedPosts returns the current indexed posts cache.
func (managerInstance *Manager) GetIndexedPosts() []searchpkg.IndexedContent {
	managerInstance.mu.RLock()
	defer managerInstance.mu.RUnlock()
	return managerInstance.indexedPosts
}

// UpdateIndexedContentCache updates or inserts a single indexed Content entry.
func (managerInstance *Manager) UpdateIndexedContentCache(relativePath string, parseResult *content.ParsedMarkdownResult) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()

	if len(managerInstance.indexedPosts) == 0 {
		return
	}

	found := false
	targetKey := fspkg.NormalizePath(relativePath)

	// Canonicalize the search record link to a relative HTML path to avoid duplicates
	// and malformed double-prefixed paths in incremental dev builds.
	record := parseResult.SearchRecord
	record.Link = fspkg.MarkdownToHTMLPath(relativePath)

	for index, IndexedContent := range managerInstance.indexedPosts {
		if indexedPostStableKey(IndexedContent) == targetKey {
			managerInstance.indexedPosts[index] = searchpkg.IndexedContent{
				Record:          record,
				SourcePath:      targetKey,
				WordFreqs:       parseResult.WordFreqs,
				DocLen:          parseResult.DocLen,
				StemMap:         parseResult.StemMap,
				PositionalIndex: parseResult.PositionalIndex,
			}
			found = true
			break
		}
	}

	if !found {
		managerInstance.indexedPosts = append(managerInstance.indexedPosts, searchpkg.IndexedContent{
			Record:          record,
			SourcePath:      targetKey,
			WordFreqs:       parseResult.WordFreqs,
			DocLen:          parseResult.DocLen,
			StemMap:         parseResult.StemMap,
			PositionalIndex: parseResult.PositionalIndex,
		})
	}
}

// PruneDeletedItem removes a Content from the indexed cache.
func (managerInstance *Manager) PruneDeletedItem(relativePath string) {
	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()

	targetKey := fspkg.NormalizePath(relativePath)
	newIndexed := make([]searchpkg.IndexedContent, 0, len(managerInstance.indexedPosts))
	for _, IndexedContent := range managerInstance.indexedPosts {
		if indexedPostStableKey(IndexedContent) != targetKey {
			newIndexed = append(newIndexed, IndexedContent)
		}
	}
	managerInstance.indexedPosts = newIndexed
}

// RegenerateIndex rebuilds and writes the search index.
func (managerInstance *Manager) RegenerateIndex(_ context.Context) error {
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
	if managerInstance.cache != nil && !managerInstance.cfg.ShouldForceRebuild {
		if cachedHash, err := managerInstance.cache.GetSearchHash(); err == nil && cachedHash == newHash {
			managerInstance.logger.Debug("Search index unchanged, skipping generation", "hash", newHash)
			return nil
		}
	}

	managerInstance.mu.Lock()
	managerInstance.indexedPosts = indexedPosts
	managerInstance.mu.Unlock()

	indexPath, indexSize, generateError := generators.GenerateSearchIndex(sink, indexedPosts, managerInstance.cfg.Features.Generators.Search.Ranking)
	if generateError != nil {
		return generateError
	}
	renderSvc.RegisterFile(indexPath)

	// Persist the new hash for future builds.
	if managerInstance.cache != nil {
		if err := managerInstance.cache.SetSearchHash(newHash); err != nil {
			managerInstance.logger.Warn("Failed to persist search hash", "error", err)
		}
	}

	if managerInstance.health != nil {
		isEnabled := managerInstance.cfg.Features.Generators.Search.IsEnabled
		// Search is always "in sync" if regenerated successfully here
		managerInstance.health.RecordSearchStats(int64(len(indexedPosts)), indexSize, isEnabled, true)
	}

	return nil
}

func (managerInstance *Manager) calculateSearchHash(posts []searchpkg.IndexedContent) string {
	if len(posts) == 0 {
		return ""
	}

	// Sort copy to ensure deterministic hash.
	sorted := make([]searchpkg.IndexedContent, len(posts))
	copy(sorted, posts)
	sort.Slice(sorted, func(i, j int) bool {
		return indexedPostStableKey(sorted[i]) < indexedPostStableKey(sorted[j])
	})

	hasher := xxh3.New()
	// Include schema version and logic version to force rebuild on engine changes
	_, _ = hasher.WriteString(strconv.Itoa(searchpkg.CurrentSchemaVersion))
	
	// Include ranking configuration so changing kosh.yaml triggers a re-index
	ranking := managerInstance.cfg.Features.Generators.Search.Ranking
	_, _ = hasher.WriteString(strconv.FormatFloat(ranking.TitleBoost, 'f', -1, 64))
	_, _ = hasher.WriteString(strconv.FormatFloat(ranking.TagBoost, 'f', -1, 64))
	_, _ = hasher.WriteString(strconv.FormatFloat(ranking.DescriptionBoost, 'f', -1, 64))
	_, _ = hasher.WriteString(strconv.FormatFloat(ranking.BM25K1, 'f', -1, 64))
	_, _ = hasher.WriteString(strconv.FormatFloat(ranking.BM25B, 'f', -1, 64))

	for _, p := range sorted {
		_, _ = hasher.WriteString(indexedPostStableKey(p))
		// Use the Title and some content-based field for hashing.
		_, _ = hasher.WriteString(p.Record.Title)
		_, _ = hasher.WriteString(p.Record.Description)
	}

	return fmt.Sprintf("%x", hasher.Sum64())
}

func (managerInstance *Manager) ensureIndexedPosts() ([]searchpkg.IndexedContent, error) {
	managerInstance.mu.RLock()
	if len(managerInstance.indexedPosts) > 0 {
		posts := managerInstance.indexedPosts
		managerInstance.mu.RUnlock()
		return posts, nil
	}
	managerInstance.mu.RUnlock()

	postIDs, posts, records, err := managerInstance.getPostsFromCache()
	if err != nil || len(postIDs) == 0 {
		return nil, err
	}

	return managerInstance.buildIndexedPostsParallel(postIDs, posts, records)
}

func (managerInstance *Manager) getPostsFromCache() ([]string, map[string]*models.ContentMeta, map[string]*models.SearchRecord, error) {
	if managerInstance.cache == nil {
		return nil, nil, nil, nil
	}

	postIDs, err := managerInstance.cache.ListAllItems()
	if err != nil || len(postIDs) == 0 {
		return nil, nil, nil, err
	}

	posts, err := managerInstance.cache.GetItemsByIDs(postIDs)
	if err != nil {
		return nil, nil, nil, err
	}

	records, err := managerInstance.cache.GetSearchRecords(postIDs)
	if err != nil {
		return nil, nil, nil, err
	}

	sort.Strings(postIDs)
	return postIDs, posts, records, nil
}

func (managerInstance *Manager) buildIndexedPostsParallel(ids []string, posts map[string]*models.ContentMeta, records map[string]*models.SearchRecord) ([]searchpkg.IndexedContent, error) {
	indexedPosts := make([]searchpkg.IndexedContent, 0, len(posts))
	var mu sync.Mutex

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU())

	for _, id := range ids {
		ContentID := id
		g.Go(func() error {
			metadata, ok := posts[ContentID]
			record, ok2 := records[ContentID]
			if !ok || metadata == nil || !ok2 || record == nil {
				return nil
			}

			indexed := searchpkg.IndexedContent{
				Record: searchpkg.ContentRecord{
					Title:           metadata.Title,
					Link:            metadata.Link,
					Description:     metadata.Description,
					Taxonomies:      metadata.Taxonomies,
					NormalizedTaxs:  record.NormalizedTaxs,
					Content:         record.Content,
					Date:            metadata.Date.Unix(),
				},
				SourcePath:      metadata.Path,
				WordFreqs:       record.WordFreqs,
				DocLen:          record.DocLen,
				StemMap:         record.StemMap,
				PositionalIndex: record.PositionalIndex,
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

func indexedPostStableKey(indexedContent searchpkg.IndexedContent) string {
	if indexedContent.SourcePath != "" {
		return fspkg.NormalizePath(indexedContent.SourcePath)
	}
	return fspkg.NormalizePath(indexedContent.Record.Link)
}

func dedupeIndexedPosts(posts []searchpkg.IndexedContent) []searchpkg.IndexedContent {
	if len(posts) < 2 {
		return posts
	}
	seen := make(map[string]int, len(posts))
	result := make([]searchpkg.IndexedContent, 0, len(posts))
	for _, IndexedContent := range posts {
		key := indexedPostStableKey(IndexedContent)
		if index, ok := seen[key]; ok {
			result[index] = IndexedContent
			continue
		}
		seen[key] = len(result)
		result = append(result, IndexedContent)
	}
	return result
}
