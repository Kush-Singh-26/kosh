package post

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

// ContentResult contains the aggregated results of content processing
type ContentResult struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	Taxonomies     map[string]models.TaxonomyData
	TaxonomyMap    map[string]map[string][]models.PostMetadata
	IndexedPosts   []models.IndexedPost
	AnyPostChanged bool
	Has404         bool
}

// ContentContext holds aggregated content metadata ready for site-wide generators.
type ContentContext struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	Taxonomies     map[string]models.TaxonomyData
	TaxonomyMap    map[string]map[string][]models.PostMetadata
	IndexedPosts        []models.IndexedPost
	PrebuiltSearchIndex *models.SearchIndex
	AnyPostChanged      bool
}

// ToContentContext converts a ContentResult into its ContentContext subset.
func (pr *ContentResult) ToContentContext() *ContentContext {
	return &ContentContext{
		AllPosts:       pr.AllPosts,
		PinnedPosts:    pr.PinnedPosts,
		Taxonomies:     pr.Taxonomies,
		TaxonomyMap:    pr.TaxonomyMap,
		IndexedPosts:        pr.IndexedPosts,
		PrebuiltSearchIndex: nil, // Will be set by orchestration if needed
		AnyPostChanged:      pr.AnyPostChanged,
	}
}

// Cache is a narrowed interface for PostService.
type Cache interface {
	models.PostCache
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
	ReconfigureWithReporter(r ui.Reporter, l *slog.Logger)
	Process(opts ProcessOptions) (*ContentResult, error)
	ProcessStreaming(opts ProcessOptions) (*ContentResult, error)
	ProcessSingle(ctx context.Context, path string, source []byte) error
	ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *ParsedMarkdownResult) error
	GetMetadataContext(ctx context.Context) (*ContentContext, error)
	WaitForCacheCommit()
}
