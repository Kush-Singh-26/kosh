package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"golang.org/x/sync/errgroup"
)

// buildSetupResult holds data from the initial setup phase
type buildSetupResult struct {
	wasmWg             *sync.WaitGroup
	forceSocialRebuild bool
}

// buildAssetResult holds synchronization primitives for asset building
type buildAssetResult struct {
	assetsReady    <-chan struct{}
	discoveryReady <-chan struct{} // signals when image rewrite map is populated
	assetWg        *sync.WaitGroup
	assetErrChan   <-chan error
}

// buildScanResult holds channels for the parallel metadata scan
type buildScanResult struct {
	fileChan           <-chan models.ScannedFile
	scannerReady       <-chan struct{}
	metadataResultChan <-chan *models.MetadataScannerResult
	scannerErrChan     <-chan error
}

// setupPhase handles early build configuration and project-wide setup
func (b *Engine) setupPhase(ctx context.Context) (*buildSetupResult, error) {
	if b.Metrics != nil {
		b.Metrics.Reset()
	}

	if b.Health != nil {
		b.Health.Reset()
	}

	// Always start each full build pass with a fresh session/tracking state
	b.refreshBuildSession()

	// Check for cancellation early
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Project-wide setup
	wasmWg := b.setupWasmDeployment(ctx)

	// Handle incremental social card rebuild if needed
	forceSocialRebuild := b.checkSocialCardRebuild()

	// Warm up the JS renderer pool
	b.initializeNativeRenderer(ctx)

	// Set dev build version
	if b.Cfg.IsDev {
		b.Cfg.BuildVersion = time.Now().UnixNano()
	}

	// Pre-create output directories
	if err := b.createOutputDirectories(); err != nil {
		return nil, err
	}

	return &buildSetupResult{
		wasmWg:             wasmWg,
		forceSocialRebuild: forceSocialRebuild,
	}, nil
}

// assetPhase starts the asset building pipeline
func (b *Engine) assetPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildAssetResult {
	assetsReady, discoveryReady, assetWg, assetErrChan := b.Assets.SetupBuilding(ctx, contentAssetsChan)

	return &buildAssetResult{
		assetsReady:    assetsReady,
		discoveryReady: discoveryReady,
		assetWg:        assetWg,
		assetErrChan:   assetErrChan,
	}
}

// scanPhase launches the parallel metadata scanner
func (b *Engine) scanPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildScanResult {
	fileChan := make(chan models.ScannedFile, 1024)
	scannerReady := make(chan struct{})
	metadataResultChan := make(chan *models.MetadataScannerResult, 1)
	scannerErrChan := make(chan error, 1)

	go func() {
		defer close(scannerReady)
		defer close(fileChan)
		defer close(metadataResultChan)
		defer close(scannerErrChan)

		metadataResult, scannerErr := b.Deps.Scanner.Scan(ctx, b.Cfg.ContentDir, b.SourceFs, b.Cfg, fileChan)
		if scannerErr == nil {
			contentAssetsChan <- metadataResult.ContentAssets
		}
		// Always send result and error (even if nil)
		metadataResultChan <- metadataResult
		scannerErrChan <- scannerErr
	}()

	return &buildScanResult{
		fileChan:           fileChan,
		scannerReady:       scannerReady,
		metadataResultChan: metadataResultChan,
		scannerErrChan:     scannerErrChan,
	}
}

