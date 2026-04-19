package content

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// ShortcodeProcessor processes shortcodes in markdown content.
type ShortcodeProcessor interface {
	Process(markdown []byte) ([]byte, error)
	SetRenderer(renderer func([]byte) ([]byte, error))
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
	record         models.ContentRecord
	plainText      string
	indexed        *models.IndexedContent
	cached         *models.SearchRecord
	SearchIngestor models.SearchIngestor
}

// workerLocalState accumulates results within a single parse worker,
// eliminating contention on the shared contentProcessContext.
type workerLocalState struct {
	mu              sync.Mutex
	allItems         []models.ContentMetadata
	pinnedItems      []models.ContentMetadata
	taxonomyEntries  []taxonomyEntry
	indexedItems     []models.IndexedContent
	searchTasks      []deferredSearchTask
	newItemsMeta     []*models.ContentMeta
	newSearchRecords map[string]*models.SearchRecord
	newDependencies  map[string]*models.Dependencies
	anyChanged       bool
	errs             []error
}

type taxonomyEntry struct {
	taxonomy string
	term     string
	item     models.ContentMetadata
}

type deferredSearchTask struct {
	record     models.ContentRecord
	plainText  string
	localIndex int // index into this worker's indexedItems
	cached     *models.SearchRecord
}

// WorkerContext holds shared dependencies and configuration for streaming workers.
type WorkerContext struct {
	Ctx                context.Context
	ProcessContext     *contentProcessContext
	CardPool           *async.WorkerPool[socialCardTask]
	SearchPool         *async.WorkerPool[searchTask]
	SearchIngestor     models.SearchIngestor
	RenderChan         chan<- renderTask
	ShouldForce        bool
	ForceSocialRebuild bool
}

type contentProcessContext struct {
	allItems         []models.ContentMetadata
	pinnedItems      []models.ContentMetadata
	taxonomyMap      map[string]map[string][]models.ContentMetadata // Taxonomy -> Term -> Items
	anyItemChanged   atomic.Bool
	newItemsMeta     []*models.ContentMeta
	newSearchRecords map[string]*models.SearchRecord
	newDependencies  map[string]*models.Dependencies
	indexedItems     []models.IndexedContent
	errs             []error
}

// AggregateContext bundles inputs needed to aggregate a single item result.
type AggregateContext struct {
	Ctx              context.Context
	Result           *ParsedMarkdownResult
	Item             models.ContentMetadata
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
