package orchestration

import (
	"context"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// buildAssetResult holds synchronization primitives for asset building.
type buildAssetResult struct {
	assetsReady    <-chan struct{}
	discoveryReady <-chan struct{} // signals when image rewrite map is populated
	assetWg        *sync.WaitGroup
	assetErrChan   <-chan error
}

// assetPhase starts the asset building pipeline.
func (b *Engine) assetPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildAssetResult {
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.StartPhase(ui.PhaseAssets)
	}
	forceAssetBuild := b.Cfg.ForceRebuild
	assetsReady, discoveryReady, assetWg, assetErrChan := b.Assets.SetupBuilding(ctx, contentAssetsChan, forceAssetBuild)

	return &buildAssetResult{
		assetsReady:    assetsReady,
		discoveryReady: discoveryReady,
		assetWg:        assetWg,
		assetErrChan:   assetErrChan,
	}
}
