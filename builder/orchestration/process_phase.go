package orchestration

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

type postStreamRes struct {
	res *post.PostResult
	err error
}

func (b *Engine) startPostProcessingStream(ctx context.Context, setup *buildSetupResult, scan *buildScanResult) chan postStreamRes {
	postResChan := make(chan postStreamRes, 1)
	logger := b.Deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	async.FireAndForget(ctx, logger, "post processing stream", func() error {
		res, err := b.Deps.Post.ProcessStreaming(post.ProcessOptions{
			Ctx:                ctx,
			ShouldForce:        b.Cfg.ForceRebuild,
			ForceSocialRebuild: setup.forceSocialRebuild,
			OutputMissing:      b.State.IsCleanBuild,
			FileChan:           scan.fileChan,
		})
		postResChan <- postStreamRes{res, err}
		return nil
	})
	return postResChan
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

func waitForPostProcessing(ctx context.Context, postResChan chan postStreamRes) (*post.PostResult, error) {
	select {
	case pr := <-postResChan:
		return pr.res, pr.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// processPhase executes post processing and site-wide orchestration.
// Assets (including image compression) MUST complete before post rendering,
// since post rendering rewrites PNG/JPG/JPEG image references to WebP.
func (b *Engine) processPhase(
	ctx context.Context,
	setup *buildSetupResult,
	assetsRes *buildAssetResult,
	scan *buildScanResult,
) error {
	// Start streaming post processing (ingesting from the scanner's fileChan).
	postResChan := b.startPostProcessingStream(ctx, setup, scan)

	// Wait for scanner and discovery metadata (needed for ContentAssets and site-wide state).
	// This ensures the image/WebP rewrite map is populated so HTML can reference
	// the correct paths. Image compression continues in the background while posts render.
	metadataResult, discoverySignal, scannerErr, assetErr, _ := b.waitForScannerAndAssets(WaitScannerAssetsOptions{
		Ctx:                ctx,
		ScannerReady:       scan.scannerReady,
		MetadataResultChan: scan.metadataResultChan,
		ScannerErrChan:     scan.scannerErrChan,
		AssetWg:            assetsRes.assetWg,
		AssetErrChan:       assetsRes.assetErrChan,
		DiscoveryReady:     assetsRes.discoveryReady,
	})

	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhaseScan, 0)
		b.Deps.Reporter.StartPhase(ui.PhasePosts)
	}

	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}

	// Discovery signal must be ready before templates can be safely executed by the renderer.
	if err := waitForDiscovery(ctx, discoverySignal); err != nil {
		return err
	}

	var siteWideHas404 bool
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Set up site-wide generators (need full asset completion).
	runSiteWide, _ := b.setupSiteWideRendering(SiteWideOptions{
		Ctx:                ctx,
		AssetsReady:        assetsRes.assetsReady,
		WasmWg:             setup.wasmWg,
		ForceSocialRebuild: setup.forceSocialRebuild,
	})

	// Wait for post processing to finish (it was started as a stream).
	postResult, processErr := waitForPostProcessing(ctx, postResChan)
	if processErr != nil {
		return fmt.Errorf("post processing failed: %w", processErr)
	}

	if b.Deps.Reporter != nil {
		b.Deps.Reporter.EndPhase(ui.PhasePosts, 0)
		b.Deps.Reporter.StartPhase(ui.PhaseSiteWide)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Site-wide generators.
	metadataCtx := postResult.ToMetadataContext()
	assetsChanged := b.Assets.CheckChanged(ctx, assetsRes.assetsReady)
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	// Wait for site-wide rendering.
	return b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404 || b.Deps.Render.Has404Template(), metadataCtx)
}

// ProcessPostsOptions configures post processing for a build pass.
type ProcessPostsOptions struct {
	Ctx                context.Context
	ShouldForce        bool
	ForceSocialRebuild bool
	OutputMissing      bool
	Files              []models.ScannedFile
}

// processPosts executes post processing and returns the result.
func (b *Engine) processPosts(opts ProcessPostsOptions) (*post.PostResult, error) {
	return b.Deps.Post.Process(post.ProcessOptions{
		Ctx:                opts.Ctx,
		ShouldForce:        opts.ShouldForce,
		ForceSocialRebuild: opts.ForceSocialRebuild,
		OutputMissing:      opts.OutputMissing,
		Files:              opts.Files,
	})
}
