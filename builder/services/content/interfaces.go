package content

import (
	"context"
	"log/slog"
	"sync"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	renderSvc "github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// Result contains the aggregated results of content processing
type Result struct {
	allItems       []models.ContentMetadata
	PinnedItems    []models.ContentMetadata
	Taxonomies     map[string]models.TaxonomyData
	TaxonomyMap    map[string]map[string][]models.ContentMetadata
	indexedItems   []models.IndexedContent
	anyItemChanged bool
	Has404         bool
}

// Context holds aggregated content metadata ready for site-wide generators.
type Context struct {
	AllItems            []models.ContentMetadata
	PinnedItems         []models.ContentMetadata
	Taxonomies          map[string]models.TaxonomyData
	TaxonomyMap         map[string]map[string][]models.ContentMetadata
	IndexedItems        []models.IndexedContent
	PrebuiltSearchIndex *models.SearchIndex
	AnyItemChanged      bool
}

// ToContext converts a Result into its Context subset.
func (pr *Result) ToContext() *Context {
	return &Context{
		AllItems:            pr.allItems,
		PinnedItems:         pr.PinnedItems,
		Taxonomies:          pr.Taxonomies,
		TaxonomyMap:         pr.TaxonomyMap,
		IndexedItems:        pr.indexedItems,
		PrebuiltSearchIndex: nil, // Will be set by orchestration if needed
		AnyItemChanged:      pr.anyItemChanged,
	}
}

// Cache is a narrowed interface for ContentService.
type Cache interface {
	models.ContentCache
	models.SearchCache
	models.SocialCardCache
	models.BuildArtifactCache
}

// Dependencies holds all dependencies for PostService.
type Dependencies struct {
	Ctx            *buildctx.BuildContext
	Cfg            *config.Config
	Cache          Cache
	Renderer       renderSvc.Service
	Logger         *slog.Logger
	Metrics        *metrics.BuildMetrics
	MdPool         *sync.Pool
	NativeRenderer *native.Renderer
	SourceFs       afero.Fs
	Sink           fspkg.ArtifactSink
	DiagramAdapter *cache.DiagramCacheAdapter
	Fragments      *cache.FragmentCacheAdapter
	Reporter       ui.Reporter
	Health         models.HealthRecorder
	Shortcodes     ShortcodeProcessor
}

// Parser handles markdown parsing and processing
type Parser interface {
	ParseMarkdownMetadata(opts ParseOptions) (*ParsedMarkdownResult, error)
	ParseMarkdown(opts ParseOptions) (*ParsedMarkdownResult, error)
}

// ProcessOptions configures post processing operations.
type ProcessOptions struct {
	SearchIngestor     models.SearchIngestor
	Ctx                context.Context
	ShouldForce        bool
	ForceSocialRebuild bool
	OutputMissing      bool
	Files              []models.ScannedResource
	FileChan           <-chan models.ScannedResource
}

// Service handles markdown parsing and content processing.
type Service interface {
	ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs)
	SetAssetsGate(ch <-chan struct{})
	SetMarkdownRenderer(renderer func([]byte) ([]byte, error))
	ReconfigureWithReporter(r ui.Reporter, l *slog.Logger)
	Process(opts ProcessOptions) (*Result, error)
	ProcessStreaming(opts ProcessOptions) (*Result, error)
	ProcessSingle(ctx context.Context, path string, source []byte) error
	ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *ParsedMarkdownResult) error
	ProcessShortcodes(source []byte) ([]byte, error)
	GetMetadataContext(ctx context.Context) (*Context, error)
	WaitForCacheCommit()
}
