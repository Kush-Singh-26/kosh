package orchestration

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

type postStreamResult struct {
	result *content.Result
	error  error
}

func (engineInstance *Engine) startPostProcessingStream(ctx context.Context, setup *buildSetupResult, scan *buildScanResult, searchIngestor searchpkg.SearchIngestor) chan postStreamResult {
	contentResultChan := make(chan postStreamResult, 1)
	logger := engineInstance.Deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	engineInstance.buildWaitGroup.Add(1)
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    logger,
		Operation: "Content processing stream",
		Fn: func() error {
			result, processError := engineInstance.Deps.Content.ProcessStreaming(content.ProcessOptions{
				Ctx:                ctx,
				SearchIngestor:     searchIngestor,
				ShouldForce:        engineInstance.Cfg.ShouldForceRebuild,
				ForceSocialRebuild: setup.forceSocialRebuild,
				ForceRerender:      engineInstance.State.ForceRerender.Load(),
				OutputMissing:      engineInstance.State.IsCleanBuild,
				FileChan:           scan.fileChan,
			})
			contentResultChan <- postStreamResult{result, processError}
			return nil
		},
		Cleanup: engineInstance.buildWaitGroup.Done,
	})
	return contentResultChan
}

func waitForDiscovery(ctx context.Context, discoverySignal <-chan struct{}) error {
	if discoverySignal == nil {
		return nil
	}
	select {
	case <-discoverySignal:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitForPostProcessing(ctx context.Context, contentResultChan chan postStreamResult) (*content.Result, error) {
	select {
	case postStreamRes := <-contentResultChan:
		return postStreamRes.result, postStreamRes.error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (engineInstance *Engine) finalizePostPhase(ctx context.Context, contentResultChan chan postStreamResult, searchStream *index.StreamBuilder) (*content.Result, *searchpkg.SearchIndex, error) {
	contentResult, processError := waitForPostProcessing(ctx, contentResultChan)
	if processError != nil {
		return nil, nil, fmt.Errorf("content processing failed: %w", processError)
	}

	finalSearchIndex := searchStream.Complete()
	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.EndPhase(ui.PhasePosts, 0)
		engineInstance.Deps.Reporter.StartPhase(ui.PhaseSiteWide)
	}
	return contentResult, finalSearchIndex, nil
}

func (engineInstance *Engine) runSiteWidePhase(
	ctx context.Context,
	setup *buildSetupResult,
	assetsRes *buildAssetResult,
	contentResult *content.Result,
	finalSearchIndex *searchpkg.SearchIndex,
) error {
	runSiteWide, _ := engineInstance.setupSiteWideRendering(SiteWideOptions{
		Ctx:                ctx,
		AssetsReadySignal:  assetsRes.assetsReadySignal,
		WasmWaitGroup:      setup.wasmWg,
		ForceSocialRebuild: setup.forceSocialRebuild,
		SearchIndex:        finalSearchIndex,
	})

	metadataCtx := contentResult.ToContext()
	assetsChanged := engineInstance.Assets.CheckChanged(ctx, assetsRes.assetsReadySignal)
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	has404 := contentResult.Has404 || engineInstance.Deps.Render.Has404Template()
	if err := engineInstance.waitForSiteWideRendering(siteWideGroup, siteTimer, has404, metadataCtx); err != nil {
		return fmt.Errorf("site-wide rendering failed: %w", err)
	}

	return engineInstance.flushCaches(ctx)
}

func (engineInstance *Engine) flushCaches(ctx context.Context) error {
	if engineInstance.Deps.Fragments != nil {
		if err := engineInstance.Deps.Fragments.Flush(ctx); err != nil {
			engineInstance.Deps.Logger.Warn("Fragment cache flush failed", "error", err)
		}
	}
	if engineInstance.Deps.Diagrams != nil {
		if err := engineInstance.Deps.Diagrams.Flush(ctx); err != nil {
			engineInstance.Deps.Logger.Warn("Diagram cache flush failed", "error", err)
		}
	}
	return nil
}

// processPhase executes Content processing and site-wide orchestration.
func (engineInstance *Engine) processPhase(
	ctx context.Context,
	setup *buildSetupResult,
	assetsRes *buildAssetResult,
	scan *buildScanResult,
) error {
	searchStream := index.NewStreamBuilder(ctx, 0)
	contentResultChan := engineInstance.startPostProcessingStream(ctx, setup, scan, searchStream)

	_, discoverySignal, scannerError, assetError, _ := engineInstance.waitForScannerAndAssets(WaitScannerAssetsOptions{
		Ctx:                  ctx,
		ScannerReady:         scan.scannerReady,
		MetadataResultChan:   scan.metadataResultChan,
		ScannerErrChan:       scan.scannerErrChan,
		AssetWaitGroup:       assetsRes.assetWaitGroup,
		AssetErrorChan:       assetsRes.assetErrorChan,
		DiscoveryReadySignal: assetsRes.discoveryReadySignal,
	})

	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.EndPhase(ui.PhaseScan, 0)
		engineInstance.Deps.Reporter.StartPhase(ui.PhasePosts)
	}

	if scannerError != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerError)
	}
	if assetError != nil {
		return fmt.Errorf("failed to build assets: %w", assetError)
	}
	if err := waitForDiscovery(ctx, discoverySignal); err != nil {
		return err
	}

	contentResult, finalSearchIndex, err := engineInstance.finalizePostPhase(ctx, contentResultChan, searchStream)
	if err != nil {
		return err
	}

	return engineInstance.runSiteWidePhase(ctx, setup, assetsRes, contentResult, finalSearchIndex)
}
