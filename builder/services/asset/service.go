package asset

import (
	"log/slog"
	"sync"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// Option configures optional parameters for AssetService.
type Option func(*assetService)

// WithMetrics sets the build metrics collector.
func WithMetrics(metrics *metrics.BuildMetrics) Option {
	return func(service *assetService) { service.metrics = metrics }
}

// WithAssetsReadySignal sets the channel signaled when assets are ready.
func WithAssetsReadySignal(readySignal chan struct{}) Option {
	return func(service *assetService) { service.assetsReady = readySignal }
}

// WithContentAssetsChannel sets the channel for content asset notifications.
func WithContentAssetsChannel(assetsChannel <-chan []models.ScannedAsset) Option {
	return func(service *assetService) { service.contentAssetsChan = assetsChannel }
}

// assetService implements AssetService.
type assetService struct {
	ctx               *buildctx.BuildContext
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
func NewService(dependencies Dependencies, options ...Option) Service {
	service := &assetService{
		ctx:      dependencies.Ctx,
		sourceFs: dependencies.SourceFs,
		sink:     dependencies.Sink,
		cfg:      dependencies.Cfg,
		renderer: dependencies.Renderer,
		logger:   dependencies.Logger,
		metrics:  dependencies.Metrics,
		reporter: dependencies.Reporter,
	}

	for _, option := range options {
		option(service)
	}

	return service
}

// ReconfigureForBuild updates the sink and source filesystem for a new build.
func (service *assetService) ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs) {
	service.sink = sink
	service.sourceFs = sourceFs
}

// SetMetrics sets the build metrics collector.
func (service *assetService) SetMetrics(metrics *metrics.BuildMetrics) { service.metrics = metrics }

// SetAssetsReadySignal sets the signal channel for asset completion.
func (service *assetService) SetAssetsReadySignal(readySignal chan struct{}) {
	service.assetsReady = readySignal
}

// SetDiscoveryReady sets the signal channel for discovery completion.
func (service *assetService) SetDiscoveryReady(readySignal chan struct{}) {
	service.discoveryReady = readySignal
}

// SetContentAssetsChannel sets the channel for content asset notifications.
func (service *assetService) SetContentAssetsChannel(assetsChannel <-chan []models.ScannedAsset) {
	service.contentAssetsChan = assetsChannel
}

// ReconfigureWithReporter updates the reporter and logger for subsequent builds.
func (service *assetService) ReconfigureWithReporter(reporter ui.Reporter, logger *slog.Logger) {
	service.reporter = reporter
	service.logger = logger
}

// DiscoveryReady returns a channel that is closed when discovery completes.
func (service *assetService) DiscoveryReady() <-chan struct{} {
	return service.discoveryReady
}
