package post

import (
	"context"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// ShortcodeProcessor processes shortcodes in markdown content.
type ShortcodeProcessor interface {
	Process(markdown []byte) ([]byte, error)
}

type renderTask struct {
	parseResult      *ParsedMarkdownResult
	file             models.ScannedResource
	htmlContent      string
	destinationPath  string
	relativePath     string
	htmlRelativePath string
	source           []byte
}

type searchTask struct {
	record         models.PostRecord
	plainText      string
	indexed        *models.IndexedPost
	cached         *models.SearchRecord
	SearchIngestor models.SearchIngestor
}

// workerLocalState accumulates results within a single parse worker,
// eliminating contention on the shared postProcessContext.
type workerLocalState struct {
	allPosts         []models.PostMetadata
	pinnedItems      []models.PostMetadata
	taxonomyEntries  []taxonomyEntry
	indexedPosts     []models.IndexedPost
	searchTasks      []deferredSearchTask
	newPostsMeta     []*models.PostMeta
	newSearchRecords map[string]*models.SearchRecord
	newDependencies  map[string]*models.Dependencies
	anyChanged       bool
	errs             []error
}

type taxonomyEntry struct {
	taxonomy string
	term     string
	post     models.PostMetadata
}

type deferredSearchTask struct {
	record     models.PostRecord
	plainText  string
	localIndex int // index into this worker's indexedPosts
	cached     *models.SearchRecord
}

// WorkerContext holds shared dependencies and configuration for streaming workers.
type WorkerContext struct {
	Ctx                context.Context
	ProcessContext     *postProcessContext
	CardPool           *async.WorkerPool[socialCardTask]
	SearchPool         *async.WorkerPool[searchTask]
	SearchIngestor     models.SearchIngestor
	RenderChan         chan<- renderTask
	ShouldForce        bool
	ForceSocialRebuild bool
}

type postProcessContext struct {
	allPosts         []models.PostMetadata
	pinnedItems      []models.PostMetadata
	taxonomyMap      map[string]map[string][]models.PostMetadata // Taxonomy -> Term -> Posts
	anyPostChanged   atomic.Bool
	newPostsMeta     []*models.PostMeta
	newSearchRecords map[string]*models.SearchRecord
	newDependencies  map[string]*models.Dependencies
	indexedPosts     []models.IndexedPost
	errs             []error
}

// AggregateContext bundles inputs needed to aggregate a single post result.
type AggregateContext struct {
	Ctx              context.Context
	Result           *ParsedMarkdownResult
	Post             models.PostMetadata
	HTMLContent      string
	DestinationPath  string
	RelativePath     string
	HTMLRelativePath string
	SSRHashes        []string
	UseCache         bool
	WorkerContext    WorkerContext
	Local            *workerLocalState
	SourceBytes      []byte
	ScannedFile      models.ScannedResource
}

// SocialCardOptions configures social card generation for a post.
type SocialCardOptions struct {
	RelativePath       string
	Result             *ParsedMarkdownResult
	HTMLRelativePath   string
	ForceSocialRebuild bool
	CardPool           *async.WorkerPool[socialCardTask]
}
