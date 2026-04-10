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

// PostResult contains the aggregated results of post processing
type PostResult struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	TagMap         map[string][]models.PostMetadata
	AllTags        []models.TagData
	IndexedPosts   []models.IndexedPost
	AnyPostChanged bool
	Has404         bool
}

// MetadataContext holds aggregated post metadata ready for site-wide generators.
type MetadataContext struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	TagMap         map[string][]models.PostMetadata
	AllTags        []models.TagData
	IndexedPosts   []models.IndexedPost
	AnyPostChanged bool
}

// ToMetadataContext converts a PostResult into its MetadataContext subset.
func (pr *PostResult) ToMetadataContext() *MetadataContext {
	return &MetadataContext{
		AllPosts:       pr.AllPosts,
		PinnedPosts:    pr.PinnedPosts,
		TagMap:         pr.TagMap,
		AllTags:        pr.AllTags,
		IndexedPosts:   pr.IndexedPosts,
		AnyPostChanged: pr.AnyPostChanged,
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
	Reporter       ui.Reporter
}

// Parser handles markdown parsing and processing
type Parser interface {
	ParseMarkdownMetadata(opts ParseOptions) (*ParsedMarkdownResult, error)
	ParseMarkdown(opts ParseOptions) (*ParsedMarkdownResult, error)
}

// ProcessOptions configures post processing operations.
type ProcessOptions struct {
	Ctx                context.Context
	ShouldForce        bool
	ForceSocialRebuild bool
	OutputMissing      bool
	Files              []models.ScannedFile
	FileChan           <-chan models.ScannedFile
}

// Service handles markdown parsing and post processing.
type Service interface {
	ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs)
	SetAssetsGate(ch <-chan struct{})
	ReconfigureWithReporter(r ui.Reporter, l *slog.Logger)
	Process(opts ProcessOptions) (*PostResult, error)
	ProcessStreaming(opts ProcessOptions) (*PostResult, error)
	ProcessSingle(ctx context.Context, path string, source []byte) error
	ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *ParsedMarkdownResult) error
	GetMetadataContext(ctx context.Context) (*MetadataContext, error)
	WaitForCacheCommit()
}
