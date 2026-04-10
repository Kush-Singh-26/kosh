package orchestration

import (
	"context"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// buildAssetResult holds synchronization primitives for asset building.
type buildAssetResult struct {
	assetsReadySignal    <-chan struct{}
	discoveryReadySignal <-chan struct{} // signals when image rewrite map is populated
	assetWaitGroup       *sync.WaitGroup
	assetErrorChan       <-chan error
}

// assetPhase starts the asset building pipeline.
func (engineInstance *Engine) assetPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildAssetResult {
	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.StartPhase(ui.PhaseAssets)
	}
	forceAssetBuild := engineInstance.Cfg.ShouldForceRebuild
	assetsReadySignal, discoveryReadySignal, assetWaitGroup, assetErrorChan := engineInstance.Assets.SetupBuilding(ctx, contentAssetsChan, forceAssetBuild)

	return &buildAssetResult{
		assetsReadySignal:    assetsReadySignal,
		discoveryReadySignal: discoveryReadySignal,
		assetWaitGroup:       assetWaitGroup,
		assetErrorChan:       assetErrorChan,
	}
}
