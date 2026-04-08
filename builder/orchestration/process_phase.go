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
	type postStreamRes struct {
		res *post.PostResult
		err error
	}
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
	if discoverySignal != nil {
		select {
		case <-discoverySignal:
		case <-ctx.Done():
			return ctx.Err()
		}
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
	var postResult *post.PostResult
	var processErr error
	select {
	case pr := <-postResChan:
		postResult = pr.res
		processErr = pr.err
	case <-ctx.Done():
		return ctx.Err()
	}

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

	// Check if assets changed since last site-wide render.
	assetsChanged := b.Assets.CheckChanged(ctx, assetsRes.assetsReady)

	// Site-wide generators.
	metadataCtx := postResult.ToMetadataContext()
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	// Wait for site-wide rendering.
	return b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404 || b.Deps.Render.Has404Template())
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