// processPhase executes post processing and site-wide orchestration.
// Assets (including image compression) MUST complete before post rendering,
// since post rendering rewrites PNG/JPG/JPEG image references to WebP.
func (b *Engine) processPhase(
	ctx context.Context,
	setup *buildSetupResult,
	assets *buildAssetResult,
	scan *buildScanResult,
) error {
	// Wait for scanner and discovery BEFORE rendering posts.
	// This ensures the image/WebP rewrite map is populated so HTML can reference
	// the correct paths. Image compression continues in the background while posts render.
	metadataResult, discoveryReady, scannerErr, assetErr, _ := b.waitForScannerAndAssets(
		scan.scannerReady, scan.metadataResultChan, scan.scannerErrChan,
		assets.assetWg, assets.assetErrChan, assets.discoveryReady,
	)
	if scannerErr != nil {
		return fmt.Errorf("metadata scan failed: %w", scannerErr)
	}
	if assetErr != nil {
		return fmt.Errorf("failed to build assets: %w", assetErr)
	}

	var siteWideHas404 bool
	if metadataResult != nil && metadataResult.Has404 {
		siteWideHas404 = true
	}

	// Set up site-wide generators (need full asset completion)
	runSiteWide, _ := b.setupSiteWideRendering(ctx, assets.assetsReady, setup.wasmWg, setup.forceSocialRebuild)

	// Wait for discovery signal so image rewrite map is ready before post-processing
	if discoveryReady != nil {
		<-discoveryReady
	}

	// Process Posts (render HTML with WebP image rewrites)
	postResult, processErr := b.processPosts(ctx, b.Cfg.ForceRebuild, setup.forceSocialRebuild, b.State.IsCleanBuild, metadataResult.Files)
	if processErr != nil {
		return fmt.Errorf("post processing failed: %w", processErr)
	}
	if postResult.Has404 {
		siteWideHas404 = true
	}

	// Check if assets changed since last site-wide render
	assetsChanged := b.Assets.CheckChanged(ctx, assets.assetsReady)

	// Site-wide generators
	metadataCtx := &services.MetadataContext{
		AllPosts:       postResult.AllPosts,
		PinnedPosts:    postResult.PinnedPosts,
		TagMap:         postResult.TagMap,
		IndexedPosts:   postResult.IndexedPosts,
		AnyPostChanged: postResult.AnyPostChanged,
	}
	siteWideGroup, siteTimer := runSiteWide(metadataCtx, assetsChanged)

	// Wait for site-wide rendering
	return b.waitForSiteWideRendering(siteWideGroup, siteTimer, siteWideHas404 || b.Deps.Render.Has404Template())
}

// setupWasmDeployment launches WASM compilation asynchronously.
func (b *Engine) setupWasmDeployment(ctx context.Context) *sync.WaitGroup {
	var wasmWg sync.WaitGroup
	wasmWg.Add(1)
	async.FireAndForgetWithCleanup(ctx, b.Logger, "WASM compilation",
		func() error {
			return b.Deps.Wasm.CheckAndUpdate(ctx)
		},
		func() {
			wasmWg.Done()
		})
	return &wasmWg
}

// checkSocialCardRebuild determines if social cards need forced rebuild
func (b *Engine) checkSocialCardRebuild() bool {
	if b.Cfg.ForceRebuild {
		return false
	}
	lastBuildTime := b.Tx.GetLastBuildTime()
	if lastBuildTime.IsZero() {
		return false
	}
	info, err := os.Stat("builder/generators/social.go")
	return err == nil && info.ModTime().After(lastBuildTime)
}

// initializeNativeRenderer warms up the JS renderer pool
func (b *Engine) initializeNativeRenderer(ctx context.Context) {
	if b.NativeRenderer != nil {
		b.NativeRenderer.EnsureInitialized(ctx)
	}
}

// createOutputDirectories creates required output directories
func (b *Engine) createOutputDirectories() error {
	for _, dir := range []string{"tags", "static/images/cards", "sitemap"} {
		if err := b.Sink.MkdirAll(filepath.Join(b.Cfg.OutputDir, dir)); err != nil {
			b.Logger.Error("Failed to create directory", "dir", dir, "error", err)
			return err
		}
	}
	return nil
}

