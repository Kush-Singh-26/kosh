package asset

import (
	"context"
	"log/slog"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// Dependencies holds all dependencies for AssetService.
type Dependencies struct {
	Ctx      *buildctx.BuildContext
	SourceFs afero.Fs
	Sink     fspkg.ArtifactSink
	Cfg      *config.Config
	Renderer render.Service
	Logger   *slog.Logger
	Metrics  *metrics.BuildMetrics
	Reporter ui.Reporter
}

// Service handles static asset processing (CSS/JS bundling, image optimization).
type Service interface {
	ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs)
	SetMetrics(metrics *metrics.BuildMetrics)
	SetAssetsReadySignal(signal chan struct{})
	SetContentAssetsChannel(signal <-chan []models.ScannedAsset)
	SetDiscoveryReady(signal chan struct{})
	ReconfigureWithReporter(reporter ui.Reporter, logger *slog.Logger)
	Build(ctx context.Context) error
	BuildWithOptions(ctx context.Context, skipImages bool) error
	DiscoveryReady() <-chan struct{}
	BuildForAssetChange(ctx context.Context) (map[string]string, error)
	BuildForAssetChangeWithOptions(ctx context.Context, forceImages bool) (map[string]string, error)
}
