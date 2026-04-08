package asset

import (
	"log/slog"
	"sync"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// Option configures optional parameters for AssetService.
type Option func(*assetService)

// WithMetrics sets the build metrics collector.
func WithMetrics(m *metrics.BuildMetrics) Option {
	return func(s *assetService) { s.metrics = m }
}

// WithAssetsReadySignal sets the channel signaled when assets are ready.
func WithAssetsReadySignal(ch chan struct{}) Option {
	return func(s *assetService) { s.assetsReady = ch }
}

// WithContentAssetsChannel sets the channel for content asset notifications.
func WithContentAssetsChannel(ch <-chan []models.ScannedAsset) Option {
	return func(s *assetService) { s.contentAssetsChan = ch }
}

// assetService implements AssetService.
type assetService struct {
	ctx               *buildCtx.BuildContext
	sourceFs          afero.Fs
	sink              fspkg.ArtifactSink
	cfg               *config.Config
	renderer          render.Service
	logger            *slog.Logger
	metrics           *metrics.BuildMetrics
	reporter          ui.Reporter
	contentAssetsChan <-chan []models.ScannedAsset
	assetsReady       chan struct{}
	discoveryReady    chan struct{}
	warnOnce          sync.Map
}

// NewService constructs the AssetService with injected dependencies.
func NewService(deps Dependencies, opts ...Option) Service {
	s := &assetService{
		ctx:      deps.Ctx,
		sourceFs: deps.SourceFs,
		sink:     deps.Sink,
		cfg:      deps.Cfg,
		renderer: deps.Renderer,
		logger:   deps.Logger,
		metrics:  deps.Metrics,
		reporter: deps.Reporter,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ReconfigureForBuild updates the sink and source filesystem for a new build.
func (s *assetService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.sink = sink
	s.sourceFs = fs
}

// SetMetrics sets the build metrics collector.
func (s *assetService) SetMetrics(m *metrics.BuildMetrics) { s.metrics = m }

// SetAssetsReadySignal sets the signal channel for asset completion.
func (s *assetService) SetAssetsReadySignal(ch chan struct{}) { s.assetsReady = ch }

// SetDiscoveryReady sets the signal channel for discovery completion.
func (s *assetService) SetDiscoveryReady(ch chan struct{}) { s.discoveryReady = ch }

// SetContentAssetsChannel sets the channel for content asset notifications.
func (s *assetService) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	s.contentAssetsChan = ch
}

// ReconfigureWithReporter updates the reporter and logger for subsequent builds.
func (s *assetService) ReconfigureWithReporter(r ui.Reporter, l *slog.Logger) {
	s.reporter = r
	s.logger = l
}

// DiscoveryReady returns a channel that is closed when discovery completes.
func (s *assetService) DiscoveryReady() <-chan struct{} {
	return s.discoveryReady
}
