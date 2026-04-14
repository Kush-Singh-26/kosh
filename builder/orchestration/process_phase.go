package orchestration

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search/index"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

type postStreamResult struct {
	result *post.ContentResult
	error  error
}

func (engineInstance *Engine) startPostProcessingStream(ctx context.Context, setup *buildSetupResult, scan *buildScanResult, searchIngestor models.SearchIngestor) chan postStreamResult {
	postResultChan := make(chan postStreamResult, 1)
	logger := engineInstance.Deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	async.FireAndForget(ctx, logger, "post processing stream", func() error {
		result, processError := engineInstance.Deps.Post.ProcessStreaming(post.ProcessOptions{
			Ctx:                ctx,
			SearchIngestor:     searchIngestor,
			ShouldForce:        engineInstance.Cfg.ShouldForceRebuild,
			ForceSocialRebuild: setup.forceSocialRebuild,
			OutputMissing:      engineInstance.State.IsCleanBuild,
			FileChan:           scan.fileChan,
		})
		postResultChan <- postStreamResult{result, processError}
		return nil
	})
	return postResultChan
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

func waitForPostProcessing(ctx context.Context, postResultChan chan postStreamResult) (*post.ContentResult, error) {
	select {
	case postStreamRes := <-postResultChan:
		return postStreamRes.result, postStreamRes.error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// processPhase executes post processing and site-wide orchestration.
// Assets (including image compression) MUST complete before post rendering,
// since post rendering rewrites PNG/JPG/JPEG image references to WebP.
func (engineInstance *Engine) processPhase(
	ctx context.Context,
	setup *buildSetupResult,
	assetsRes *buildAssetResult,
	scan *buildScanResult,
) error {
	// Initialize Search Stream Builder early
	searchStream := index.NewStreamBuilder(0) // Will refine expectedDocs when scanner finishes if needed

	// Start streaming post processing (ingesting from the scanner's fileChan).
	postResultChan := engineInstance.startPostProcessingStream(ctx, setup, scan, searchStream)

	// Wait for scanner and discovery metadata (needed for ContentAssets and site-wide state).
	// This ensures the image/WebP rewrite map is populated so HTML can reference
	// the correct paths. Image compression continues in the background while posts render.
	metadataResult, discoverySignal, scannerError, assetError, _ := engineInstance.waitForScannerAndAssets(WaitScannerAssetsOptions{
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

	// Discovery signal must be ready before templates can be safely executed by the renderer.
	if err := waitForDiscovery(ctx, discoverySignal); err != nil {
		return err
	}

	var siteWideHas404 bool
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Wait for post processing to finish (it was started as a stream).
	postResult, processError := waitForPostProcessing(ctx, postResultChan)
	if processError != nil {
		return fmt.Errorf("post processing failed: %w", processError)
	}

	// Finalize search index construction (pipelined)
	finalSearchIndex := searchStream.Complete()

	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.EndPhase(ui.PhasePosts, 0)
		engineInstance.Deps.Reporter.StartPhase(ui.PhaseSiteWide)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Set up site-wide generators (need full asset completion).
	runSiteWide, _ := engineInstance.setupSiteWideRendering(SiteWideOptions{
		Ctx:                ctx,
		AssetsReadySignal:  assetsRes.assetsReadySignal,
		WasmWaitGroup:      setup.wasmWg,
		ForceSocialRebuild: setup.forceSocialRebuild,
		SearchIndex:        finalSearchIndex,
	})

	// Site-wide generators.
	metadataCtx := postResult.ToContentContext()
	assetsChanged := engineInstance.Assets.CheckChanged(ctx, assetsRes.assetsReadySignal)
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	// Wait for site-wide rendering.
	if err := engineInstance.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404 || engineInstance.Deps.Render.Has404Template(), metadataCtx); err != nil {
		return fmt.Errorf("site-wide rendering failed: %w", err)
	}

	// Persist cached objects (fragments, diagrams) for reuse in next build.
	// We flush here to ensure data is committed even if SaveCaches is interrupted later.
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

// ProcessPostsOptions configures content processing for a build pass.
type ProcessPostsOptions struct {
	Ctx                context.Context
	ShouldForce        bool
	ForceSocialRebuild bool
	OutputMissing      bool
	Files              []models.ScannedResource
}

// processPosts executes content processing and returns the result.
func (engineInstance *Engine) processPosts(opts ProcessPostsOptions) (*post.ContentResult, error) {
	return engineInstance.Deps.Post.Process(post.ProcessOptions{
		Ctx:                opts.Ctx,
		ShouldForce:        opts.ShouldForce,
		ForceSocialRebuild: opts.ForceSocialRebuild,
		OutputMissing:      opts.OutputMissing,
		Files:              opts.Files,
	})
}