// waitForScannerAndAssets waits for scanner and asset building to complete.
// The discoveryReady signal unblocks post-processing while image compression continues.
func (b *Engine) waitForScannerAndAssets(
	scannerReady <-chan struct{},
	metadataResultChan <-chan *models.MetadataScannerResult,
	scannerErrChan <-chan error,
	assetWg *sync.WaitGroup,
	assetErrChan <-chan error,
	discoveryReady <-chan struct{},
) (*models.MetadataScannerResult, <-chan struct{}, error, error, error) {
	<-scannerReady

	// Receive scanner result and error
	metadataResult := <-metadataResultChan
	scannerErr := <-scannerErrChan

	// Return discoveryReady separately so post-processing can unblock on it.
	// The caller will wait for assetWg separately if needed.
	discoverySignal := discoveryReady

	var assetErr error
	select {
	case err := <-assetErrChan:
		assetErr = err
	default:
	}

	return metadataResult, discoverySignal, scannerErr, assetErr, nil
}

// waitForSiteWideRendering waits for site-wide generators and renders 404 if needed
func (b *Engine) waitForSiteWideRendering(siteWideGroup *errgroup.Group, siteTimer *timeutil.PhaseTimer, siteWideHas404 bool) error {
	if siteWideGroup == nil {
		return nil
	}

	if err := siteWideGroup.Wait(); err != nil {
		if siteTimer != nil {
			siteTimer.Stop()
		}
		return err
	}
	if siteTimer != nil {
		siteTimer.Stop()
	}

	if siteWideHas404 {
		if err := b.Deps.Render.Render404(filepath.Join(b.Cfg.OutputDir, "404.html"), models.PageData{
			Title: "404 Not Found", BaseURL: "", TabTitle: "404 Not Found",
			Config: b.Cfg, RelativePrefix: "/",
		}); err != nil {
			return fmt.Errorf("failed to render 404 page: %w", err)
		}
	}
	return nil
}

// finalizeBuild writes post-build files and commits the transaction
func (b *Engine) finalizeBuild(ctx context.Context, wasmWg *sync.WaitGroup) error {
	// Write .nojekyll file
	if err := b.Sink.WriteFile(filepath.Join(b.Cfg.OutputDir, ".nojekyll"), []byte{}); err != nil {
		return fmt.Errorf("failed to write .nojekyll: %w", err)
	}
	b.Deps.Render.RegisterFile(filepath.Join(b.Cfg.OutputDir, ".nojekyll"))

	// Sync/Commit transaction
	b.Logger.Info("Publishing output...")
	syncTimer := timeutil.StartPhase("Publish")
	// Ensure WASM compilation and PWA generation finished before deploying and publishing
	wasmWg.Wait()

	// Reset ForceRebuild AFTER all async checks have completed
	b.Cfg.ForceRebuild = false

	if err := b.Deps.Wasm.Deploy(ctx, b.Sink); err != nil {
		b.Logger.Warn("Failed to deploy Search WASM", "error", err)
	}
	if err := b.Tx.Commit(ctx); err != nil {
		syncTimer.Stop()
		return fmt.Errorf("failed to publish build transaction: %w", err)
	}
	syncTimer.Stop()

	return nil
}

// processPosts executes post processing and returns the result
func (b *Engine) processPosts(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, files []models.ScannedFile) (*services.PostResult, error) {
	return b.Deps.Post.Process(ctx, shouldForce, forceSocialRebuild, outputMissing, files)
}

// finalizePhase handles post-build cleanup and commit
func (b *Engine) finalizePhase(ctx context.Context, wasmWg *sync.WaitGroup) error {
	// Post-build files and commit
	if err := b.finalizeBuild(ctx, wasmWg); err != nil {
		return err
	}

	// Cleanup orphans (Dev mode only)
	b.cleanupOrphans()

	// Clear memory state
	b.Deps.Render.ClearRenderedFiles()

	// Build complete
	b.Metrics.RecordEnd()
	b.Logger.Info("Build complete")
	b.Metrics.Print()

	if b.Health != nil {
		b.Health.LogSummary()
	}

	return nil
}
