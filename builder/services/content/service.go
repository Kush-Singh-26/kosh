package content

import (
	"html/template"
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
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// postService implements PostService.
type contentService struct {
	ctx            *buildctx.BuildContext
	cfg            *config.Config
	cache          Cache
	renderer       render.Service
	logger         *slog.Logger
	metrics        *metrics.BuildMetrics
	mdPool         *sync.Pool
	nativeRenderer *native.Renderer
	sourceFs       afero.Fs
	sink           fspkg.ArtifactSink
	reporter       ui.Reporter
	assetsReady    <-chan struct{}
	diagramAdapter *cache.DiagramCacheAdapter
	fragments      *cache.FragmentCacheAdapter
	shortcodes     ShortcodeProcessor
	cacheWg        sync.WaitGroup
}

// NewService constructs the PostService with injected dependencies.
func NewService(deps Dependencies) Service {
	return &contentService{
		ctx:            deps.Ctx,
		cfg:            deps.Cfg,
		cache:          deps.Cache,
		renderer:       deps.Renderer,
		logger:         deps.Logger,
		metrics:        deps.Metrics,
		mdPool:         deps.MdPool,
		nativeRenderer: deps.NativeRenderer,
		sourceFs:       deps.SourceFs,
		sink:           deps.Sink,
		reporter:       deps.Reporter,
		diagramAdapter: deps.DiagramAdapter,
		fragments:      deps.Fragments,
		shortcodes:     deps.Shortcodes,
	}
}

// ReconfigureForBuild updates the sink and source filesystem for a new build.
func (service *contentService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	service.sink = sink
	service.sourceFs = fs
}

// SetAssetsGate sets the assets-ready signal channel.
func (service *contentService) SetAssetsGate(assetsReadyChan <-chan struct{}) {
	service.assetsReady = assetsReadyChan
}

// ReconfigureWithReporter updates the reporter and logger for subsequent builds.
func (service *contentService) ReconfigureWithReporter(reporter ui.Reporter, logger *slog.Logger) {
	service.reporter = reporter
	service.logger = logger
}

// WaitForCacheCommit blocks until cache commits complete.
func (service *contentService) WaitForCacheCommit() { service.cacheWg.Wait() }

func (service *contentService) generateJSONLD(item models.ContentMetadata, cardImageURL string) template.HTML {
	jsonld, err := models.GenerateContentJSONLD(item, service.cfg.Author, cardImageURL, service.cfg.ArticleType)
	if err != nil {
		service.logger.Error("Failed to generate JSON-LD", "post", item.Link, "error", err)
		return ""
	}
	return jsonld
}
