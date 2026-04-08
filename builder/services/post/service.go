package post

import (
	"log/slog"
	"sync"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// postService implements PostService.
type postService struct {
	ctx            *buildCtx.BuildContext
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
	cacheWg        sync.WaitGroup
}

// NewService constructs the PostService with injected dependencies.
func NewService(deps Dependencies) Service {
	return &postService{
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
	}
}

// ReconfigureForBuild updates the sink and source filesystem for a new build.
func (s *postService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
}

// SetAssetsGate sets the assets-ready signal channel.
func (s *postService) SetAssetsGate(ch <-chan struct{}) { s.assetsReady = ch }

// ReconfigureWithReporter updates the reporter and logger for subsequent builds.
func (s *postService) ReconfigureWithReporter(r ui.Reporter, l *slog.Logger) {
	s.reporter = r
	s.logger = l
}

// WaitForCacheCommit blocks until cache commits complete.
func (s *postService) WaitForCacheCommit() { s.cacheWg.Wait() }
